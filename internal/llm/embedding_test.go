package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"kubeminds/internal/agent"
)

// fakeEmbeddingResponse builds a minimal OpenAI embeddings API response.
func fakeEmbeddingResponse(dim int) []byte {
	embedding := make([]float64, dim)
	for i := range embedding {
		embedding[i] = 0.1
	}
	resp := map[string]interface{}{
		"object": "list",
		"data": []map[string]interface{}{
			{"object": "embedding", "index": 0, "embedding": embedding},
		},
		"model": "text-embedding-3-small",
		"usage": map[string]int{"prompt_tokens": 3, "total_tokens": 3},
	}
	b, _ := json.Marshal(resp)
	return b
}

// newFakeEmbeddingServer creates a test HTTP server that returns a canned embedding response.
func newFakeEmbeddingServer(t *testing.T, dim int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fakeEmbeddingResponse(dim))
	}))
}

func TestOpenAIEmbedder_Embed(t *testing.T) {
	const dim = 1536
	srv := newFakeEmbeddingServer(t, dim)
	defer srv.Close()

	embedder := NewOpenAIEmbedder("test-key", srv.URL)

	vec, err := embedder.Embed(context.Background(), "container OOM killed")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vec) != dim {
		t.Errorf("expected %d dims, got %d", dim, len(vec))
	}
}

func TestOpenAIEmbedder_EmptyText(t *testing.T) {
	embedder := NewOpenAIEmbedder("test-key", "")

	if _, err := embedder.Embed(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty input, got nil")
	}
}

func TestOpenAIEmbedder_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"invalid key","type":"auth_error"}}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	embedder := NewOpenAIEmbedder("bad-key", srv.URL)

	if _, err := embedder.Embed(context.Background(), "some text"); err == nil {
		t.Fatal("expected API error, got nil")
	}
}

// TestEmbeddingProvider_Interface ensures OpenAIEmbedder satisfies agent.EmbeddingProvider.
func TestEmbeddingProvider_Interface(t *testing.T) {
	var _ agent.EmbeddingProvider = (*OpenAIEmbedder)(nil)
}

// mockEmbeddingProviderLLM is a local mock for the llm package tests.
type mockEmbeddingProviderLLM struct {
	dim int
	err error
}

func (m *mockEmbeddingProviderLLM) Embed(_ context.Context, text string) ([]float32, error) {
	if m.err != nil {
		return nil, m.err
	}
	if text == "" {
		return nil, fmt.Errorf("empty text")
	}
	return make([]float32, m.dim), nil
}

func TestMockEmbeddingProvider(t *testing.T) {
	m := &mockEmbeddingProviderLLM{dim: 4}
	vec, err := m.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vec) != 4 {
		t.Errorf("expected 4 dims, got %d", len(vec))
	}
}
