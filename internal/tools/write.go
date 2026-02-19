package tools

import (
	"context"
	"encoding/json"
	"fmt"

	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"kubeminds/internal/agent"
)

type DeletePodArgs struct {
	Namespace string `json:"namespace"`
	PodName   string `json:"pod_name"`
}

type PatchDeploymentArgs struct {
	Namespace      string `json:"namespace"`
	DeploymentName string `json:"deployment_name"`
	PatchJSON      string `json:"patch_json"`
}

type ScaleStatefulSetArgs struct {
	Namespace       string `json:"namespace"`
	StatefulSetName string `json:"statefulset_name"`
	Replicas        int32  `json:"replicas"`
}

// DeletePodTool implements the delete_pod tool
type DeletePodTool struct {
	client kubernetes.Interface
}

func NewDeletePodTool(client kubernetes.Interface) *DeletePodTool {
	return &DeletePodTool{client: client}
}

func (t *DeletePodTool) Name() string {
	return "delete_pod"
}

func (t *DeletePodTool) Description() string {
	return "Delete a pod in a namespace. This is a high-risk operation and requires explicit approval. Use this to terminate stuck pods or force recovery."
}

func (t *DeletePodTool) Schema() string {
	return `{
		"type": "object",
		"properties": {
			"namespace": {
				"type": "string",
				"description": "The namespace of the pod"
			},
			"pod_name": {
				"type": "string",
				"description": "The name of the pod to delete"
			}
		},
		"required": ["namespace", "pod_name"]
	}`
}

func (t *DeletePodTool) SafetyLevel() agent.SafetyLevel {
	return agent.SafetyLevelHighRisk
}

func (t *DeletePodTool) Execute(ctx context.Context, args string) (string, error) {
	var parsedArgs DeletePodArgs
	if err := json.Unmarshal([]byte(args), &parsedArgs); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	err := t.client.CoreV1().Pods(parsedArgs.Namespace).Delete(ctx, parsedArgs.PodName, metav1.DeleteOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to delete pod: %w", err)
	}

	return fmt.Sprintf("Successfully deleted pod '%s' in namespace '%s'", parsedArgs.PodName, parsedArgs.Namespace), nil
}

// PatchDeploymentTool implements the patch_deployment tool
type PatchDeploymentTool struct {
	client kubernetes.Interface
}

func NewPatchDeploymentTool(client kubernetes.Interface) *PatchDeploymentTool {
	return &PatchDeploymentTool{client: client}
}

func (t *PatchDeploymentTool) Name() string {
	return "patch_deployment"
}

func (t *PatchDeploymentTool) Description() string {
	return "Apply a JSON merge patch to a deployment. This is a high-risk operation and requires explicit approval. Use this to update deployment specs like image, replicas, or environment variables."
}

func (t *PatchDeploymentTool) Schema() string {
	return `{
		"type": "object",
		"properties": {
			"namespace": {
				"type": "string",
				"description": "The namespace of the deployment"
			},
			"deployment_name": {
				"type": "string",
				"description": "The name of the deployment to patch"
			},
			"patch_json": {
				"type": "string",
				"description": "The JSON merge patch to apply (raw JSON string)"
			}
		},
		"required": ["namespace", "deployment_name", "patch_json"]
	}`
}

func (t *PatchDeploymentTool) SafetyLevel() agent.SafetyLevel {
	return agent.SafetyLevelHighRisk
}

func (t *PatchDeploymentTool) Execute(ctx context.Context, args string) (string, error) {
	var parsedArgs PatchDeploymentArgs
	if err := json.Unmarshal([]byte(args), &parsedArgs); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	patchBytes := []byte(parsedArgs.PatchJSON)
	_, err := t.client.AppsV1().Deployments(parsedArgs.Namespace).Patch(ctx, parsedArgs.DeploymentName, types.MergePatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to patch deployment: %w", err)
	}

	return fmt.Sprintf("Successfully patched deployment '%s' in namespace '%s'", parsedArgs.DeploymentName, parsedArgs.Namespace), nil
}

// ScaleStatefulSetTool implements the scale_statefulset tool
type ScaleStatefulSetTool struct {
	client kubernetes.Interface
}

func NewScaleStatefulSetTool(client kubernetes.Interface) *ScaleStatefulSetTool {
	return &ScaleStatefulSetTool{client: client}
}

func (t *ScaleStatefulSetTool) Name() string {
	return "scale_statefulset"
}

func (t *ScaleStatefulSetTool) Description() string {
	return "Scale a StatefulSet to a specified number of replicas. This is a high-risk operation and requires explicit approval. Use this to resize the StatefulSet for recovery or capacity adjustment."
}

func (t *ScaleStatefulSetTool) Schema() string {
	return `{
		"type": "object",
		"properties": {
			"namespace": {
				"type": "string",
				"description": "The namespace of the StatefulSet"
			},
			"statefulset_name": {
				"type": "string",
				"description": "The name of the StatefulSet to scale"
			},
			"replicas": {
				"type": "integer",
				"description": "The desired number of replicas"
			}
		},
		"required": ["namespace", "statefulset_name", "replicas"]
	}`
}

func (t *ScaleStatefulSetTool) SafetyLevel() agent.SafetyLevel {
	return agent.SafetyLevelHighRisk
}

func (t *ScaleStatefulSetTool) Execute(ctx context.Context, args string) (string, error) {
	var parsedArgs ScaleStatefulSetArgs
	if err := json.Unmarshal([]byte(args), &parsedArgs); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	scale := &autoscalingv1.Scale{
		ObjectMeta: metav1.ObjectMeta{
			Name:      parsedArgs.StatefulSetName,
			Namespace: parsedArgs.Namespace,
		},
		Spec: autoscalingv1.ScaleSpec{
			Replicas: parsedArgs.Replicas,
		},
	}

	_, err := t.client.AppsV1().StatefulSets(parsedArgs.Namespace).UpdateScale(ctx, parsedArgs.StatefulSetName, scale, metav1.UpdateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to scale statefulset: %w", err)
	}

	return fmt.Sprintf("Successfully scaled StatefulSet '%s' in namespace '%s' to %d replicas", parsedArgs.StatefulSetName, parsedArgs.Namespace, parsedArgs.Replicas), nil
}
