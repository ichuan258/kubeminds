package llm

// factory.go constructs an LLM Router from the application's LLMConfig.
//
// Only providers explicitly listed under llm.providers in config.yaml are registered.
// If a provider's apiKey is empty, it is still registered — the provider itself will
// return an auth error when called, which gives a clear failure message at runtime
// rather than a silent skip at startup.

import (
	"fmt"

	"kubeminds/internal/agent"
	"kubeminds/internal/config"
)

// NewRouterFromConfig builds a Router from the LLM configuration block.
// It creates a concrete provider for each entry in cfg.Providers and wraps them
// in a Router that selects the one named by cfg.DefaultProvider.
//
// Supported provider names: "openai", "gemini", "anthropic".
// Unknown names return an error so misconfiguration is caught at startup.
func NewRouterFromConfig(cfg config.LLMConfig) (*Router, error) {
	if cfg.DefaultProvider == "" {
		return nil, fmt.Errorf("llm factory: llm.defaultProvider must be set")
	}
	if len(cfg.Providers) == 0 {
		return nil, fmt.Errorf("llm factory: no providers configured under llm.providers")
	}

	providers := make(map[string]agent.LLMProvider, len(cfg.Providers))

	for name, pcfg := range cfg.Providers {
		p, err := buildProvider(name, pcfg)
		if err != nil {
			return nil, fmt.Errorf("llm factory: failed to build provider %q: %w", name, err)
		}
		providers[name] = p
	}

	return NewRouter(providers, cfg.DefaultProvider)
}

// buildProvider instantiates a single provider from its ProviderConfig.
func buildProvider(name string, cfg config.ProviderConfig) (agent.LLMProvider, error) {
	switch name {
	case "openai":
		// OpenAIProvider handles OpenAI-compatible endpoints.
		// If baseUrl is empty, the library default (https://api.openai.com/v1) is used.
		return NewOpenAIProvider(cfg.APIKey, cfg.Model, cfg.BaseURL), nil

	case "gemini":
		// GeminiProvider wraps OpenAIProvider with Google's compat endpoint.
		// If baseUrl is set in config, it overrides the built-in default.
		return NewGeminiProvider(cfg.APIKey, cfg.Model, cfg.BaseURL), nil

	case "anthropic":
		// AnthropicProvider uses the native Anthropic SDK.
		// If baseUrl is set in config, it overrides https://api.anthropic.com.
		return NewAnthropicProvider(cfg.APIKey, cfg.Model, cfg.BaseURL), nil

	default:
		return nil, fmt.Errorf("unknown provider name %q; supported: openai, gemini, anthropic", name)
	}
}
