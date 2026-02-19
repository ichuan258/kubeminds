package alert

import (
	"encoding/json"
	"net/http"

	"github.com/go-logr/logr"
)

// Handler receives AlertManager webhook payloads and feeds them to the Aggregator.
type Handler struct {
	aggregator *Aggregator
	log        logr.Logger
}

// NewHandler creates a new Handler.
func NewHandler(aggregator *Aggregator, log logr.Logger) *Handler {
	return &Handler{
		aggregator: aggregator,
		log:        log,
	}
}

// ServeWebhook handles POST /api/v1/alerts/webhook.
// It decodes the AlertManager v4 payload, filters out resolved alerts,
// and ingests each firing alert into the Aggregator.
// It always responds asynchronously (202 Accepted) on success.
func (h *Handler) ServeWebhook(w http.ResponseWriter, r *http.Request) {
	var payload AlertManagerPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.log.Error(err, "failed to decode AlertManager payload")
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	firing := 0
	for _, item := range payload.Alerts {
		if item.Status != "firing" {
			h.log.V(1).Info("skipping non-firing alert", "status", item.Status)
			continue
		}

		if err := h.aggregator.Ingest(item); err != nil {
			h.log.Error(err, "failed to ingest alert",
				"alertname", item.Labels["alertname"],
				"namespace", item.Labels["namespace"],
				"pod", item.Labels["pod"],
			)
			http.Error(w, "failed to ingest alert", http.StatusInternalServerError)
			return
		}
		firing++
	}

	h.log.Info("webhook received",
		"total", len(payload.Alerts),
		"firing", firing,
	)

	w.WriteHeader(http.StatusAccepted)
}
