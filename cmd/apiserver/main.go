package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	kubemindsv1alpha1 "kubeminds/api/v1alpha1"
	"kubeminds/internal/agent"
	"kubeminds/internal/api"
	"kubeminds/internal/config"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(kubemindsv1alpha1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var probeAddr string
	var apiPort int
	var configPath string
	var k8sProvider string
	var kubeconfigPath string
	var k8sContext string
	var insecureSkipVerify bool

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8082", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8083", "The address the probe endpoint binds to.")
	flag.IntVar(&apiPort, "api-port", 8081, "The port the API server binds to.")
	flag.StringVar(&configPath, "config", "cmd/config/config.yaml", "The path to the configuration file.")
	flag.StringVar(&k8sProvider, "k8s-provider", "", "K8s connection provider: '', 'local', 'gcloud', 'aws'.")
	flag.StringVar(&kubeconfigPath, "kubeconfig-path", "", "Path to kubeconfig file (used by local/gcloud providers).")
	flag.StringVar(&k8sContext, "k8s-context", "", "Kubeconfig context name to use (optional).")
	flag.BoolVar(&insecureSkipVerify, "insecure-skip-tls-verify", false, "Skip TLS verification (gcloud SSH tunnel scenarios).")
	flag.Parse()

	// Use Zap for structured logging
	zapLog, _ := zap.NewDevelopment()
	log.SetLogger(zapr.NewLogger(zapLog))

	// Load configuration
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		setupLog.Error(err, "unable to load configuration")
		os.Exit(1)
	}

	// Override config with flags if they were set
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "k8s-provider":
			cfg.K8s.Provider = config.K8sProvider(k8sProvider)
		case "kubeconfig-path":
			cfg.K8s.KubeconfigPath = kubeconfigPath
		case "k8s-context":
			cfg.K8s.Context = k8sContext
		case "insecure-skip-tls-verify":
			cfg.K8s.InsecureSkipVerify = insecureSkipVerify
		}
	})

	// Env variable fallbacks (YAML → Flag → Env)
	if cfg.K8s.Provider == "" {
		if p := os.Getenv("K8S_PROVIDER"); p != "" {
			cfg.K8s.Provider = config.K8sProvider(p)
		}
	}
	if cfg.K8s.KubeconfigPath == "" {
		cfg.K8s.KubeconfigPath = os.Getenv("KUBECONFIG_PATH")
	}
	if !cfg.K8s.InsecureSkipVerify && os.Getenv("INSECURE_SKIP_TLS_VERIFY") == "true" {
		cfg.K8s.InsecureSkipVerify = true
	}

	// Build K8s rest.Config from provider settings
	restCfg, err := config.NewK8sRestConfig(cfg)
	if err != nil {
		setupLog.Error(err, "unable to build K8s rest config")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(restCfg, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Initialize Clientset for internal tools
	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		setupLog.Error(err, "unable to build kubernetes clientset")
		os.Exit(1)
	}

	// Initialize SkillManager
	skillDir := os.Getenv("SKILL_DIR")
	if skillDir == "" {
		skillDir = "skills"
	}
	skillManager, err := agent.NewSkillManager(skillDir, nil)
	if err != nil {
		setupLog.Error(err, "unable to initialize skill manager")
		os.Exit(1)
	}

	// Initialize API Server
	apiServer := api.NewServer(
		mgr.GetClient(),
		clientset,
		skillManager,
		apiPort,
		log.Log.WithName("api-server"),
	)
	go func() {
		setupLog.Info("starting api server", "port", fmt.Sprintf("%d", apiPort))
		if err := apiServer.Start(); err != nil {
			setupLog.Error(err, "problem running api server")
			os.Exit(1)
		}
	}()

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
