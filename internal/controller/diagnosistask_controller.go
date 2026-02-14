/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubemindsv1alpha1 "kubeminds/api/v1alpha1"
	"kubeminds/internal/agent"
	"kubeminds/internal/llm"
	"kubeminds/internal/tools"
)

// DiagnosisTaskReconciler reconciles a DiagnosisTask object
type DiagnosisTaskReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	K8sClient kubernetes.Interface
	APIKey    string
	Model     string
	BaseURL   string

	// ProviderFactory allows injecting a custom LLM provider (e.g., for testing)
	ProviderFactory func(apiKey, model, baseUrl string) agent.LLMProvider

	// ActiveAgents tracks running agents to prevent duplicate execution and enable cancellation
	ActiveAgents sync.Map // map[string]context.CancelFunc

	// SkillManager manages available skills
	SkillManager *agent.SkillManager
}

// +kubebuilder:rbac:groups=kubeminds.io,resources=diagnosistasks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kubeminds.io,resources=diagnosistasks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kubeminds.io,resources=diagnosistasks/finalizers,verbs=update

func (r *DiagnosisTaskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := slog.Default().With("diagnosistask", req.NamespacedName)

	// Initialize SkillManager if nil (lazy init)
	if r.SkillManager == nil {
		r.SkillManager = agent.NewSkillManager()
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
		// Create cancelable context
		agentCtx, cancel := context.WithCancel(context.Background())
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

		// Start agent in a goroutine
		go func() {
			defer r.ActiveAgents.Delete(req.NamespacedName.String())
			defer cancel()

			// Initialize tools
			agentTools := []agent.Tool{
				tools.NewGetPodLogsTool(r.K8sClient),
				tools.NewGetPodEventsTool(r.K8sClient),
				tools.NewGetPodSpecTool(r.K8sClient),
			}

			// Initialize LLM
			var llmProvider agent.LLMProvider
			if r.ProviderFactory != nil {
				llmProvider = r.ProviderFactory(r.APIKey, r.Model, r.BaseURL)
			} else {
				llmProvider = llm.NewOpenAIProvider(r.APIKey, r.Model, r.BaseURL)
			}

			// Define Checkpoint Callback
			onStepComplete := func(finding kubemindsv1alpha1.Finding) {
				updateCtx := context.Background()

				var latestTask kubemindsv1alpha1.DiagnosisTask
				if err := r.Get(updateCtx, req.NamespacedName, &latestTask); err != nil {
					log.Error("Failed to get task for checkpoint update", "error", err)
					return
				}

				latestTask.Status.Checkpoint = append(latestTask.Status.Checkpoint, finding)
				if err := r.Status().Update(updateCtx, &latestTask); err != nil {
					log.Error("Failed to update checkpoint", "error", err)
				}
			}

			// Match Skill
			skill := r.SkillManager.Match(&task)
			log.Info("Matched skill", "skill", skill.Name)

			// Create Agent with Skill
			ag := agent.NewAgent(llmProvider, agentTools, task.Spec.Policy.MaxSteps, log, onStepComplete, skill)

			// Restore from checkpoint if available
			if len(task.Status.Checkpoint) > 0 {
				ag.Restore(task.Status.Checkpoint)
			}

			// Formulate Goal
			goal := fmt.Sprintf("Diagnose the issue with %s %s in namespace %s.",
				task.Spec.Target.Kind, task.Spec.Target.Name, task.Spec.Target.Namespace)

			// Run Agent
			result, err := ag.Run(agentCtx, goal)

			// Update CRD Status with result
			updateCtx := context.Background()
			var latestTask kubemindsv1alpha1.DiagnosisTask
			if err := r.Get(updateCtx, req.NamespacedName, &latestTask); err != nil {
				log.Error("Failed to get latest task for update", "error", err)
				return
			}

			if err != nil {
				latestTask.Status.Phase = kubemindsv1alpha1.PhaseFailed
				latestTask.Status.Report = &kubemindsv1alpha1.DiagnosisReport{
					RootCause:  "Agent execution failed",
					Suggestion: err.Error(),
				}
			} else {
				latestTask.Status.Phase = kubemindsv1alpha1.PhaseCompleted
				latestTask.Status.Report = &kubemindsv1alpha1.DiagnosisReport{
					RootCause:  result.RootCause,
					Suggestion: result.Suggestion,
				}
			}

			if err := r.Status().Update(updateCtx, &latestTask); err != nil {
				log.Error("Failed to update status with result", "error", err)
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
