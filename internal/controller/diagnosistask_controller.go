package controller

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubemindsv1alpha1 "kubeminds/api/v1alpha1"
	"kubeminds/internal/agent"
	"kubeminds/internal/tools"
)

// DiagnosisTaskReconciler reconciles a DiagnosisTask object
type DiagnosisTaskReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	K8sClient    kubernetes.Interface
	SkillDir     string
	AgentTimeout time.Duration

	// LLMProvider is the LLM backend used by every agent spawned by this controller.
	// Inject llm.NewRouterFromConfig(cfg.LLM) at startup, or llm.NewMockProvider() for tests.
	LLMProvider agent.LLMProvider

	// ActiveAgents tracks running agents to prevent duplicate execution and enable cancellation
	ActiveAgents sync.Map // map[string]context.CancelFunc

	// SkillManager manages available skills
	SkillManager *agent.SkillManager

	// ToolRouter manages available tools
	ToolRouter *tools.Router

	// L2Store is an optional L2 event store. When non-nil, recent alert events for
	// the target namespace are injected into the agent's context before each run.
	L2Store agent.EventStore

	// KnowledgeBase is an optional L3 knowledge base. When non-nil, similar historical
	// diagnoses are retrieved and injected before each run, and completed diagnoses are
	// saved asynchronously after each successful run.
	KnowledgeBase agent.KnowledgeBase

	// Embedder is required when KnowledgeBase is set. It generates the embedding vectors
	// used for semantic search and storage.
	Embedder agent.EmbeddingProvider
}

// +kubebuilder:rbac:groups=kubeminds.io,resources=diagnosistasks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kubeminds.io,resources=diagnosistasks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kubeminds.io,resources=diagnosistasks/finalizers,verbs=update

func (r *DiagnosisTaskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := slog.Default().With("diagnosistask", req.NamespacedName)

	// Initialize SkillManager if nil (lazy init)
	if r.SkillManager == nil {
		sm, err := agent.NewSkillManager(r.SkillDir, log)
		if err != nil {
			log.Error("Failed to initialize SkillManager", "error", err)
			// Return error to retry later (e.g. if file system is temporarily unavailable)
			return ctrl.Result{RequeueAfter: 10 * time.Second}, err
		}
		r.SkillManager = sm
	}

	// Fetch the DiagnosisTask instance
	var task kubemindsv1alpha1.DiagnosisTask
	if err := r.Get(ctx, req.NamespacedName, &task); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion/cleanup
	if !task.ObjectMeta.DeletionTimestamp.IsZero() ||
		task.Status.Phase == kubemindsv1alpha1.PhaseCompleted ||
		task.Status.Phase == kubemindsv1alpha1.PhaseFailed {
		if cancel, ok := r.ActiveAgents.Load(req.NamespacedName.String()); ok {
			log.Info("Stopping active agent")
			cancel.(context.CancelFunc)()
			r.ActiveAgents.Delete(req.NamespacedName.String())
		}
		return ctrl.Result{}, nil
	}

	// If status phase is empty, set it to Pending
	if task.Status.Phase == "" {
		task.Status.Phase = kubemindsv1alpha1.PhasePending
		if err := r.Status().Update(ctx, &task); err != nil {
			log.Error("Failed to update status to Pending", "error", err)
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Check if agent is already running locally
	if _, loaded := r.ActiveAgents.Load(req.NamespacedName.String()); loaded {
		return ctrl.Result{}, nil
	}

	// Handle WaitingApproval: check if human has approved before resuming
	if task.Status.Phase == kubemindsv1alpha1.PhaseWaitingApproval {
		if task.Spec.Approved {
			log.Info("Task approved by human, transitioning to Running")
			task.Status.Phase = kubemindsv1alpha1.PhaseRunning
			if err := r.Status().Update(ctx, &task); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update phase to Running after approval: %w", err)
			}
			return ctrl.Result{Requeue: true}, nil
		}
		// Not yet approved; wait for spec.approved to be set
		return ctrl.Result{}, nil
	}

	// Determine if we should start/resume
	shouldStart := false
	isResume := false

	if task.Status.Phase == kubemindsv1alpha1.PhasePending {
		shouldStart = true
	} else if task.Status.Phase == kubemindsv1alpha1.PhaseRunning {
		// It's Running in Status but not locally -> Resume!
		shouldStart = true
		isResume = true
		log.Info("Resuming interrupted task")
	}

	if shouldStart {
		// Create context with timeout to prevent agent goroutine from hanging indefinitely
		timeout := r.AgentTimeout
		if timeout == 0 {
			timeout = 10 * time.Minute
		}
		agentCtx, cancel := context.WithTimeout(context.Background(), timeout)
		r.ActiveAgents.Store(req.NamespacedName.String(), cancel)

		// Update status to Running if needed
		if !isResume {
			task.Status.Phase = kubemindsv1alpha1.PhaseRunning
			if err := r.Status().Update(ctx, &task); err != nil {
				log.Error("Failed to update status to Running", "error", err)
				cancel()
				r.ActiveAgents.Delete(req.NamespacedName.String())
				return ctrl.Result{}, err
			}
		}

		// Start agent using errgroup for structured lifecycle management (CLAUDE.md §3.2)
		eg, agentCtx := errgroup.WithContext(agentCtx)
		eg.Go(func() error {
			defer r.ActiveAgents.Delete(req.NamespacedName.String())

			// Initialize tools from Router
			agentTools, err := r.ToolRouter.ListTools(agentCtx)
			if err != nil {
				log.Error("Failed to list tools", "error", err)
				return fmt.Errorf("failed to list tools: %w", err)
			}

			// Use the LLM provider injected at startup (Router or Mock).
			llmProvider := r.LLMProvider

			// Define Checkpoint Callback
			onStepComplete := func(finding *kubemindsv1alpha1.Finding, historyEntry string) {
				updateCtx := context.Background()

				var latestTask kubemindsv1alpha1.DiagnosisTask
				if err := r.Get(updateCtx, req.NamespacedName, &latestTask); err != nil {
					log.Error("Failed to get task for checkpoint update", "error", err)
					return
				}

				if finding != nil {
					latestTask.Status.Checkpoint = append(latestTask.Status.Checkpoint, *finding)
				}
				if historyEntry != "" {
					latestTask.Status.History = append(latestTask.Status.History, historyEntry)
				}

				if err := r.Status().Update(updateCtx, &latestTask); err != nil {
					log.Error("Failed to update task status", "error", err)
				}
			}

			// Match Skill
			skill := r.SkillManager.Match(&task)
			log.Info("Matched skill", "skill", skill.Name)

			// Update MatchedSkill in status
			updateCtx := context.Background()
			var currentTask kubemindsv1alpha1.DiagnosisTask
			if err := r.Get(updateCtx, req.NamespacedName, &currentTask); err == nil {
				// We need to fetch the latest version to update status
				currentTask.Status.MatchedSkill = skill.Name
				if err := r.Status().Update(updateCtx, &currentTask); err != nil {
					log.Error("Failed to update matched skill", "error", err)
				}
			}

			// Create Agent with Skill
			ag := agent.NewAgent(llmProvider, agentTools, task.Spec.Policy.MaxSteps, log, onStepComplete, skill)

			// Restore from checkpoint if available
			if len(task.Status.Checkpoint) > 0 {
				ag.Restore(task.Status.Checkpoint)
			}

			// Formulate Goal
			goal := fmt.Sprintf("Diagnose the issue with %s %s in namespace %s.",
				task.Spec.Target.Kind, task.Spec.Target.Name, task.Spec.Target.Namespace)

			// Inject L2 context: recent alert events for the same namespace.
			if r.L2Store != nil {
				events, err := r.L2Store.GetRecentEvents(agentCtx, task.Spec.Target.Namespace, task.Spec.Target.Name, 10)
				if err != nil {
					log.Info("l2: failed to fetch recent events (non-fatal)", "error", err)
				} else if formatted := agent.FormatAlertEvents(events); formatted != "" {
					ag.InjectContext(formatted)
				}
			}

			// Inject L3 context: historically similar diagnoses via semantic search.
			if r.KnowledgeBase != nil && r.Embedder != nil {
				alertName := ""
				if task.Spec.AlertContext != nil {
					alertName = task.Spec.AlertContext.Name
				}
				queryText := alertName + " " + task.Spec.Target.Namespace
				emb, err := r.Embedder.Embed(agentCtx, queryText)
				if err != nil {
					log.Info("l3: failed to generate query embedding (non-fatal)", "error", err)
				} else {
					historicals, err := r.KnowledgeBase.SearchSimilar(agentCtx, emb, 3)
					if err != nil {
						log.Info("l3: failed to search similar diagnoses (non-fatal)", "error", err)
					} else if formatted := agent.FormatHistoricalFindings(historicals); formatted != "" {
						ag.InjectContext(formatted)
					}
				}
			}

			// Run Agent
			result, err := ag.Run(agentCtx, goal, task.Spec.Approved)

			// Update CRD Status with result
			updateCtx = context.Background()
			var latestTask kubemindsv1alpha1.DiagnosisTask
			if err := r.Get(updateCtx, req.NamespacedName, &latestTask); err != nil {
				log.Error("Failed to get latest task for update", "error", err)
				return fmt.Errorf("failed to get latest task for status update: %w", err)
			}

			if err != nil {
				// Check for WaitingForApproval
				var waitingErr *agent.ErrWaitingForApproval
				if errors.As(err, &waitingErr) {
					log.Info("Agent requested approval", "tool", waitingErr.ToolName)
					latestTask.Status.Phase = kubemindsv1alpha1.PhaseWaitingApproval
					latestTask.Status.Message = fmt.Sprintf("Tool %s requires approval.", waitingErr.ToolName)
				} else {
					latestTask.Status.Phase = kubemindsv1alpha1.PhaseFailed
					latestTask.Status.Report = &kubemindsv1alpha1.DiagnosisReport{
						RootCause:  "Agent execution failed",
						Suggestion: err.Error(),
					}
				}
			} else {
				latestTask.Status.Phase = kubemindsv1alpha1.PhaseCompleted
				latestTask.Status.Report = &kubemindsv1alpha1.DiagnosisReport{
					RootCause:  result.RootCause,
					Suggestion: result.Suggestion,
				}

				// Save diagnosis to L3 knowledge base asynchronously.
				// This must not block the reconcile path or status update.
				if r.KnowledgeBase != nil && r.Embedder != nil {
					alertName := ""
					if latestTask.Spec.AlertContext != nil {
						alertName = latestTask.Spec.AlertContext.Name
					}
					finding := agent.KnowledgeFinding{
						AlertName:  alertName,
						Namespace:  latestTask.Spec.Target.Namespace,
						RootCause:  result.RootCause,
						Suggestion: result.Suggestion,
					}
					go func(f agent.KnowledgeFinding) {
						saveCtx := context.Background()
						text := f.RootCause + " " + f.Suggestion
						emb, err := r.Embedder.Embed(saveCtx, text)
						if err != nil {
							log.Error("l3: failed to generate embedding for completed diagnosis", "error", err)
							return
						}
						if err := r.KnowledgeBase.SaveDiagnosis(saveCtx, f, emb); err != nil {
							log.Error("l3: failed to save diagnosis to knowledge base", "error", err)
						}
					}(finding)
				}
			}

			if err := r.Status().Update(updateCtx, &latestTask); err != nil {
				log.Error("Failed to update status with result", "error", err)
			}
			return nil
		})

		// Wait for errgroup in a background goroutine so Reconcile returns immediately.
		// This outer goroutine is intentionally minimal: it only waits and logs.
		go func() {
			defer cancel()
			if err := eg.Wait(); err != nil {
				log.Error("Agent errgroup exited with error", "error", err)
			}
		}()
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DiagnosisTaskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubemindsv1alpha1.DiagnosisTask{}).
		Complete(r)
}
