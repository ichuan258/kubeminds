package tools

import (
	"context"
	"errors"
	"testing"

	"k8s.io/client-go/kubernetes/fake"
	"kubeminds/internal/agent"
)

// stubProvider is a test double for agent.ToolProvider
type stubProvider struct {
	tools []agent.Tool
	err   error
}

func (s *stubProvider) ListTools(_ context.Context) ([]agent.Tool, error) {
	return s.tools, s.err
}

// stubTool is a minimal agent.Tool implementation for testing
type stubTool struct {
	name string
}

func (t *stubTool) Name() string                                        { return t.name }
func (t *stubTool) Description() string                                 { return "stub tool" }
func (t *stubTool) Execute(_ context.Context, _ string) (string, error) { return "", nil }
func (t *stubTool) Schema() string                                      { return "{}" }
func (t *stubTool) SafetyLevel() agent.SafetyLevel                      { return agent.SafetyLevelReadOnly }

// TestRouter_NoProviders verifies the router returns an empty list when no providers are registered.
func TestRouter_NoProviders(t *testing.T) {
	r := NewRouter(nil)
	tools, err := r.ListTools(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}
}

// TestRouter_SingleProvider verifies the router correctly aggregates tools from one provider.
func TestRouter_SingleProvider(t *testing.T) {
	r := NewRouter(nil)
	r.AddProvider(&stubProvider{
		tools: []agent.Tool{
			&stubTool{name: "tool_a"},
			&stubTool{name: "tool_b"},
		},
	})

	tools, err := r.ListTools(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}
}

// TestRouter_MultipleProviders verifies the router merges tools from all providers.
func TestRouter_MultipleProviders(t *testing.T) {
	r := NewRouter(nil)
	r.AddProvider(&stubProvider{tools: []agent.Tool{&stubTool{name: "internal_tool"}}})
	r.AddProvider(&stubProvider{tools: []agent.Tool{&stubTool{name: "mcp_tool"}}})
	r.AddProvider(&stubProvider{tools: []agent.Tool{&stubTool{name: "grpc_tool"}}})

	tools, err := r.ListTools(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(tools) != 3 {
		t.Errorf("expected 3 tools, got %d", len(tools))
	}

	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name()] = true
	}
	for _, want := range []string{"internal_tool", "mcp_tool", "grpc_tool"} {
		if !names[want] {
			t.Errorf("expected tool %q in result", want)
		}
	}
}

// TestRouter_PartialFailure verifies the router continues on provider errors and returns
// tools from the healthy providers (partial failure is allowed).
func TestRouter_PartialFailure(t *testing.T) {
	r := NewRouter(nil)
	r.AddProvider(&stubProvider{tools: []agent.Tool{&stubTool{name: "good_tool"}}})
	r.AddProvider(&stubProvider{err: errors.New("provider unavailable")})
	r.AddProvider(&stubProvider{tools: []agent.Tool{&stubTool{name: "another_good_tool"}}})

	tools, err := r.ListTools(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(tools) != 2 {
		t.Errorf("expected 2 tools from healthy providers, got %d", len(tools))
	}
}

// TestInternalProvider_ListTools verifies InternalProvider returns all 12 K8s tools.
func TestInternalProvider_ListTools(t *testing.T) {
	client := fake.NewSimpleClientset()
	p := NewInternalProvider(client)

	tools, err := p.ListTools(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(tools) != 12 {
		t.Errorf("expected 12 tools, got %d", len(tools))
	}

	// Verify all tools have non-empty names
	for _, tool := range tools {
		if tool.Name() == "" {
			t.Errorf("tool has empty name")
		}
	}
}

// TestGRPCProvider_ListTools verifies the gRPC stub returns an empty list without error.
func TestGRPCProvider_ListTools(t *testing.T) {
	p := NewGRPCProvider()
	tools, err := p.ListTools(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(tools) != 0 {
		t.Errorf("expected empty tool list from gRPC stub, got %d", len(tools))
	}
}

// TestMCPProvider_ListTools verifies the MCP stub returns an empty list without error.
func TestMCPProvider_ListTools(t *testing.T) {
	p := NewMCPProvider()
	tools, err := p.ListTools(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(tools) != 0 {
		t.Errorf("expected empty tool list from MCP stub, got %d", len(tools))
	}
}
