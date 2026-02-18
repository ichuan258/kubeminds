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
	"kubeminds/internal/tools"
)

// Server is the REST API server
type Server struct {
	client       client.Client
	k8sClient    kubernetes.Interface
	skillManager *agent.SkillManager
	port         int
	log          logr.Logger
}

// NewServer creates a new API server
func NewServer(client client.Client, k8sClient kubernetes.Interface, skillManager *agent.SkillManager, port int, log logr.Logger) *Server {
	return &Server{
		client:       client,
		k8sClient:    k8sClient,
		skillManager: skillManager,
		port:         port,
		log:          log,
	}
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

	// Skills (MVP: Mocked)
	v1.HandleFunc("/skills", s.listSkills).Methods("GET")

	// Config (MVP: Mocked)
	v1.HandleFunc("/config/tools", s.getToolConfig).Methods("GET")

	// Health check
	r.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	addr := fmt.Sprintf(":%d", s.port)
	s.log.Info("listening", "address", addr)
	return http.ListenAndServe(addr, r)
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
	availableTools := tools.ListTools(s.k8sClient)
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

// --- Helpers ---

func respondJSON(w http.ResponseWriter, status int, payload interface{}) {
	response, err := json.Marshal(payload)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(response)
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
