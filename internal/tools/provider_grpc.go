package tools

import (
	"context"
	"kubeminds/internal/agent"
)

// GRPCProvider provides tools from gRPC services
type GRPCProvider struct {
	// configuration for gRPC services will go here
}

// NewGRPCProvider creates a new gRPC tool provider
func NewGRPCProvider() *GRPCProvider {
	return &GRPCProvider{}
}

// ListTools returns the list of gRPC tools
func (p *GRPCProvider) ListTools(ctx context.Context) ([]agent.Tool, error) {
	// TODO: Connect to gRPC services and list tools
	// For now, return empty list
	return []agent.Tool{}, nil
}
