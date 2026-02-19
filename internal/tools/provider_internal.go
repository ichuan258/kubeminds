package tools

import (
	"context"
	"k8s.io/client-go/kubernetes"
	"kubeminds/internal/agent"
)

// InternalProvider provides built-in Kubernetes tools
type InternalProvider struct {
	client kubernetes.Interface
}

// NewInternalProvider creates a new internal tool provider
func NewInternalProvider(client kubernetes.Interface) *InternalProvider {
	return &InternalProvider{
		client: client,
	}
}

// ListTools returns the list of internal tools
func (p *InternalProvider) ListTools(ctx context.Context) ([]agent.Tool, error) {
	return ListTools(p.client), nil
}
