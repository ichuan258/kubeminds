package alert

import (
	"strings"
	"time"
)

// AlertManagerPayload is the AlertManager v4 webhook payload format.
// See: https://prometheus.io/docs/alerting/latest/configuration/#webhook_config
type AlertManagerPayload struct {
	Version           string            `json:"version"`
	GroupKey          string            `json:"groupKey"`
	TruncatedAlerts   int               `json:"truncatedAlerts"`
	Status            string            `json:"status"`
	Receiver          string            `json:"receiver"`
	GroupLabels       map[string]string `json:"groupLabels"`
	CommonLabels      map[string]string `json:"commonLabels"`
	CommonAnnotations map[string]string `json:"commonAnnotations"`
	ExternalURL       string            `json:"externalURL"`
	Alerts            []AlertItem       `json:"alerts"`
}

// AlertItem represents a single alert within the payload.
type AlertItem struct {
	Status       string            `json:"status"` // "firing" | "resolved"
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       time.Time         `json:"endsAt"`
	GeneratorURL string            `json:"generatorURL"`
	Fingerprint  string            `json:"fingerprint"`
}

// GroupKey uniquely identifies a group of related alerts.
// Format: "<alertname>/<namespace>/<pod>"
// Missing fields are represented as "_".
type GroupKey string

// AlertGroup holds alerts with the same GroupKey within an aggregation window.
type AlertGroup struct {
	Key          GroupKey
	MergedLabels map[string]string // labels merge: later alerts overwrite earlier ones
	AlertName    string
	Namespace    string
	Pod          string // empty for non-pod-level alerts
	FirstSeen    time.Time
	LastSeen     time.Time // used for last_seen sliding window expiry
	Count        int
}

// buildGroupKey constructs a GroupKey from alert labels.
// Uses alertname + namespace + pod as the three-tuple key.
// Missing fields default to "_" to avoid ambiguity.
func buildGroupKey(labels map[string]string) GroupKey {
	alertname := labels["alertname"]
	namespace := labels["namespace"]
	pod := labels["pod"]

	if alertname == "" {
		alertname = "unknown"
	}
	if namespace == "" {
		namespace = "_"
	}
	if pod == "" {
		pod = "_"
	}

	return GroupKey(alertname + "/" + namespace + "/" + pod)
}

// sanitizeName converts an arbitrary string into a valid K8s resource name segment.
// Replaces non-alphanumeric characters with "-", lowercases, and truncates to maxLen.
func sanitizeName(s string, maxLen int) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	result := strings.Trim(b.String(), "-")
	if len(result) > maxLen {
		result = result[:maxLen]
		result = strings.TrimRight(result, "-")
	}
	return result
}
