package llm

// Router selects an LLM provider by name and delegates all Chat calls to it.
//
// For Phase 2, routing is intentionally simple: one default provider is used for
// all requests. There is no runtime failover — if you need a different provider,
// change defaultProvider in config.yaml and restart.
//
// This design keeps the Agent loop unaware of which underlying provider is active,
// which makes swapping providers trivially safe.

import (
	"context"
	"fmt"

	"kubeminds/internal/agent"
)

// Router implements agent.LLMProvider by dispatching to a named sub-provider.
type Router struct {
	// providers holds all configured providers, keyed by their name (e.g. "openai").
	providers map[string]agent.LLMProvider

	// defaultProvider is the name of the provider that Chat calls are routed to.
	// It must match a key in providers.
	defaultProvider string
}

// NewRouter creates a Router from a pre-built provider map.
// defaultProvider must be one of the keys in providers.
func NewRouter(providers map[string]agent.LLMProvider, defaultProvider string) (*Router, error) {
	if _, ok := providers[defaultProvider]; !ok {
		return nil, fmt.Errorf("llm router: defaultProvider %q is not configured in providers %v",
			defaultProvider, providerNames(providers))
	}
	return &Router{
		providers:       providers,
		defaultProvider: defaultProvider,
	}, nil
}

// Chat implements agent.LLMProvider by forwarding the call to the default provider.
func (r *Router) Chat(ctx context.Context, messages []agent.Message, tools []agent.Tool) (*agent.Message, error) {
	p, ok := r.providers[r.defaultProvider]
	if !ok {
		// Defensive: should not happen after NewRouter validates, but guard anyway.
		return nil, fmt.Errorf("llm router: provider %q not found", r.defaultProvider)
	}
	return p.Chat(ctx, messages, tools)
}

// DefaultProvider returns the name of the currently active provider.
func (r *Router) DefaultProvider() string {
	return r.defaultProvider
}

// providerNames extracts map keys as a slice for use in error messages.
func providerNames(m map[string]agent.LLMProvider) []string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	return names
}
