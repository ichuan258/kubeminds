package alert

import (
	"context"
	"fmt"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubemindsv1alpha1 "kubeminds/api/v1alpha1"
)

// DiagnosisTaskCreator converts AlertGroups into DiagnosisTask CRDs.
type DiagnosisTaskCreator struct {
	client    client.Client
	namespace string // target namespace for created DiagnosisTasks
}

// NewDiagnosisTaskCreator creates a new DiagnosisTaskCreator.
func NewDiagnosisTaskCreator(k8sClient client.Client, namespace string) *DiagnosisTaskCreator {
	return &DiagnosisTaskCreator{
		client:    k8sClient,
		namespace: namespace,
	}
}

// Create converts an AlertGroup into a DiagnosisTask and creates it via the K8s API.
// It is idempotent: an AlreadyExists error is treated as success.
func (c *DiagnosisTaskCreator) Create(ctx context.Context, group *AlertGroup) error {
	task := c.buildTask(group)

	if err := c.client.Create(ctx, task); err != nil {
		if k8serrors.IsAlreadyExists(err) {
			return nil
		}
		return fmt.Errorf("failed to create DiagnosisTask for alert group %s: %w", group.Key, err)
	}

	return nil
}

// buildTask maps an AlertGroup to a DiagnosisTask.
func (c *DiagnosisTaskCreator) buildTask(group *AlertGroup) *kubemindsv1alpha1.DiagnosisTask {
	name := c.buildTaskName(group.AlertName)

	target := c.buildTarget(group)

	// Copy merged labels to avoid sharing the map reference.
	labelsCopy := make(map[string]string, len(group.MergedLabels))
	for k, v := range group.MergedLabels {
		labelsCopy[k] = v
	}

	return &kubemindsv1alpha1.DiagnosisTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: c.namespace,
		},
		Spec: kubemindsv1alpha1.DiagnosisTaskSpec{
			Target: target,
			AlertContext: &kubemindsv1alpha1.AlertContext{
				Name:   group.AlertName,
				Labels: labelsCopy,
			},
		},
	}
}

// buildTarget derives the DiagnosisTarget from the AlertGroup.
// If the group has a pod label, the target is pod-level; otherwise namespace-level.
func (c *DiagnosisTaskCreator) buildTarget(group *AlertGroup) kubemindsv1alpha1.DiagnosisTarget {
	if group.Pod != "" {
		return kubemindsv1alpha1.DiagnosisTarget{
			Namespace: group.Namespace,
			Name:      group.Pod,
			Kind:      "Pod",
		}
	}

	ns := group.Namespace
	if ns == "" {
		ns = c.namespace
	}
	return kubemindsv1alpha1.DiagnosisTarget{
		Namespace: ns,
		Name:      ns,
		Kind:      "Namespace",
	}
}

// buildTaskName generates a unique, K8s-valid resource name for the DiagnosisTask.
// Format: "alert-<sanitized-alertname>-<unix-ms>"
func (c *DiagnosisTaskCreator) buildTaskName(alertName string) string {
	const maxAlertSegment = 40
	safe := sanitizeName(alertName, maxAlertSegment)
	if safe == "" {
		safe = "unknown"
	}
	return fmt.Sprintf("alert-%s-%d", safe, time.Now().UnixMilli())
}
