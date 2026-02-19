package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/go-logr/zapr"
	goredis "github.com/redis/go-redis/v9"
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
	"kubeminds/internal/alert"
	"kubeminds/internal/api"
	"kubeminds/internal/config"
	"kubeminds/internal/controller"
	"kubeminds/internal/llm"
	"kubeminds/internal/tools"
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

	// Initialize Alert Aggregator
	windowSize, sweepInterval, err := config.ParseAlertAggregatorConfig(cfg.AlertAggregator)
	if err != nil {
		setupLog.Error(err, "invalid alert aggregator configuration")
		os.Exit(1)
	}
	aggregator := alert.NewAggregator(
		mgr.GetClient(),
		cfg.AlertAggregator.TargetNamespace,
		windowSize,
		sweepInterval,
		log.Log.WithName("alert-aggregator"),
	)
	alertHandler := alert.NewHandler(aggregator, log.Log.WithName("alert-handler"))

	// Create Tool Router
	toolRouter := tools.NewRouter(slog.Default())
	toolRouter.AddProvider(tools.NewInternalProvider(clientset))
	toolRouter.AddProvider(tools.NewMCPProvider())
	toolRouter.AddProvider(tools.NewGRPCProvider())

	// Build LLM Router for the ping endpoint.
	// A failed router build is non-fatal for the API server — the ping endpoint
	// will return 503 if the router is nil, which is the right behavior when
	// LLM config is intentionally omitted (e.g. alert-only deployments).
	var llmRouter *llm.Router
	if cfg.LLM.DefaultProvider != "" && len(cfg.LLM.Providers) > 0 {
		r, err := llm.NewRouterFromConfig(cfg.LLM)
		if err != nil {
			setupLog.Error(err, "failed to build LLM router; /api/v1/llm/ping will be unavailable")
		} else {
			llmRouter = r
		}
	}

	// Initialize L2 Event Store (optional — enabled when redis.addr is set in config).
	var l2Store agent.EventStore
	if cfg.Redis.Addr != "" {
		eventTTL, err := config.ParseRedisEventTTL(cfg.Redis)
		if err != nil {
			setupLog.Error(err, "invalid redis.eventTTL configuration")
			os.Exit(1)
		}
		redisClient := goredis.NewClient(&goredis.Options{
			Addr:     cfg.Redis.Addr,
			Password: cfg.Redis.Password,
			DB:       cfg.Redis.DB,
		})
		l2Store = agent.NewRedisEventStore(redisClient, eventTTL)
		aggregator.WithL2Store(l2Store)
		setupLog.Info("L2 Redis event store enabled", "addr", cfg.Redis.Addr)
	}

	// Initialize L3 Knowledge Base (optional — enabled when postgres.dsn is set in config).
	var knowledgeBase agent.KnowledgeBase
	var embedder agent.EmbeddingProvider
	if cfg.PostgreSQL.DSN != "" {
		embedDim := cfg.PostgreSQL.EmbedDim
		if embedDim == 0 {
			embedDim = 1536
		}
		kb, err := agent.NewPGKnowledgeBaseFromDSN(context.Background(), cfg.PostgreSQL.DSN, embedDim)
		if err != nil {
			setupLog.Error(err, "failed to connect to PostgreSQL for L3 knowledge base")
			os.Exit(1)
		}
		if err := kb.InitSchema(context.Background()); err != nil {
			setupLog.Error(err, "failed to initialize L3 schema")
			os.Exit(1)
		}
		knowledgeBase = kb

		// Reuse the openai provider's API key and base URL for embedding generation.
		openaiCfg := cfg.LLM.Providers["openai"]
		embedder = llm.NewOpenAIEmbedder(openaiCfg.APIKey, openaiCfg.BaseURL)
		setupLog.Info("L3 PostgreSQL knowledge base enabled")
	}

	// Register the DiagnosisTask controller with the manager.
	agentTimeout := time.Duration(cfg.AgentTimeoutMinutes) * time.Minute
	if err := (&controller.DiagnosisTaskReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		K8sClient:     clientset,
		SkillDir:      skillDir,
		AgentTimeout:  agentTimeout,
		LLMProvider:   llmRouter,
		ToolRouter:    toolRouter,
		L2Store:       l2Store,
		KnowledgeBase: knowledgeBase,
		Embedder:      embedder,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create DiagnosisTask controller")
		os.Exit(1)
	}

	// Initialize API Server
	apiServer := api.NewServer(
		mgr.GetClient(),
		clientset,
		skillManager,
		toolRouter,
		apiPort,
		log.Log.WithName("api-server"),
	).WithAlertHandler(alertHandler).WithLLMRouter(llmRouter)

	go func() {
		setupLog.Info("starting api server", "port", fmt.Sprintf("%d", apiPort))
		if err := apiServer.Start(); err != nil {
			setupLog.Error(err, "problem running api server")
			os.Exit(1)
		}
	}()

	setupLog.Info("starting manager")
	sigCtx := ctrl.SetupSignalHandler()

	// Start the alert aggregator sweep loop, tied to the process signal context.
	go aggregator.Run(sigCtx)

	if err := mgr.Start(sigCtx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
