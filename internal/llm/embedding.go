package llm

import (
	"context"
	"fmt"

	"github.com/sashabaranov/go-openai"
	"kubeminds/internal/agent"
)

// Compile-time check: OpenAIEmbedder must satisfy agent.EmbeddingProvider.
var _ agent.EmbeddingProvider = (*OpenAIEmbedder)(nil)

// OpenAIEmbedder implements agent.EmbeddingProvider using the OpenAI Embeddings API.
// It is compatible with any OpenAI-compatible endpoint (e.g. local proxies).
type OpenAIEmbedder struct {
	client *openai.Client
	model  openai.EmbeddingModel
}

// NewOpenAIEmbedder creates an OpenAIEmbedder.
// apiKey and baseURL follow the same semantics as NewOpenAIProvider.
// model defaults to text-embedding-3-small when empty.
func NewOpenAIEmbedder(apiKey, baseURL string) *OpenAIEmbedder {
	cfg := openai.DefaultConfig(apiKey)
	if baseURL != "" {
		cfg.BaseURL = baseURL
	}
	return &OpenAIEmbedder{
		client: openai.NewClientWithConfig(cfg),
		model:  openai.SmallEmbedding3, // text-embedding-3-small, 1536 dims
	}
}

// Embed calls the OpenAI Embeddings API and returns the first embedding vector.
func (e *OpenAIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return nil, fmt.Errorf("embedding: input text must not be empty")
	}

	resp, err := e.client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
		Model: e.model,
		Input: []string{text},
	})
	if err != nil {
		return nil, fmt.Errorf("embedding: openai api error: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("embedding: no embeddings returned by api")
	}

	return resp.Data[0].Embedding, nil
}
