package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type PodArgs struct {
	Namespace string `json:"namespace"`
	PodName   string `json:"pod_name"`
}

// GetPodLogsTool implements the get_pod_logs tool
type GetPodLogsTool struct {
	client kubernetes.Interface
}

func NewGetPodLogsTool(client kubernetes.Interface) *GetPodLogsTool {
	return &GetPodLogsTool{client: client}
}

func (t *GetPodLogsTool) Name() string {
	return "get_pod_logs"
}

func (t *GetPodLogsTool) Description() string {
	return "Get logs from a specific pod in a namespace. Use this to analyze application errors and stack traces."
}

func (t *GetPodLogsTool) Schema() string {
	return `{
		"type": "object",
		"properties": {
			"namespace": {
				"type": "string",
				"description": "The namespace of the pod"
			},
			"pod_name": {
				"type": "string",
				"description": "The name of the pod"
			}
		},
		"required": ["namespace", "pod_name"]
	}`
}

func (t *GetPodLogsTool) Execute(ctx context.Context, args string) (string, error) {
	var parsedArgs PodArgs
	if err := json.Unmarshal([]byte(args), &parsedArgs); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	req := t.client.CoreV1().Pods(parsedArgs.Namespace).GetLogs(parsedArgs.PodName, &corev1.PodLogOptions{
		TailLines: func(i int64) *int64 { return &i }(100), // Default to last 100 lines for MVP
	})

	podLogs, err := req.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("error in opening stream: %w", err)
	}
	defer podLogs.Close()

	buf := new(strings.Builder)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		return "", fmt.Errorf("error in reading stream: %w", err)
	}

	return buf.String(), nil
}

// GetPodEventsTool implements the get_pod_events tool
type GetPodEventsTool struct {
	client kubernetes.Interface
}

func NewGetPodEventsTool(client kubernetes.Interface) *GetPodEventsTool {
	return &GetPodEventsTool{client: client}
}

func (t *GetPodEventsTool) Name() string {
	return "get_pod_events"
}

func (t *GetPodEventsTool) Description() string {
	return "Get events related to a specific pod. Use this to identify scheduling issues, image pull errors, or restart reasons."
}

func (t *GetPodEventsTool) Schema() string {
	return `{
		"type": "object",
		"properties": {
			"namespace": {
				"type": "string",
				"description": "The namespace of the pod"
			},
			"pod_name": {
				"type": "string",
				"description": "The name of the pod"
			}
		},
		"required": ["namespace", "pod_name"]
	}`
}

func (t *GetPodEventsTool) Execute(ctx context.Context, args string) (string, error) {
	var parsedArgs PodArgs
	if err := json.Unmarshal([]byte(args), &parsedArgs); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	events, err := t.client.CoreV1().Events(parsedArgs.Namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s,involvedObject.kind=Pod", parsedArgs.PodName),
	})
	if err != nil {
		return "", fmt.Errorf("failed to list events: %w", err)
	}

	if len(events.Items) == 0 {
		return "No events found for this pod.", nil
	}

	var result string
	for _, e := range events.Items {
		result += fmt.Sprintf("[%s] %s: %s\n", e.Type, e.Reason, e.Message)
	}
	return result, nil
}

// GetPodSpecTool implements the get_pod_spec tool
type GetPodSpecTool struct {
	client kubernetes.Interface
}

func NewGetPodSpecTool(client kubernetes.Interface) *GetPodSpecTool {
	return &GetPodSpecTool{client: client}
}

func (t *GetPodSpecTool) Name() string {
	return "get_pod_spec"
}

func (t *GetPodSpecTool) Description() string {
	return "Get the full specification and status of a pod in YAML format. Use this to check configuration, resource limits, and status conditions."
}

func (t *GetPodSpecTool) Schema() string {
	return `{
		"type": "object",
		"properties": {
			"namespace": {
				"type": "string",
				"description": "The namespace of the pod"
			},
			"pod_name": {
				"type": "string",
				"description": "The name of the pod"
			}
		},
		"required": ["namespace", "pod_name"]
	}`
}

func (t *GetPodSpecTool) Execute(ctx context.Context, args string) (string, error) {
	var parsedArgs PodArgs
	if err := json.Unmarshal([]byte(args), &parsedArgs); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	pod, err := t.client.CoreV1().Pods(parsedArgs.Namespace).Get(ctx, parsedArgs.PodName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get pod: %w", err)
	}

	// Remove managed fields to reduce noise
	pod.ManagedFields = nil

	data, err := json.MarshalIndent(pod, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal pod: %w", err)
	}

	return string(data), nil
}
