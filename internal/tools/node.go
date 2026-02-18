package tools

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"kubeminds/internal/agent"
)

type NodeArgs struct {
	NodeName string `json:"node_name"`
}

// GetNodeStatusTool implements the get_node_status tool
type GetNodeStatusTool struct {
	client kubernetes.Interface
}

func NewGetNodeStatusTool(client kubernetes.Interface) *GetNodeStatusTool {
	return &GetNodeStatusTool{client: client}
}

func (t *GetNodeStatusTool) Name() string {
	return "get_node_status"
}

func (t *GetNodeStatusTool) Description() string {
	return "Get the status and conditions of a specific Kubernetes node. Use this to diagnose node-level issues like NotReady, DiskPressure, or MemoryPressure."
}

func (t *GetNodeStatusTool) Schema() string {
	return `{
		"type": "object",
		"properties": {
			"node_name": {
				"type": "string",
				"description": "The name of the node"
			}
		},
		"required": ["node_name"]
	}`
}

func (t *GetNodeStatusTool) SafetyLevel() agent.SafetyLevel {
	return agent.SafetyLevelReadOnly
}

func (t *GetNodeStatusTool) Execute(ctx context.Context, args string) (string, error) {
	var parsedArgs NodeArgs
	if err := json.Unmarshal([]byte(args), &parsedArgs); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	node, err := t.client.CoreV1().Nodes().Get(ctx, parsedArgs.NodeName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get node: %w", err)
	}

	// Remove managed fields to reduce noise
	node.ManagedFields = nil

	data, err := json.MarshalIndent(node, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal node: %w", err)
	}

	return string(data), nil
}

// GetNodeEventsTool implements the get_node_events tool
type GetNodeEventsTool struct {
	client kubernetes.Interface
}

func NewGetNodeEventsTool(client kubernetes.Interface) *GetNodeEventsTool {
	return &GetNodeEventsTool{client: client}
}

func (t *GetNodeEventsTool) Name() string {
	return "get_node_events"
}

func (t *GetNodeEventsTool) Description() string {
	return "Get events related to a specific node. Use this to identify node heartbeat failures, status changes, or resource issues."
}

func (t *GetNodeEventsTool) Schema() string {
	return `{
		"type": "object",
		"properties": {
			"node_name": {
				"type": "string",
				"description": "The name of the node"
			}
		},
		"required": ["node_name"]
	}`
}

func (t *GetNodeEventsTool) SafetyLevel() agent.SafetyLevel {
	return agent.SafetyLevelReadOnly
}

func (t *GetNodeEventsTool) Execute(ctx context.Context, args string) (string, error) {
	var parsedArgs NodeArgs
	if err := json.Unmarshal([]byte(args), &parsedArgs); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	events, err := t.client.CoreV1().Events("").List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s,involvedObject.kind=Node", parsedArgs.NodeName),
	})
	if err != nil {
		return "", fmt.Errorf("failed to list events: %w", err)
	}

	if len(events.Items) == 0 {
		return "No events found for this node.", nil
	}

	var result string
	for _, e := range events.Items {
		result += fmt.Sprintf("[%s] %s: %s (count: %d)\n", e.Type, e.Reason, e.Message, e.Count)
	}
	return result, nil
}
