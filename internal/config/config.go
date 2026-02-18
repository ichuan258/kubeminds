package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// K8sProvider identifies which K8s connection strategy to use.
type K8sProvider string

const (
	K8sProviderAuto   K8sProvider = ""       // default: ctrl.GetConfigOrDie() auto-discovery
	K8sProviderLocal  K8sProvider = "local"  // explicit kubeconfig file
	K8sProviderGCloud K8sProvider = "gcloud" // kubeconfig + optional insecure TLS (SSH tunnel)
	K8sProviderAWS    K8sProvider = "aws"    // EKS via AWS credentials (stubbed)
)

// K8sConfig holds Kubernetes connection configuration.
type K8sConfig struct {
	Provider           K8sProvider `yaml:"provider"`
	KubeconfigPath     string      `yaml:"kubeconfigPath"`
	InsecureSkipVerify bool        `yaml:"insecureSkipVerify"`
	Context            string      `yaml:"context"`
}

// Config holds the application configuration
type Config struct {
	MetricsAddr           string    `yaml:"metricsAddr"`
	ProbeAddr             string    `yaml:"probeAddr"`
	EnableLeaderElection  bool      `yaml:"enableLeaderElection"`
	APIKey                string    `yaml:"apiKey"`
	Model                 string    `yaml:"model"`
	BaseURL               string    `yaml:"baseUrl"`
	SkillDir              string    `yaml:"skillDir"`
	AgentTimeoutMinutes   int       `yaml:"agentTimeoutMinutes"`
	K8s                   K8sConfig `yaml:"k8s"`
}

// LoadConfig loads the configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	config := &Config{
		// Set defaults
		MetricsAddr:          ":8080",
		ProbeAddr:            ":8081",
		EnableLeaderElection: false,
		Model:                "gpt-4o",
		BaseURL:              "https://api.openai.com/v1",
		SkillDir:             "skills/",
		AgentTimeoutMinutes:  10,
	}

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// If file doesn't exist, return default config
		// This allows running without a config file if flags are used
		return config, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return config, nil
}
