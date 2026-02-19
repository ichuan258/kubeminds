package alert

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newTestHandler() (*Handler, *Aggregator) {
	fakeClient := fake.NewClientBuilder().WithScheme(newTestScheme()).Build()
	agg := NewAggregator(
		fakeClient,
		"default",
		60*time.Second, // long window; tests don't need flush
		5*time.Second,
		logr.Discard(),
	)
	h := NewHandler(agg, logr.Discard())
	return h, agg
}

func postWebhook(t *testing.T, h *Handler, payload interface{}) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeWebhook(w, req)
	return w
}

func TestHandler_FiringAlerts_202(t *testing.T) {
	h, agg := newTestHandler()

	payload := AlertManagerPayload{
		Alerts: []AlertItem{
			{
				Status: "firing",
				Labels: map[string]string{
					"alertname": "KubePodCrashLooping",
					"namespace": "default",
					"pod":       "nginx-abc",
				},
			},
		},
	}

	w := postWebhook(t, h, payload)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202", w.Code)
	}
	if agg.GroupCount() != 1 {
		t.Errorf("GroupCount() = %d, want 1", agg.GroupCount())
	}
}

func TestHandler_ResolvedAlerts_Filtered(t *testing.T) {
	h, agg := newTestHandler()

	payload := AlertManagerPayload{
		Alerts: []AlertItem{
			{
				Status: "resolved",
				Labels: map[string]string{
					"alertname": "KubePodCrashLooping",
					"namespace": "default",
					"pod":       "nginx-abc",
				},
			},
		},
	}

	w := postWebhook(t, h, payload)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202", w.Code)
	}
	if agg.GroupCount() != 0 {
		t.Errorf("GroupCount() = %d, want 0 (resolved should be ignored)", agg.GroupCount())
	}
}

func TestHandler_MixedPayload_OnlyFiringIngested(t *testing.T) {
	h, agg := newTestHandler()

	payload := AlertManagerPayload{
		Alerts: []AlertItem{
			{
				Status: "firing",
				Labels: map[string]string{"alertname": "OOM", "namespace": "prod", "pod": "app-1"},
			},
			{
				Status: "resolved",
				Labels: map[string]string{"alertname": "OOM", "namespace": "prod", "pod": "app-2"},
			},
			{
				Status: "firing",
				Labels: map[string]string{"alertname": "OOM", "namespace": "prod", "pod": "app-3"},
			},
		},
	}

	w := postWebhook(t, h, payload)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202", w.Code)
	}
	// Only 2 firing alerts → 2 groups (different pods).
	if agg.GroupCount() != 2 {
		t.Errorf("GroupCount() = %d, want 2", agg.GroupCount())
	}
}

func TestHandler_InvalidJSON_400(t *testing.T) {
	h, _ := newTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/webhook", bytes.NewBufferString(`{not valid json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeWebhook(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandler_EmptyAlerts_202(t *testing.T) {
	h, agg := newTestHandler()

	payload := AlertManagerPayload{
		Alerts: []AlertItem{},
	}

	w := postWebhook(t, h, payload)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202", w.Code)
	}
	if agg.GroupCount() != 0 {
		t.Errorf("GroupCount() = %d, want 0", agg.GroupCount())
	}
}
