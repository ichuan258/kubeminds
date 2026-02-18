package tools

import (
	"context"
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetNodeStatusTool(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node",
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		},
	)

	tool := NewGetNodeStatusTool(client)

	t.Run("should return node status", func(t *testing.T) {
		args := NodeArgs{NodeName: "test-node"}
		argsJSON, _ := json.Marshal(args)
		result, err := tool.Execute(context.Background(), string(argsJSON))

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == "" {
			t.Fatalf("expected result, got empty string")
		}
		if !json.Valid([]byte(result)) {
			t.Fatalf("result is not valid JSON")
		}
	})

	t.Run("should fail for non-existent node", func(t *testing.T) {
		args := NodeArgs{NodeName: "non-existent"}
		argsJSON, _ := json.Marshal(args)
		_, err := tool.Execute(context.Background(), string(argsJSON))

		if err == nil {
			t.Fatalf("expected error for non-existent node")
		}
	})

	t.Run("should have correct metadata", func(t *testing.T) {
		if tool.Name() != "get_node_status" {
			t.Errorf("expected name 'get_node_status', got %s", tool.Name())
		}
		if tool.SafetyLevel() != "ReadOnly" {
			t.Errorf("expected ReadOnly safety level")
		}
		if !json.Valid([]byte(tool.Schema())) {
			t.Errorf("schema is not valid JSON")
		}
	})
}

func TestGetNodeEventsTool(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Event{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-event",
				Namespace: "default",
			},
			InvolvedObject: corev1.ObjectReference{
				Kind: "Node",
				Name: "test-node",
			},
			Reason:  "NodeNotReady",
			Message: "Test node is not ready",
			Type:    corev1.EventTypeWarning,
			Count:   1,
		},
	)

	tool := NewGetNodeEventsTool(client)

	t.Run("should return node events", func(t *testing.T) {
		args := NodeArgs{NodeName: "test-node"}
		argsJSON, _ := json.Marshal(args)
		result, err := tool.Execute(context.Background(), string(argsJSON))

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == "" {
			t.Fatalf("expected result, got empty string")
		}
		if !contains(result, "NodeNotReady") {
			t.Fatalf("expected 'NodeNotReady' in result")
		}
	})

	t.Run("should have correct metadata", func(t *testing.T) {
		if tool.Name() != "get_node_events" {
			t.Errorf("expected name 'get_node_events', got %s", tool.Name())
		}
		if tool.SafetyLevel() != "ReadOnly" {
			t.Errorf("expected ReadOnly safety level")
		}
	})
}

func contains(s, substr string) bool {
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
