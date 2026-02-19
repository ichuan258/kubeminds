package alert

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"kubeminds/internal/agent"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Aggregator deduplicates and merges incoming alerts within a sliding time window,
// then creates a single DiagnosisTask per group when the window expires.
type Aggregator struct {
	mu            sync.Mutex
	groups        map[GroupKey]*AlertGroup
	windowSize    time.Duration
	sweepInterval time.Duration
	creator       *DiagnosisTaskCreator
	log           logr.Logger

	// l2Store is an optional L2 event store. When non-nil, each flushed alert
	// group is written as an AlertEvent so the Agent can query recent context.
	l2Store agent.EventStore
}

// NewAggregator constructs an Aggregator. All dependencies are injected; no global state.
func NewAggregator(
	k8sClient client.Client,
	targetNamespace string,
	windowSize time.Duration,
	sweepInterval time.Duration,
	log logr.Logger,
) *Aggregator {
	return &Aggregator{
		groups:        make(map[GroupKey]*AlertGroup),
		windowSize:    windowSize,
		sweepInterval: sweepInterval,
		creator:       NewDiagnosisTaskCreator(k8sClient, targetNamespace),
		log:           log,
	}
}

// WithL2Store attaches an optional L2 EventStore. Call before Run().
// When set, each flushed AlertGroup is written to the event stream asynchronously.
func (a *Aggregator) WithL2Store(store agent.EventStore) *Aggregator {
	a.l2Store = store
	return a
}

// Run starts the background sweep goroutine. It blocks until ctx is cancelled.
// The caller is responsible for managing the goroutine lifecycle (e.g. via errgroup).
func (a *Aggregator) Run(ctx context.Context) {
	ticker := time.NewTicker(a.sweepInterval)
	defer ticker.Stop()

	a.log.Info("alert aggregator started",
		"windowSize", a.windowSize,
		"sweepInterval", a.sweepInterval,
	)

	for {
		select {
		case <-ctx.Done():
			a.log.Info("alert aggregator stopped")
			return
		case <-ticker.C:
			a.sweep(ctx)
		}
	}
}

// Ingest accepts a single AlertItem and adds it to the appropriate group.
// It is thread-safe and performs no I/O.
func (a *Aggregator) Ingest(item AlertItem) error {
	key := buildGroupKey(item.Labels)
	now := time.Now()

	a.mu.Lock()
	defer a.mu.Unlock()

	group, exists := a.groups[key]
	if !exists {
		group = &AlertGroup{
			Key:          key,
			MergedLabels: make(map[string]string),
			AlertName:    item.Labels["alertname"],
			Namespace:    item.Labels["namespace"],
			Pod:          item.Labels["pod"],
			FirstSeen:    now,
		}
		a.groups[key] = group
	}

	// Merge labels: later alerts overwrite earlier ones.
	for k, v := range item.Labels {
		group.MergedLabels[k] = v
	}

	// Update sliding window anchor and counter.
	group.LastSeen = now
	group.Count++

	a.log.V(1).Info("alert ingested",
		"key", string(key),
		"count", group.Count,
	)

	return nil
}

// GroupCount returns the number of active alert groups. Used for observability and tests.
func (a *Aggregator) GroupCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.groups)
}

// sweep checks all groups for expiry and flushes those whose last_seen exceeds windowSize.
// K8s API calls happen outside the lock to avoid blocking Ingest.
func (a *Aggregator) sweep(ctx context.Context) {
	now := time.Now()

	// Collect expired groups and remove them from the map while holding the lock.
	var expired []*AlertGroup

	a.mu.Lock()
	for key, group := range a.groups {
		if now.Sub(group.LastSeen) > a.windowSize {
			expired = append(expired, group)
			delete(a.groups, key)
		}
	}
	a.mu.Unlock()

	// Flush each expired group outside the lock.
	for _, group := range expired {
		if err := a.flush(ctx, group); err != nil {
			a.log.Error(err, "failed to flush alert group",
				"key", string(group.Key),
				"alertName", group.AlertName,
				"count", group.Count,
			)
		}
	}
}

// flush creates a DiagnosisTask for the given expired AlertGroup.
func (a *Aggregator) flush(ctx context.Context, group *AlertGroup) error {
	a.log.Info("flushing alert group",
		"key", string(group.Key),
		"alertName", group.AlertName,
		"count", group.Count,
		"firstSeen", group.FirstSeen,
		"lastSeen", group.LastSeen,
	)

	if err := a.creator.Create(ctx, group); err != nil {
		return fmt.Errorf("flush alert group %s: %w", group.Key, err)
	}

	a.log.Info("DiagnosisTask created for alert group",
		"key", string(group.Key),
		"alertName", group.AlertName,
	)

	// Write to L2 event store asynchronously so K8s task creation is never blocked.
	if a.l2Store != nil {
		event := agent.AlertEvent{
			AlertName: group.AlertName,
			Namespace: group.Namespace,
			Pod:       group.Pod,
			Count:     group.Count,
			FirstSeen: group.FirstSeen,
			LastSeen:  group.LastSeen,
		}
		go func(ev agent.AlertEvent) {
			if err := a.l2Store.AppendAlertEvent(context.Background(), ev); err != nil {
				a.log.Error(err, "l2: failed to append alert event", "alertName", ev.AlertName)
			}
		}(event)
	}

	return nil
}
