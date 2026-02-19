package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
	"kubeminds/internal/crypto"
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

// AlertAggregatorConfig holds configuration for the alert aggregator.
type AlertAggregatorConfig struct {
	// WindowSize is the sliding deduplication window duration (e.g. "60s", "2m").
	WindowSize string `yaml:"windowSize"`
	// SweepInterval is how often expired groups are checked (e.g. "5s").
	SweepInterval string `yaml:"sweepInterval"`
	// TargetNamespace is the namespace where DiagnosisTasks are created.
	TargetNamespace string `yaml:"targetNamespace"`
}

// ParseAlertAggregatorConfig parses duration fields from AlertAggregatorConfig.
func ParseAlertAggregatorConfig(cfg AlertAggregatorConfig) (windowSize, sweepInterval time.Duration, err error) {
	windowSize, err = time.ParseDuration(cfg.WindowSize)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid alertAggregator.windowSize %q: %w", cfg.WindowSize, err)
	}
	sweepInterval, err = time.ParseDuration(cfg.SweepInterval)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid alertAggregator.sweepInterval %q: %w", cfg.SweepInterval, err)
	}
	return windowSize, sweepInterval, nil
}

// ProviderConfig holds configuration for a single LLM provider.
// APIKey may be a plain-text string or an encrypted value prefixed with "enc:aes256:".
// Encrypted values are decrypted at load time using KUBEMINDS_MASTER_KEY (see internal/crypto).
type ProviderConfig struct {
	// APIKey is the provider's API key.
	// Store as "enc:aes256:..." to keep it encrypted at rest in config.yaml.
	// Generate an encrypted value with: make encrypt-key KEY=<your-key>
	APIKey string `yaml:"apiKey"` // #nosec

	// Model is the model identifier (e.g. "gpt-4o", "gemini-2.0-flash", "claude-sonnet-4-6").
	Model string `yaml:"model"`

	// BaseURL overrides the provider's default API endpoint.
	// Leave empty to use the provider-specific default.
	BaseURL string `yaml:"baseUrl"`
}

// LLMConfig holds the multi-provider LLM configuration.
// Only the provider named by DefaultProvider is used at runtime; the others are ignored.
// This lets operators maintain multiple provider configs and switch by changing one field.
type LLMConfig struct {
	// DefaultProvider selects which entry in Providers is used.
	// Supported values: "openai", "gemini", "anthropic".
	DefaultProvider string `yaml:"defaultProvider"`

	// Providers maps provider names to their configurations.
	// Keys must match the values supported by the LLM factory (openai/gemini/anthropic).
	Providers map[string]ProviderConfig `yaml:"providers"`
}

// RedisConfig holds configuration for the L2 Redis event store.
type RedisConfig struct {
	// Addr is the Redis server address (host:port). Leave empty to disable L2.
	Addr string `yaml:"addr"`
	// Password is the Redis password. Supports "enc:aes256:..." encrypted values.
	Password string `yaml:"password"` // #nosec
	// DB is the Redis database number (default 0).
	DB int `yaml:"db"`
	// EventTTL is how long L2 stream events are retained (default "24h").
	EventTTL string `yaml:"eventTTL"`
}

// ParseRedisEventTTL parses the EventTTL duration from RedisConfig.
// Returns 24h as the default when EventTTL is empty.
func ParseRedisEventTTL(cfg RedisConfig) (time.Duration, error) {
	if cfg.EventTTL == "" {
		return 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(cfg.EventTTL)
	if err != nil {
		return 0, fmt.Errorf("invalid redis.eventTTL %q: %w", cfg.EventTTL, err)
	}
	return d, nil
}

// PostgreSQLConfig holds configuration for the L3 PostgreSQL knowledge base.
type PostgreSQLConfig struct {
	// DSN is the PostgreSQL connection string. Leave empty to disable L3.
	// Example: "postgres://user:pass@localhost:5432/kubeminds?sslmode=disable"
	DSN string `yaml:"dsn"`
	// MaxOpenConns is the maximum number of open connections (default 10).
	MaxOpenConns int `yaml:"maxOpenConns"`
	// EmbedDim is the embedding vector dimension (default 1536 for text-embedding-3-small).
	EmbedDim int `yaml:"embedDim"`
}

// MCPConfig holds configuration for Model Context Protocol servers.
type MCPConfig struct {
	Servers map[string]MCPServerConfig `yaml:"servers"`
}

// MCPServerConfig defines how to connect to an MCP server.
type MCPServerConfig struct {
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
	Env     []string `yaml:"env"`
}

// GRPCConfig holds configuration for external gRPC tool services.
type GRPCConfig struct {
	Services map[string]GRPCServiceConfig `yaml:"services"`
}

// GRPCServiceConfig defines how to connect to a gRPC tool service.
type GRPCServiceConfig struct {
	Address string `yaml:"address"`
	TLS     bool   `yaml:"tls"`
}

// Config holds the application configuration.
// Fields under llm.providers[*].apiKey support "enc:aes256:..." encrypted values —
// they are transparently decrypted by LoadConfig using KUBEMINDS_MASTER_KEY.
type Config struct {
	MetricsAddr          string                `yaml:"metricsAddr"`
	ProbeAddr            string                `yaml:"probeAddr"`
	EnableLeaderElection bool                  `yaml:"enableLeaderElection"`
	SkillDir             string                `yaml:"skillDir"`
	AgentTimeoutMinutes  int                   `yaml:"agentTimeoutMinutes"`
	K8s                  K8sConfig             `yaml:"k8s"`
	AlertAggregator      AlertAggregatorConfig `yaml:"alertAggregator"`

	// LLM holds multi-provider LLM configuration.
	// Use llm.defaultProvider to select the active provider.
	LLM LLMConfig `yaml:"llm"`

	// MCP holds configuration for MCP servers.
	MCP MCPConfig `yaml:"mcp"`

	// GRPC holds configuration for gRPC tool services.
	GRPC GRPCConfig `yaml:"grpc"`

	// Redis holds configuration for the L2 event store.
	// Leave Redis.Addr empty to run without L2 (default).
	Redis RedisConfig `yaml:"redis"`

	// PostgreSQL holds configuration for the L3 knowledge base.
	// Leave PostgreSQL.DSN empty to run without L3 (default).
	PostgreSQL PostgreSQLConfig `yaml:"postgres"`
}

// LoadConfig loads the configuration from a YAML file.
// After loading, any provider apiKey values prefixed with "enc:aes256:" are automatically
// decrypted using the KUBEMINDS_MASTER_KEY environment variable.
func LoadConfig(path string) (*Config, error) {
	config := defaultConfig()

	// If file doesn't exist, return defaults (allows running with flags only).
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return config, nil
	}

	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Decrypt any encrypted API keys in the provider configs.
	// We iterate over a copy of the map so we can write back the decrypted values.
	if err := decryptProviderKeys(config); err != nil {
		return nil, err
	}

	return config, nil
}

// defaultConfig returns a Config populated with sensible defaults.
func defaultConfig() *Config {
	return &Config{
		MetricsAddr:          ":8080",
		ProbeAddr:            ":8081",
		EnableLeaderElection: false,
		SkillDir:             "skills/",
		AgentTimeoutMinutes:  10,
		AlertAggregator: AlertAggregatorConfig{
			WindowSize:      "60s",
			SweepInterval:   "5s",
			TargetNamespace: "default",
		},
		LLM: LLMConfig{
			DefaultProvider: "openai",
			Providers:       map[string]ProviderConfig{},
		},
		MCP: MCPConfig{
			Servers: map[string]MCPServerConfig{},
		},
		GRPC: GRPCConfig{
			Services: map[string]GRPCServiceConfig{},
		},
		Redis: RedisConfig{
			EventTTL: "24h",
		},
		PostgreSQL: PostgreSQLConfig{
			MaxOpenConns: 10,
			EmbedDim:     1536,
		},
	}
}

// decryptProviderKeys iterates over all configured providers and decrypts any API key
// that carries the "enc:aes256:" prefix. The decrypted values replace the encrypted ones
// in-place so the rest of the application always works with plain-text keys in memory.
//
// If any key requires decryption but KUBEMINDS_MASTER_KEY is absent or wrong, an error
// is returned and the application should refuse to start.
func decryptProviderKeys(cfg *Config) error {
	for name, provider := range cfg.LLM.Providers {
		if !crypto.IsEncrypted(provider.APIKey) {
			continue
		}

		plainKey, err := crypto.DecryptValue(provider.APIKey)
		if err != nil {
			return fmt.Errorf("config: failed to decrypt apiKey for provider %q: %w", name, err)
		}

		// Write back the decrypted value. Map values are not addressable in Go,
		// so we must reassign the whole struct.
		provider.APIKey = plainKey
		cfg.LLM.Providers[name] = provider
	}
	return nil
}
