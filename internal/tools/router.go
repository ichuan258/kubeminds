package tools

import (
	"context"
	"kubeminds/internal/agent"
	"log/slog"
)

// Router aggregates tools from multiple providers
type Router struct {
	providers []agent.ToolProvider
	logger    *slog.Logger
}

// NewRouter creates a new tool router
func NewRouter(logger *slog.Logger) *Router {
	if logger == nil {
		logger = slog.Default()
	}
	return &Router{
		logger: logger,
	}
}

// AddProvider adds a tool provider to the router
func (r *Router) AddProvider(provider agent.ToolProvider) {
	r.providers = append(r.providers, provider)
}

// ListTools returns a list of all tools from all providers
func (r *Router) ListTools(ctx context.Context) ([]agent.Tool, error) {
	var allTools []agent.Tool
	for i, provider := range r.providers {
		providerTools, err := provider.ListTools(ctx)
		if err != nil {
			// External providers (MCP, gRPC) may not be ready — log as warn to avoid noise
			r.logger.Warn("failed to list tools from provider, skipping", "provider_index", i, "error", err)
			continue
		}
		allTools = append(allTools, providerTools...)
	}
	return allTools, nil
}
