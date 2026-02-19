package agent

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	l2StreamPrefix = "kubeminds:events:"
	l2StreamMaxLen = 500 // max entries per namespace stream (approximate MAXLEN)
)

// RedisEventStore implements EventStore using Redis Streams.
// Each namespace has its own stream at key "kubeminds:events:{namespace}".
// Entries older than eventTTL are automatically expired via Redis key TTL.
type RedisEventStore struct {
	client   *redis.Client
	eventTTL time.Duration
}

// NewRedisEventStore returns a RedisEventStore backed by the provided redis.Client.
func NewRedisEventStore(client *redis.Client, eventTTL time.Duration) *RedisEventStore {
	return &RedisEventStore{client: client, eventTTL: eventTTL}
}

// AppendAlertEvent writes an alert event to the Redis Stream for the event's namespace.
// The stream is capped at l2StreamMaxLen entries (approximate) and its TTL is refreshed.
func (s *RedisEventStore) AppendAlertEvent(ctx context.Context, event AlertEvent) error {
	key := l2StreamPrefix + event.Namespace

	args := &redis.XAddArgs{
		Stream: key,
		MaxLen: l2StreamMaxLen,
		Approx: true,
		Values: map[string]interface{}{
			"alert_name": event.AlertName,
			"namespace":  event.Namespace,
			"pod":        event.Pod,
			"count":      strconv.Itoa(event.Count),
			"first_seen": strconv.FormatInt(event.FirstSeen.Unix(), 10),
			"last_seen":  strconv.FormatInt(event.LastSeen.Unix(), 10),
		},
	}

	if err := s.client.XAdd(ctx, args).Err(); err != nil {
		return fmt.Errorf("l2: xadd to stream %s: %w", key, err)
	}

	// Refresh TTL so the stream expires if no new alerts arrive.
	// Errors are non-fatal; the stream will still be readable.
	_ = s.client.Expire(ctx, key, s.eventTTL).Err()

	return nil
}

// GetRecentEvents returns the most recent alert events for the given namespace from
// the Redis Stream. If pod is non-empty, results are filtered to that pod only.
// The returned slice is ordered newest-first.
func (s *RedisEventStore) GetRecentEvents(ctx context.Context, namespace, pod string, limit int) ([]AlertEvent, error) {
	key := l2StreamPrefix + namespace

	// Over-fetch to allow pod filtering without a second round-trip.
	fetchN := int64(limit)
	if pod != "" {
		fetchN = int64(limit * 4)
	}

	entries, err := s.client.XRevRangeN(ctx, key, "+", "-", fetchN).Result()
	if err != nil {
		return nil, fmt.Errorf("l2: xrevrange on stream %s: %w", key, err)
	}

	var events []AlertEvent
	for _, e := range entries {
		ev := parseL2StreamEntry(e)
		if pod == "" || ev.Pod == pod {
			events = append(events, ev)
		}
		if len(events) >= limit {
			break
		}
	}

	return events, nil
}

// parseL2StreamEntry converts a raw Redis XMessage into an AlertEvent.
func parseL2StreamEntry(e redis.XMessage) AlertEvent {
	str := func(k string) string {
		if v, ok := e.Values[k]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		return ""
	}
	parseInt := func(k string) int {
		n, _ := strconv.Atoi(str(k))
		return n
	}
	parseUnix := func(k string) time.Time {
		n, _ := strconv.ParseInt(str(k), 10, 64)
		if n == 0 {
			return time.Time{}
		}
		return time.Unix(n, 0)
	}

	return AlertEvent{
		AlertName: str("alert_name"),
		Namespace: str("namespace"),
		Pod:       str("pod"),
		Count:     parseInt("count"),
		FirstSeen: parseUnix("first_seen"),
		LastSeen:  parseUnix("last_seen"),
	}
}

// FormatAlertEvents formats a list of recent alert events as a human-readable
// string suitable for injection into the agent's LLM context.
func FormatAlertEvents(events []AlertEvent) string {
	if len(events) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Recent alert events in this namespace (from L2 event stream):\n")
	for _, e := range events {
		b.WriteString(fmt.Sprintf(
			"  - [%s] pod=%s count=%d last_seen=%s\n",
			e.AlertName, e.Pod, e.Count, e.LastSeen.Format(time.RFC3339),
		))
	}
	return b.String()
}
