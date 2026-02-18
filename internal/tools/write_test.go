package tools

import (
	"context"
	"encoding/json"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDeletePodTool(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	)

	tool := NewDeletePodTool(client)

	t.Run("should have HighRisk safety level", func(t *testing.T) {
		if tool.SafetyLevel() != "HighRisk" {
			t.Errorf("expected HighRisk safety level, got %s", tool.SafetyLevel())
		}
	})

	t.Run("should have correct metadata", func(t *testing.T) {
		if tool.Name() != "delete_pod" {
			t.Errorf("expected name 'delete_pod', got %s", tool.Name())
		}
		if !json.Valid([]byte(tool.Schema())) {
			t.Errorf("schema is not valid JSON")
		}
	})

	t.Run("should delete pod and return success message", func(t *testing.T) {
		args := DeletePodArgs{Namespace: "default", PodName: "test-pod"}
		argsJSON, _ := json.Marshal(args)
		result, err := tool.Execute(context.Background(), string(argsJSON))

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !contains(result, "Successfully deleted") {
			t.Fatalf("expected success message in result")
		}
	})

	t.Run("should fail for non-existent pod", func(t *testing.T) {
		args := DeletePodArgs{Namespace: "default", PodName: "non-existent"}
		argsJSON, _ := json.Marshal(args)
		_, err := tool.Execute(context.Background(), string(argsJSON))

		if err == nil {
			t.Fatalf("expected error for non-existent pod")
		}
	})
}

func TestPatchDeploymentTool(t *testing.T) {
	client := fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-deployment",
				Namespace: "default",
			},
		},
	)

	tool := NewPatchDeploymentTool(client)

	t.Run("should have HighRisk safety level", func(t *testing.T) {
		if tool.SafetyLevel() != "HighRisk" {
			t.Errorf("expected HighRisk safety level, got %s", tool.SafetyLevel())
		}
	})

	t.Run("should have correct metadata", func(t *testing.T) {
		if tool.Name() != "patch_deployment" {
			t.Errorf("expected name 'patch_deployment', got %s", tool.Name())
		}
		if !json.Valid([]byte(tool.Schema())) {
			t.Errorf("schema is not valid JSON")
		}
	})

	t.Run("should patch deployment with valid JSON", func(t *testing.T) {
		patchJSON := `{"spec":{"replicas":3}}`
		args := PatchDeploymentArgs{
			Namespace:      "default",
			DeploymentName: "test-deployment",
			PatchJSON:      patchJSON,
		}
		argsJSON, _ := json.Marshal(args)
		result, err := tool.Execute(context.Background(), string(argsJSON))

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !contains(result, "Successfully patched") {
			t.Fatalf("expected success message in result")
		}
	})

	t.Run("should fail for non-existent deployment", func(t *testing.T) {
		patchJSON := `{"spec":{"replicas":3}}`
		args := PatchDeploymentArgs{
			Namespace:      "default",
			DeploymentName: "non-existent",
			PatchJSON:      patchJSON,
		}
		argsJSON, _ := json.Marshal(args)
		_, err := tool.Execute(context.Background(), string(argsJSON))

		if err == nil {
			t.Fatalf("expected error for non-existent deployment")
		}
	})

	t.Run("should fail for invalid JSON", func(t *testing.T) {
		args := PatchDeploymentArgs{
			Namespace:      "default",
			DeploymentName: "test-deployment",
			PatchJSON:      "invalid json",
		}
		argsJSON, _ := json.Marshal(args)
		_, err := tool.Execute(context.Background(), string(argsJSON))

		if err == nil {
			t.Fatalf("expected error for invalid patch JSON")
		}
	})
}

func TestScaleStatefulSetTool(t *testing.T) {
	client := fake.NewSimpleClientset(
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-statefulset",
				Namespace: "default",
			},
		},
	)

	tool := NewScaleStatefulSetTool(client)

	t.Run("should have HighRisk safety level", func(t *testing.T) {
		if tool.SafetyLevel() != "HighRisk" {
			t.Errorf("expected HighRisk safety level, got %s", tool.SafetyLevel())
		}
	})

	t.Run("should have correct metadata", func(t *testing.T) {
		if tool.Name() != "scale_statefulset" {
			t.Errorf("expected name 'scale_statefulset', got %s", tool.Name())
		}
		if !json.Valid([]byte(tool.Schema())) {
			t.Errorf("schema is not valid JSON")
		}
	})

	t.Run("should scale statefulset", func(t *testing.T) {
		args := ScaleStatefulSetArgs{
			Namespace:       "default",
			StatefulSetName: "test-statefulset",
			Replicas:        3,
		}
		argsJSON, _ := json.Marshal(args)
		result, err := tool.Execute(context.Background(), string(argsJSON))

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !contains(result, "Successfully scaled") {
			t.Fatalf("expected success message in result")
		}
		if !contains(result, "3 replicas") {
			t.Fatalf("expected replica count in result")
		}
	})

	t.Run("should fail for non-existent statefulset", func(t *testing.T) {
		args := ScaleStatefulSetArgs{
			Namespace:       "default",
			StatefulSetName: "non-existent",
			Replicas:        3,
		}
		argsJSON, _ := json.Marshal(args)
		_, err := tool.Execute(context.Background(), string(argsJSON))

		if err == nil {
			t.Fatalf("expected error for non-existent statefulset")
		}
	})
}
