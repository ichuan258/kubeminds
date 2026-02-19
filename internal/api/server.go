package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubemindsv1alpha1 "kubeminds/api/v1alpha1"
	"kubeminds/internal/agent"
	"kubeminds/internal/alert"
	"kubeminds/internal/llm"
	"kubeminds/internal/tools"
)

// Server is the REST API server
type Server struct {
	client       client.Client
	k8sClient    kubernetes.Interface
	skillManager *agent.SkillManager
	toolRouter   *tools.Router  // Unified tool router
	alertHandler *alert.Handler // nil when alert webhook is not configured
	llmRouter    *llm.Router    // nil when LLM is not configured (e.g. mock-only mode)
	port         int
	log          logr.Logger
}

// NewServer creates a new API server
func NewServer(client client.Client, k8sClient kubernetes.Interface, skillManager *agent.SkillManager, toolRouter *tools.Router, port int, log logr.Logger) *Server {
	return &Server{
		client:       client,
		k8sClient:    k8sClient,
		skillManager: skillManager,
		toolRouter:   toolRouter,
		port:         port,
		log:          log,
	}
}

// WithLLMRouter attaches an LLM router to the server, enabling the /api/v1/llm/ping endpoint.
func (s *Server) WithLLMRouter(r *llm.Router) *Server {
	s.llmRouter = r
	return s
}

// WithAlertHandler attaches an alert webhook handler to the server.
// When set, POST /api/v1/alerts/webhook is registered as a route.
func (s *Server) WithAlertHandler(h *alert.Handler) *Server {
	s.alertHandler = h
	return s
}

// Start starts the API server
func (s *Server) Start() error {
	r := mux.NewRouter()
	r.Use(loggingMiddleware(s.log))

	// API Routes
	v1 := r.PathPrefix("/api/v1").Subrouter()

	// Diagnosis Tasks
	v1.HandleFunc("/tasks", s.listTasks).Methods("GET")
	v1.HandleFunc("/tasks", s.createTask).Methods("POST")
	v1.HandleFunc("/tasks/{namespace}/{name}", s.getTask).Methods("GET")
	v1.HandleFunc("/tasks/{namespace}/{name}", s.deleteTask).Methods("DELETE")
	v1.HandleFunc("/tasks/{namespace}/{name}/approve", s.approveTask).Methods("POST")

	// Alert Aggregator webhook
	if s.alertHandler != nil {
		v1.HandleFunc("/alerts/webhook", s.alertHandler.ServeWebhook).Methods("POST")
	}

	// Skills (MVP: Mocked)
	v1.HandleFunc("/skills", s.listSkills).Methods("GET")

	// Config (MVP: Mocked)
	v1.HandleFunc("/config/tools", s.getToolConfig).Methods("GET")

	// LLM connectivity test
	v1.HandleFunc("/llm/ping", s.pingLLM).Methods("POST")

	// Health check
	r.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	addr := fmt.Sprintf(":%d", s.port)
	s.log.Info("listening", "address", addr)
	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return srv.ListenAndServe()
}

// --- Handlers ---

// List Tasks
func (s *Server) listTasks(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	var list kubemindsv1alpha1.DiagnosisTaskList

	// TODO: Support filtering by namespace and status via query params
	opts := []client.ListOption{}
	if ns := r.URL.Query().Get("namespace"); ns != "" {
		opts = append(opts, client.InNamespace(ns))
	}

	if err := s.client.List(ctx, &list, opts...); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"items": list.Items,
		"total": len(list.Items),
	})
}

// Create Task
func (s *Server) createTask(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	var task kubemindsv1alpha1.DiagnosisTask
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Set defaults
	if task.Namespace == "" {
		task.Namespace = "default"
	}
	if task.Name == "" {
		task.Name = fmt.Sprintf("manual-%d", time.Now().Unix())
	}
	task.Status.Phase = kubemindsv1alpha1.PhasePending

	if err := s.client.Create(ctx, &task); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusCreated, task)
}

// Get Task
func (s *Server) getTask(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	vars := mux.Vars(r)
	ns := vars["namespace"]
	name := vars["name"]

	var task kubemindsv1alpha1.DiagnosisTask
	if err := s.client.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &task); err != nil {
		if errors.IsNotFound(err) {
			http.Error(w, "task not found", http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	respondJSON(w, http.StatusOK, task)
}

// Approve Task
func (s *Server) approveTask(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	vars := mux.Vars(r)
	ns := vars["namespace"]
	name := vars["name"]

	var task kubemindsv1alpha1.DiagnosisTask
	if err := s.client.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &task); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Update approved status
	task.Spec.Approved = true
	// Ensure we are in waiting state before moving?
	// For MVP, simplistic update.

	if err := s.client.Update(ctx, &task); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, task)
}

// Delete Task
func (s *Server) deleteTask(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	vars := mux.Vars(r)
	ns := vars["namespace"]
	name := vars["name"]

	task := kubemindsv1alpha1.DiagnosisTask{}
	task.Name = name
	task.Namespace = ns

	if err := s.client.Delete(ctx, &task); err != nil {
		if errors.IsNotFound(err) {
			http.Error(w, "task not found", http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// List Skills
func (s *Server) listSkills(w http.ResponseWriter, r *http.Request) {
	if s.skillManager == nil {
		respondJSON(w, http.StatusOK, map[string]interface{}{"items": []interface{}{}})
		return
	}
	skills := s.skillManager.ListSkills()
	respondJSON(w, http.StatusOK, map[string]interface{}{"items": skills})
}

// Get Tool Config
func (s *Server) getToolConfig(w http.ResponseWriter, r *http.Request) {
	// For MVP, we list available internal tools.
	// Later we can add MCP servers and safety policies from config.
	var availableTools []agent.Tool
	if s.toolRouter != nil {
		var err error
		availableTools, err = s.toolRouter.ListTools(r.Context())
		if err != nil {
			s.log.Error(err, "failed to list tools")
			http.Error(w, "failed to list tools", http.StatusInternalServerError)
			return
		}
	} else {
		// Fallback for tests or if router is not provided
		availableTools = tools.ListTools(s.k8sClient)
	}

	toolConfigs := make([]map[string]string, 0, len(availableTools))
	for _, t := range availableTools {
		toolConfigs = append(toolConfigs, map[string]string{
			"name":        t.Name(),
			"description": t.Description(),
		})
	}

	config := map[string]interface{}{
		"tools": toolConfigs,
		"mcp_servers": []map[string]string{
			{"name": "slack", "status": "connected"},
		},
		"safety_policies": map[string]string{
			"delete_pod":         "Forbidden",
			"restart_deployment": "HighRisk",
			"get_pod_logs":       "ReadOnly",
		},
	}
	respondJSON(w, http.StatusOK, config)
}

// pingLLM tests connectivity to the configured LLM provider.
//
// POST /api/v1/llm/ping
//
// Request body (optional JSON):
//
//	{"provider": "gemini"}   // omit to test the default provider
//
// Response:
//
//	{"provider":"openai","model":"gpt-4o","status":"ok","latency_ms":342}
//	{"provider":"openai","model":"gpt-4o","status":"error","error":"401 Unauthorized"}
func (s *Server) pingLLM(w http.ResponseWriter, r *http.Request) {
	if s.llmRouter == nil {
		http.Error(w, "LLM provider not configured", http.StatusServiceUnavailable)
		return
	}

	// Optional body: {"provider": "..."} — ignored for now since we always use defaultProvider.
	// Kept for future extensibility when per-provider ping is needed.

	// Send a minimal chat message that requires only a short response.
	// Using a fixed timeout to avoid hanging the health check indefinitely.
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	start := time.Now()
	_, err := s.llmRouter.Chat(ctx, []agent.Message{
		{
			Type:    agent.MessageTypeUser,
			Content: "Reply with 'pong' only.",
		},
	}, nil)
	latencyMs := time.Since(start).Milliseconds()

	type pingResponse struct {
		Provider  string `json:"provider"`
		Status    string `json:"status"`
		LatencyMs int64  `json:"latency_ms"`
		Error     string `json:"error,omitempty"`
	}

	resp := pingResponse{
		Provider:  s.llmRouter.DefaultProvider(),
		LatencyMs: latencyMs,
	}

	if err != nil {
		resp.Status = "error"
		resp.Error = err.Error()
		respondJSON(w, http.StatusOK, resp) // return 200 with error body, not 5xx
		return
	}

	resp.Status = "ok"
	respondJSON(w, http.StatusOK, resp)
}

// --- Helpers ---

func respondJSON(w http.ResponseWriter, status int, payload interface{}) {
	response, err := json.Marshal(payload)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(response)
}

func loggingMiddleware(log logr.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, r)
			log.Info("request",
				"method", r.Method,
				"uri", r.RequestURI,
				"duration", time.Since(start),
			)
		})
	}
}
