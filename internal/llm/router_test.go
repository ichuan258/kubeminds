package llm

import (
	"context"
	"errors"
	"testing"

	"kubeminds/internal/agent"
)

// stubProvider is a minimal agent.LLMProvider for testing.
type stubProvider struct {
	name    string // just for identification in assertions
	callErr error  // if non-nil, Chat returns this error
}

func (s *stubProvider) Chat(_ context.Context, _ []agent.Message, _ []agent.Tool) (*agent.Message, error) {
	if s.callErr != nil {
		return nil, s.callErr
	}
	return &agent.Message{
		Type:    agent.MessageTypeAssistant,
		Content: "response from " + s.name,
	}, nil
}

func TestNewRouter_Success(t *testing.T) {
	providers := map[string]agent.LLMProvider{
		"openai":    &stubProvider{name: "openai"},
		"anthropic": &stubProvider{name: "anthropic"},
	}

	router, err := NewRouter(providers, "openai")
	if err != nil {
		t.Fatalf("NewRouter() unexpected error: %v", err)
	}
	if router.DefaultProvider() != "openai" {
		t.Errorf("DefaultProvider() = %q, want %q", router.DefaultProvider(), "openai")
	}
}

func TestNewRouter_UnknownDefault(t *testing.T) {
	providers := map[string]agent.LLMProvider{
		"openai": &stubProvider{name: "openai"},
	}

	_, err := NewRouter(providers, "gemini") // "gemini" not in providers
	if err == nil {
		t.Error("NewRouter() should return an error when defaultProvider is not in providers")
	}
}

func TestRouter_Chat_RoutesToDefault(t *testing.T) {
	providers := map[string]agent.LLMProvider{
		"openai":    &stubProvider{name: "openai"},
		"anthropic": &stubProvider{name: "anthropic"},
	}

	router, _ := NewRouter(providers, "anthropic")
	resp, err := router.Chat(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Chat() unexpected error: %v", err)
	}
	if resp.Content != "response from anthropic" {
		t.Errorf("Chat() content = %q, want response from anthropic", resp.Content)
	}
}

func TestRouter_Chat_PropagatesError(t *testing.T) {
	wantErr := errors.New("api unavailable")
	providers := map[string]agent.LLMProvider{
		"openai": &stubProvider{name: "openai", callErr: wantErr},
	}

	router, _ := NewRouter(providers, "openai")
	_, err := router.Chat(context.Background(), nil, nil)
	if !errors.Is(err, wantErr) {
		t.Errorf("Chat() error = %v, want %v", err, wantErr)
	}
}
