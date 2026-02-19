package tools

import (
	"context"
	"kubeminds/internal/agent"
)

// MCPProvider provides tools from Model Context Protocol servers
type MCPProvider struct {
	// configuration for MCP servers will go here
}

// NewMCPProvider creates a new MCP tool provider
func NewMCPProvider() *MCPProvider {
	return &MCPProvider{}
}

// ListTools returns the list of MCP tools
func (p *MCPProvider) ListTools(ctx context.Context) ([]agent.Tool, error) {
	// TODO: Connect to MCP servers and list tools
	// For now, return empty list
	return []agent.Tool{}, nil
}
