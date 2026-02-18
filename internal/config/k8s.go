package config

import (
	"fmt"
	"os"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
)

// NewK8sRestConfig builds a *rest.Config based on the configured K8s provider.
//   - "" (auto):  ctrl.GetConfigOrDie() — in-cluster → KUBECONFIG env → ~/.kube/config
//   - "local":    load from explicit kubeconfig file path
//   - "gcloud":   like local, with optional InsecureSkipVerify for SSH tunnel scenarios
//   - "aws":      stubbed — returns error
func NewK8sRestConfig(cfg *Config) (*rest.Config, error) {
	switch cfg.K8s.Provider {
	case K8sProviderLocal:
		return buildFromKubeconfig(cfg.K8s.KubeconfigPath, cfg.K8s.Context, false)

	case K8sProviderGCloud:
		return buildFromKubeconfig(cfg.K8s.KubeconfigPath, cfg.K8s.Context, cfg.K8s.InsecureSkipVerify)

	case K8sProviderAWS:
		return nil, fmt.Errorf("k8s provider %q is not yet implemented", K8sProviderAWS)

	default: // K8sProviderAuto
		return ctrl.GetConfigOrDie(), nil
	}
}

// buildFromKubeconfig constructs a rest.Config from a kubeconfig file.
// If path is empty, it falls back to standard KUBECONFIG env / ~/.kube/config discovery.
func buildFromKubeconfig(path, contextName string, insecureSkipVerify bool) (*rest.Config, error) {
	if path != "" {
		resolved, err := expandHome(path)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve kubeconfig path %q: %w", path, err)
		}
		path = resolved
	}

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if path != "" {
		loadingRules.ExplicitPath = path
	}

	overrides := &clientcmd.ConfigOverrides{}
	if contextName != "" {
		overrides.CurrentContext = contextName
	}

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)

	restCfg, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to build rest.Config from kubeconfig %q: %w", path, err)
	}

	if insecureSkipVerify {
		restCfg.TLSClientConfig = rest.TLSClientConfig{Insecure: true}
		restCfg.CAData = nil
		restCfg.CAFile = ""
	}

	return restCfg, nil
}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(path string) (string, error) {
	if len(path) == 0 || path[0] != '~' {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve home directory: %w", err)
	}
	return home + path[1:], nil
}
