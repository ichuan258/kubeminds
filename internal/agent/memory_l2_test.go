package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// mockEventStore is a simple in-memory EventStore for unit tests.
type mockEventStore struct {
	events []AlertEvent
	err    error // if non-nil, both methods return this error
}

func (m *mockEventStore) AppendAlertEvent(_ context.Context, event AlertEvent) error {
	if m.err != nil {
		return m.err
	}
	m.events = append(m.events, event)
	return nil
}

func (m *mockEventStore) GetRecentEvents(_ context.Context, namespace, pod string, limit int) ([]AlertEvent, error) {
	if m.err != nil {
		return nil, m.err
	}
	var out []AlertEvent
	for _, e := range m.events {
		if e.Namespace != namespace {
			continue
		}
		if pod != "" && e.Pod != pod {
			continue
		}
		out = append(out, e)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func sampleEvent(alertName, namespace, pod string, count int) AlertEvent {
	now := time.Now()
	return AlertEvent{
		AlertName: alertName,
		Namespace: namespace,
		Pod:       pod,
		Count:     count,
		FirstSeen: now.Add(-time.Minute),
		LastSeen:  now,
	}
}

// TestMockEventStore_AppendAndGet validates the basic read/write contract via the interface.
func TestMockEventStore_AppendAndGet(t *testing.T) {
	store := &mockEventStore{}
	ctx := context.Background()

	if err := store.AppendAlertEvent(ctx, sampleEvent("OOMKilled", "default", "pod-a", 3)); err != nil {
		t.Fatalf("AppendAlertEvent: %v", err)
	}
	if err := store.AppendAlertEvent(ctx, sampleEvent("CrashLoopBackOff", "default", "pod-b", 1)); err != nil {
		t.Fatalf("AppendAlertEvent: %v", err)
	}

	events, err := store.GetRecentEvents(ctx, "default", "", 10)
	if err != nil {
		t.Fatalf("GetRecentEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
}

// TestMockEventStore_PodFilter validates that pod filtering works as expected.
func TestMockEventStore_PodFilter(t *testing.T) {
	store := &mockEventStore{}
	ctx := context.Background()

	_ = store.AppendAlertEvent(ctx, sampleEvent("OOMKilled", "default", "pod-a", 3))
	_ = store.AppendAlertEvent(ctx, sampleEvent("OOMKilled", "default", "pod-b", 1))

	events, err := store.GetRecentEvents(ctx, "default", "pod-a", 10)
	if err != nil {
		t.Fatalf("GetRecentEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event for pod-a, got %d", len(events))
	}
	if events[0].Pod != "pod-a" {
		t.Errorf("expected pod-a, got %s", events[0].Pod)
	}
}

// TestMockEventStore_Limit validates that the limit parameter is respected.
func TestMockEventStore_Limit(t *testing.T) {
	store := &mockEventStore{}
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_ = store.AppendAlertEvent(ctx, sampleEvent("OOMKilled", "default", "pod-a", i+1))
	}

	events, err := store.GetRecentEvents(ctx, "default", "", 3)
	if err != nil {
		t.Fatalf("GetRecentEvents: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events (limit), got %d", len(events))
	}
}

// TestMockEventStore_NamespaceIsolation validates that events from different namespaces are isolated.
func TestMockEventStore_NamespaceIsolation(t *testing.T) {
	store := &mockEventStore{}
	ctx := context.Background()

	_ = store.AppendAlertEvent(ctx, sampleEvent("OOMKilled", "default", "pod-a", 1))
	_ = store.AppendAlertEvent(ctx, sampleEvent("OOMKilled", "kube-system", "pod-b", 2))

	events, err := store.GetRecentEvents(ctx, "default", "", 10)
	if err != nil {
		t.Fatalf("GetRecentEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event for namespace 'default', got %d", len(events))
	}
}

// TestMockEventStore_Error validates that errors propagate correctly.
func TestMockEventStore_Error(t *testing.T) {
	store := &mockEventStore{err: errTest}
	ctx := context.Background()

	if err := store.AppendAlertEvent(ctx, sampleEvent("OOMKilled", "default", "pod-a", 1)); err == nil {
		t.Fatal("expected error, got nil")
	}
	if _, err := store.GetRecentEvents(ctx, "default", "", 10); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// errTest is a sentinel error for tests.
var errTest = fmt.Errorf("test error")

// TestFormatAlertEvents validates the formatting helper.
func TestFormatAlertEvents(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if got := FormatAlertEvents(nil); got != "" {
			t.Errorf("expected empty string for nil events, got %q", got)
		}
	})

	t.Run("with events", func(t *testing.T) {
		events := []AlertEvent{
			{AlertName: "OOMKilled", Namespace: "default", Pod: "pod-a", Count: 3, LastSeen: time.Now()},
		}
		got := FormatAlertEvents(events)
		if !strings.Contains(got, "OOMKilled") {
			t.Errorf("expected OOMKilled in output, got: %s", got)
		}
		if !strings.Contains(got, "pod-a") {
			t.Errorf("expected pod-a in output, got: %s", got)
		}
		if !strings.Contains(got, "count=3") {
			t.Errorf("expected count=3 in output, got: %s", got)
		}
	})
}

// TestInjectContext_L2Integration verifies that InjectContext adds context to agent memory.
func TestInjectContext_L2Integration(t *testing.T) {
	mockLLM := &MockLLMProvider{
		Responses: map[int]*Message{
			0: {
				Type:    MessageTypeAssistant,
				Content: "Root Cause: pod OOM\nSuggestion: increase memory limit",
			},
		},
	}

	a := NewAgent(mockLLM, nil, 5, nil, nil, Skill{})

	store := &mockEventStore{}
	_ = store.AppendAlertEvent(context.Background(), sampleEvent("OOMKilled", "default", "pod-a", 2))

	events, _ := store.GetRecentEvents(context.Background(), "default", "pod-a", 10)
	if formatted := FormatAlertEvents(events); formatted != "" {
		a.InjectContext(formatted)
	}

	// History should contain system prompt (skill) + injected L2 context + goal.
	result, err := a.Run(context.Background(), "Diagnose pod-a", false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}

	// The injected context message should be in memory history.
	found := false
	for _, msg := range a.memory.GetHistory() {
		if strings.Contains(msg.Content, "OOMKilled") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected L2 event context in agent memory history")
	}
}
