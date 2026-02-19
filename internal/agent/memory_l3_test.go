package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// mockKnowledgeBase is an in-memory KnowledgeBase for unit tests.
type mockKnowledgeBase struct {
	findings []storedFinding
	err      error
}

type storedFinding struct {
	finding   KnowledgeFinding
	embedding []float32
}

func (m *mockKnowledgeBase) InitSchema(_ context.Context) error {
	return m.err
}

func (m *mockKnowledgeBase) SaveDiagnosis(_ context.Context, finding KnowledgeFinding, embedding []float32) error {
	if m.err != nil {
		return m.err
	}
	m.findings = append(m.findings, storedFinding{finding: finding, embedding: embedding})
	return nil
}

func (m *mockKnowledgeBase) SearchSimilar(_ context.Context, queryEmbedding []float32, limit int) ([]KnowledgeFinding, error) {
	if m.err != nil {
		return nil, m.err
	}
	if len(queryEmbedding) == 0 {
		return nil, nil
	}
	var out []KnowledgeFinding
	for _, s := range m.findings {
		out = append(out, s.finding)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// mockEmbeddingProvider returns a fixed-length zero vector for any input.
type mockEmbeddingProvider struct {
	dim int
	err error
}

func (m *mockEmbeddingProvider) Embed(_ context.Context, _ string) ([]float32, error) {
	if m.err != nil {
		return nil, m.err
	}
	return make([]float32, m.dim), nil
}

// sampleFinding builds a KnowledgeFinding for tests.
func sampleFinding(alertName, namespace, rootCause, suggestion string) KnowledgeFinding {
	return KnowledgeFinding{
		ID:         "test-id",
		AlertName:  alertName,
		Namespace:  namespace,
		RootCause:  rootCause,
		Suggestion: suggestion,
		CreatedAt:  time.Now(),
	}
}

// TestMockKnowledgeBase_SaveAndSearch validates the basic contract of the KnowledgeBase interface.
func TestMockKnowledgeBase_SaveAndSearch(t *testing.T) {
	kb := &mockKnowledgeBase{}
	embedder := &mockEmbeddingProvider{dim: 4}
	ctx := context.Background()

	f := sampleFinding("OOMKilled", "default", "container exceeded memory limit", "increase memory limit")
	emb, _ := embedder.Embed(ctx, f.RootCause)

	if err := kb.SaveDiagnosis(ctx, f, emb); err != nil {
		t.Fatalf("SaveDiagnosis: %v", err)
	}

	results, err := kb.SearchSimilar(ctx, emb, 5)
	if err != nil {
		t.Fatalf("SearchSimilar: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].AlertName != "OOMKilled" {
		t.Errorf("expected OOMKilled, got %s", results[0].AlertName)
	}
}

// TestMockKnowledgeBase_EmptyEmbedding validates that nil/empty embedding returns no results.
func TestMockKnowledgeBase_EmptyEmbedding(t *testing.T) {
	kb := &mockKnowledgeBase{}
	ctx := context.Background()

	results, err := kb.SearchSimilar(ctx, nil, 5)
	if err != nil {
		t.Fatalf("SearchSimilar with nil embedding: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for nil embedding, got %d", len(results))
	}
}

// TestMockKnowledgeBase_Limit validates that the limit parameter is respected.
func TestMockKnowledgeBase_Limit(t *testing.T) {
	kb := &mockKnowledgeBase{}
	embedder := &mockEmbeddingProvider{dim: 4}
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		f := sampleFinding("OOMKilled", "default", "root cause", "suggestion")
		emb, _ := embedder.Embed(ctx, f.RootCause)
		_ = kb.SaveDiagnosis(ctx, f, emb)
	}

	results, err := kb.SearchSimilar(ctx, []float32{0, 0, 0, 0}, 3)
	if err != nil {
		t.Fatalf("SearchSimilar: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results (limit), got %d", len(results))
	}
}

// TestMockKnowledgeBase_Error validates that errors propagate correctly.
func TestMockKnowledgeBase_Error(t *testing.T) {
	kb := &mockKnowledgeBase{err: fmt.Errorf("db unavailable")}
	ctx := context.Background()

	if err := kb.InitSchema(ctx); err == nil {
		t.Fatal("expected error from InitSchema, got nil")
	}
	if err := kb.SaveDiagnosis(ctx, KnowledgeFinding{}, nil); err == nil {
		t.Fatal("expected error from SaveDiagnosis, got nil")
	}
	if _, err := kb.SearchSimilar(ctx, []float32{1}, 5); err == nil {
		t.Fatal("expected error from SearchSimilar, got nil")
	}
}

// TestFormatHistoricalFindings validates the formatting helper.
func TestFormatHistoricalFindings(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if got := FormatHistoricalFindings(nil); got != "" {
			t.Errorf("expected empty string for nil findings, got %q", got)
		}
	})

	t.Run("with findings", func(t *testing.T) {
		findings := []KnowledgeFinding{
			{
				AlertName: "OOMKilled", Namespace: "default",
				RootCause: "memory limit exceeded", Suggestion: "increase limit",
				CreatedAt: time.Now(),
			},
		}
		got := FormatHistoricalFindings(findings)
		for _, want := range []string{"OOMKilled", "memory limit exceeded", "increase limit", "default"} {
			if !strings.Contains(got, want) {
				t.Errorf("expected %q in output, got: %s", want, got)
			}
		}
	})
}

// TestInjectContext_L3Integration verifies that L3 historical context is injected into agent memory.
func TestInjectContext_L3Integration(t *testing.T) {
	mockLLM := &MockLLMProvider{
		Responses: map[int]*Message{
			0: {
				Type:    MessageTypeAssistant,
				Content: "Root Cause: OOM\nSuggestion: increase memory limit",
			},
		},
	}

	a := NewAgent(mockLLM, nil, 5, nil, nil, Skill{})

	kb := &mockKnowledgeBase{}
	embedder := &mockEmbeddingProvider{dim: 4}

	// Pre-populate knowledge base with a historical finding.
	f := sampleFinding("OOMKilled", "default", "container OOM", "increase memory limit")
	emb, _ := embedder.Embed(context.Background(), f.RootCause)
	_ = kb.SaveDiagnosis(context.Background(), f, emb)

	// Simulate what the controller does: query L3 and inject context.
	queryEmb, _ := embedder.Embed(context.Background(), "OOMKilled default")
	historicals, _ := kb.SearchSimilar(context.Background(), queryEmb, 3)
	if formatted := FormatHistoricalFindings(historicals); formatted != "" {
		a.InjectContext(formatted)
	}

	result, err := a.Run(context.Background(), "Diagnose OOM in pod-a", false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}

	// Verify historical context was injected into memory.
	found := false
	for _, msg := range a.memory.GetHistory() {
		if strings.Contains(msg.Content, "knowledge base") || strings.Contains(msg.Content, "container OOM") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected L3 historical context in agent memory history")
	}
}
