package alert

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	kubemindsv1alpha1 "kubeminds/api/v1alpha1"
)

func newTestAggregator(windowSize, sweepInterval time.Duration) (*Aggregator, *fake.ClientBuilder) {
	cb := fake.NewClientBuilder().WithScheme(newTestScheme())
	fakeClient := cb.Build()
	agg := NewAggregator(
		fakeClient,
		"default",
		windowSize,
		sweepInterval,
		logr.Discard(),
	)
	return agg, cb
}

// waitForTasks polls the fake client until the expected number of DiagnosisTasks appear
// or the deadline is exceeded.
func waitForTasks(t *testing.T, agg *Aggregator, want int, deadline time.Duration) []kubemindsv1alpha1.DiagnosisTask {
	t.Helper()

	// Access the creator's client for listing.
	ctx := context.Background()
	end := time.Now().Add(deadline)

	for time.Now().Before(end) {
		var list kubemindsv1alpha1.DiagnosisTaskList
		if err := agg.creator.client.List(ctx, &list); err != nil {
			t.Fatalf("failed to list DiagnosisTasks: %v", err)
		}
		if len(list.Items) == want {
			return list.Items
		}
		time.Sleep(5 * time.Millisecond)
	}

	var list kubemindsv1alpha1.DiagnosisTaskList
	_ = agg.creator.client.List(ctx, &list)
	t.Fatalf("timed out waiting for %d DiagnosisTasks; got %d", want, len(list.Items))
	return nil
}

func TestAggregator_SingleAlert_CreatesOneTask(t *testing.T) {
	const window = 80 * time.Millisecond
	const sweep = 10 * time.Millisecond

	agg, _ := newTestAggregator(window, sweep)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go agg.Run(ctx)

	item := AlertItem{
		Status: "firing",
		Labels: map[string]string{
			"alertname": "KubePodCrashLooping",
			"namespace": "default",
			"pod":       "nginx-abc",
			"severity":  "critical",
		},
	}
	if err := agg.Ingest(item); err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}

	tasks := waitForTasks(t, agg, 1, 300*time.Millisecond)
	if tasks[0].Spec.AlertContext.Name != "KubePodCrashLooping" {
		t.Errorf("AlertContext.Name = %q, want %q", tasks[0].Spec.AlertContext.Name, "KubePodCrashLooping")
	}
}

func TestAggregator_DuplicateAlerts_Deduplicated(t *testing.T) {
	const window = 80 * time.Millisecond
	const sweep = 10 * time.Millisecond

	agg, _ := newTestAggregator(window, sweep)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go agg.Run(ctx)

	item := AlertItem{
		Status: "firing",
		Labels: map[string]string{
			"alertname": "KubePodCrashLooping",
			"namespace": "default",
			"pod":       "nginx-abc",
		},
	}

	// Ingest same alert 3 times.
	for i := 0; i < 3; i++ {
		if err := agg.Ingest(item); err != nil {
			t.Fatalf("Ingest() error: %v", err)
		}
	}

	// Expect exactly 1 DiagnosisTask after the window expires.
	waitForTasks(t, agg, 1, 300*time.Millisecond)

	// Verify only 1 group was active.
	if count := agg.GroupCount(); count != 0 {
		t.Errorf("GroupCount() = %d after flush, want 0", count)
	}
}

func TestAggregator_DifferentPods_SeparateTasks(t *testing.T) {
	const window = 80 * time.Millisecond
	const sweep = 10 * time.Millisecond

	agg, _ := newTestAggregator(window, sweep)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go agg.Run(ctx)

	for _, pod := range []string{"nginx-a", "nginx-b"} {
		item := AlertItem{
			Status: "firing",
			Labels: map[string]string{
				"alertname": "KubePodCrashLooping",
				"namespace": "default",
				"pod":       pod,
			},
		}
		if err := agg.Ingest(item); err != nil {
			t.Fatalf("Ingest() error: %v", err)
		}
	}

	// Expect 2 DiagnosisTasks.
	waitForTasks(t, agg, 2, 300*time.Millisecond)
}

func TestAggregator_LabelsAreMerged_LastWins(t *testing.T) {
	const window = 80 * time.Millisecond
	const sweep = 10 * time.Millisecond

	agg, _ := newTestAggregator(window, sweep)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go agg.Run(ctx)

	base := map[string]string{
		"alertname": "KubePodCrashLooping",
		"namespace": "default",
		"pod":       "nginx-abc",
	}

	// First alert: severity=warning.
	item1 := AlertItem{Status: "firing", Labels: copyMap(base)}
	item1.Labels["severity"] = "warning"
	if err := agg.Ingest(item1); err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}

	// Second alert: severity=critical (should win).
	item2 := AlertItem{Status: "firing", Labels: copyMap(base)}
	item2.Labels["severity"] = "critical"
	if err := agg.Ingest(item2); err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}

	tasks := waitForTasks(t, agg, 1, 300*time.Millisecond)
	got := tasks[0].Spec.AlertContext.Labels["severity"]
	if got != "critical" {
		t.Errorf("merged severity = %q, want %q", got, "critical")
	}
}

func TestAggregator_AlertWithinWindow_NoFlush(t *testing.T) {
	const window = 200 * time.Millisecond
	const sweep = 10 * time.Millisecond

	agg, _ := newTestAggregator(window, sweep)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go agg.Run(ctx)

	item := AlertItem{
		Status: "firing",
		Labels: map[string]string{
			"alertname": "KubePodCrashLooping",
			"namespace": "default",
			"pod":       "nginx-abc",
		},
	}
	if err := agg.Ingest(item); err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}

	// Wait only half the window — no flush should happen yet.
	time.Sleep(window / 2)

	var list kubemindsv1alpha1.DiagnosisTaskList
	if err := agg.creator.client.List(context.Background(), &list); err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(list.Items) != 0 {
		t.Errorf("expected 0 DiagnosisTasks before window expiry, got %d", len(list.Items))
	}
}

func TestAggregator_SlidingWindow_ResetOnNewAlert(t *testing.T) {
	const window = 120 * time.Millisecond
	const sweep = 10 * time.Millisecond

	agg, _ := newTestAggregator(window, sweep)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go agg.Run(ctx)

	item := AlertItem{
		Status: "firing",
		Labels: map[string]string{
			"alertname": "KubePodCrashLooping",
			"namespace": "default",
			"pod":       "nginx-abc",
		},
	}

	// Ingest at t=0 and then again at t=window/2 to reset the sliding window.
	if err := agg.Ingest(item); err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}
	time.Sleep(window / 2) // 60ms

	// Window should NOT have expired yet; a second ingest resets last_seen.
	if err := agg.Ingest(item); err != nil {
		t.Fatalf("Ingest() error: %v", err)
	}

	// Check at t=window (120ms from start): still within window because last_seen was reset.
	time.Sleep(window / 2) // another 60ms

	var list kubemindsv1alpha1.DiagnosisTaskList
	_ = agg.creator.client.List(context.Background(), &list)
	// Should still be 0 because the window was reset by the second Ingest.
	if len(list.Items) != 0 {
		t.Errorf("expected 0 DiagnosisTasks (window still open), got %d", len(list.Items))
	}

	// Now wait for the full window to expire.
	waitForTasks(t, agg, 1, 300*time.Millisecond)
}

func TestAggregator_ContextCancel_StopsSweep(t *testing.T) {
	const window = 50 * time.Millisecond
	const sweep = 10 * time.Millisecond

	agg, _ := newTestAggregator(window, sweep)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		agg.Run(ctx)
		close(done)
	}()

	// Cancel immediately.
	cancel()

	select {
	case <-done:
		// OK: Run() returned after ctx cancel.
	case <-time.After(200 * time.Millisecond):
		t.Error("Run() did not return after context cancel within deadline")
	}
}

// copyMap is a test helper that shallow-copies a map[string]string.
func copyMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
