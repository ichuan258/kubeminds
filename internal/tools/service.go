package tools

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"kubeminds/internal/agent"
)

type ServiceArgs struct {
	Namespace   string `json:"namespace"`
	ServiceName string `json:"service_name"`
}

// GetServiceSpecTool implements the get_service_spec tool
type GetServiceSpecTool struct {
	client kubernetes.Interface
}

func NewGetServiceSpecTool(client kubernetes.Interface) *GetServiceSpecTool {
	return &GetServiceSpecTool{client: client}
}

func (t *GetServiceSpecTool) Name() string {
	return "get_service_spec"
}

func (t *GetServiceSpecTool) Description() string {
	return "Get the specification and status of a Kubernetes service. Use this to check service configuration, endpoints, and ClusterIP details."
}

func (t *GetServiceSpecTool) Schema() string {
	return `{
		"type": "object",
		"properties": {
			"namespace": {
				"type": "string",
				"description": "The namespace of the service"
			},
			"service_name": {
				"type": "string",
				"description": "The name of the service"
			}
		},
		"required": ["namespace", "service_name"]
	}`
}

func (t *GetServiceSpecTool) SafetyLevel() agent.SafetyLevel {
	return agent.SafetyLevelReadOnly
}

func (t *GetServiceSpecTool) Execute(ctx context.Context, args string) (string, error) {
	var parsedArgs ServiceArgs
	if err := json.Unmarshal([]byte(args), &parsedArgs); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	svc, err := t.client.CoreV1().Services(parsedArgs.Namespace).Get(ctx, parsedArgs.ServiceName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get service: %w", err)
	}

	// Remove managed fields to reduce noise
	svc.ManagedFields = nil

	data, err := json.MarshalIndent(svc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal service: %w", err)
	}

	return string(data), nil
}

// GetEndpointsTool implements the get_endpoints tool
type GetEndpointsTool struct {
	client kubernetes.Interface
}

func NewGetEndpointsTool(client kubernetes.Interface) *GetEndpointsTool {
	return &GetEndpointsTool{client: client}
}

func (t *GetEndpointsTool) Name() string {
	return "get_endpoints"
}

func (t *GetEndpointsTool) Description() string {
	return "Get the endpoints for a Kubernetes service. Use this to verify which pods are currently backing the service and check their availability."
}

func (t *GetEndpointsTool) Schema() string {
	return `{
		"type": "object",
		"properties": {
			"namespace": {
				"type": "string",
				"description": "The namespace of the endpoints"
			},
			"service_name": {
				"type": "string",
				"description": "The name of the service (endpoints use the same name as the service)"
			}
		},
		"required": ["namespace", "service_name"]
	}`
}

func (t *GetEndpointsTool) SafetyLevel() agent.SafetyLevel {
	return agent.SafetyLevelReadOnly
}

func (t *GetEndpointsTool) Execute(ctx context.Context, args string) (string, error) {
	var parsedArgs ServiceArgs
	if err := json.Unmarshal([]byte(args), &parsedArgs); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	endpoints, err := t.client.CoreV1().Endpoints(parsedArgs.Namespace).Get(ctx, parsedArgs.ServiceName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get endpoints: %w", err)
	}

	// Remove managed fields to reduce noise
	endpoints.ManagedFields = nil

	data, err := json.MarshalIndent(endpoints, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal endpoints: %w", err)
	}

	return string(data), nil
}
