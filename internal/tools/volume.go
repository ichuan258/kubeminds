package tools

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"kubeminds/internal/agent"
)

type PVCArgs struct {
	Namespace string `json:"namespace"`
	PVCName   string `json:"pvc_name"`
}

type PVArgs struct {
	PVName string `json:"pv_name"`
}

// GetPVCStatusTool implements the get_pvc_status tool
type GetPVCStatusTool struct {
	client kubernetes.Interface
}

func NewGetPVCStatusTool(client kubernetes.Interface) *GetPVCStatusTool {
	return &GetPVCStatusTool{client: client}
}

func (t *GetPVCStatusTool) Name() string {
	return "get_pvc_status"
}

func (t *GetPVCStatusTool) Description() string {
	return "Get the status of a PersistentVolumeClaim (PVC). Use this to check PVC binding status, capacity, and access modes."
}

func (t *GetPVCStatusTool) Schema() string {
	return `{
		"type": "object",
		"properties": {
			"namespace": {
				"type": "string",
				"description": "The namespace of the PVC"
			},
			"pvc_name": {
				"type": "string",
				"description": "The name of the PVC"
			}
		},
		"required": ["namespace", "pvc_name"]
	}`
}

func (t *GetPVCStatusTool) SafetyLevel() agent.SafetyLevel {
	return agent.SafetyLevelReadOnly
}

func (t *GetPVCStatusTool) Execute(ctx context.Context, args string) (string, error) {
	var parsedArgs PVCArgs
	if err := json.Unmarshal([]byte(args), &parsedArgs); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	pvc, err := t.client.CoreV1().PersistentVolumeClaims(parsedArgs.Namespace).Get(ctx, parsedArgs.PVCName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get PVC: %w", err)
	}

	// Remove managed fields to reduce noise
	pvc.ManagedFields = nil

	data, err := json.MarshalIndent(pvc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal PVC: %w", err)
	}

	return string(data), nil
}

// GetPVStatusTool implements the get_pv_status tool
type GetPVStatusTool struct {
	client kubernetes.Interface
}

func NewGetPVStatusTool(client kubernetes.Interface) *GetPVStatusTool {
	return &GetPVStatusTool{client: client}
}

func (t *GetPVStatusTool) Name() string {
	return "get_pv_status"
}

func (t *GetPVStatusTool) Description() string {
	return "Get the status of a PersistentVolume (PV). Use this to check PV binding, reclaim policy, and storage backend health."
}

func (t *GetPVStatusTool) Schema() string {
	return `{
		"type": "object",
		"properties": {
			"pv_name": {
				"type": "string",
				"description": "The name of the PV (PVs are cluster-scoped, not namespaced)"
			}
		},
		"required": ["pv_name"]
	}`
}

func (t *GetPVStatusTool) SafetyLevel() agent.SafetyLevel {
	return agent.SafetyLevelReadOnly
}

func (t *GetPVStatusTool) Execute(ctx context.Context, args string) (string, error) {
	var parsedArgs PVArgs
	if err := json.Unmarshal([]byte(args), &parsedArgs); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	pv, err := t.client.CoreV1().PersistentVolumes().Get(ctx, parsedArgs.PVName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get PV: %w", err)
	}

	// Remove managed fields to reduce noise
	pv.ManagedFields = nil

	data, err := json.MarshalIndent(pv, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal PV: %w", err)
	}

	return string(data), nil
}
