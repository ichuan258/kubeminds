package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds the application configuration
type Config struct {
	MetricsAddr          string `yaml:"metricsAddr"`
	ProbeAddr            string `yaml:"probeAddr"`
	EnableLeaderElection bool   `yaml:"enableLeaderElection"`
	APIKey               string `yaml:"apiKey"`
	Model                string `yaml:"model"`
	BaseURL              string `yaml:"baseUrl"`
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
