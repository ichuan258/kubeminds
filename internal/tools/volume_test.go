package tools

import (
	"context"
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetPVCStatusTool(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pvc",
				Namespace: "default",
			},
			Status: corev1.PersistentVolumeClaimStatus{
				Phase: corev1.ClaimBound,
			},
		},
	)

	tool := NewGetPVCStatusTool(client)

	t.Run("should return PVC status", func(t *testing.T) {
		args := PVCArgs{Namespace: "default", PVCName: "test-pvc"}
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
		if !contains(result, "Bound") {
			t.Fatalf("expected 'Bound' phase in result")
		}
	})

	t.Run("should fail for non-existent PVC", func(t *testing.T) {
		args := PVCArgs{Namespace: "default", PVCName: "non-existent"}
		argsJSON, _ := json.Marshal(args)
		_, err := tool.Execute(context.Background(), string(argsJSON))

		if err == nil {
			t.Fatalf("expected error for non-existent PVC")
		}
	})

	t.Run("should have correct metadata", func(t *testing.T) {
		if tool.Name() != "get_pvc_status" {
			t.Errorf("expected name 'get_pvc_status', got %s", tool.Name())
		}
		if tool.SafetyLevel() != "ReadOnly" {
			t.Errorf("expected ReadOnly safety level")
		}
	})
}

func TestGetPVStatusTool(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-pv",
			},
			Status: corev1.PersistentVolumeStatus{
				Phase: corev1.VolumeBound,
			},
			Spec: corev1.PersistentVolumeSpec{
				Capacity: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("10Gi"),
				},
			},
		},
	)

	tool := NewGetPVStatusTool(client)

	t.Run("should return PV status", func(t *testing.T) {
		args := PVArgs{PVName: "test-pv"}
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
		if !contains(result, "Bound") {
			t.Fatalf("expected 'Bound' phase in result")
		}
	})

	t.Run("should fail for non-existent PV", func(t *testing.T) {
		args := PVArgs{PVName: "non-existent"}
		argsJSON, _ := json.Marshal(args)
		_, err := tool.Execute(context.Background(), string(argsJSON))

		if err == nil {
			t.Fatalf("expected error for non-existent PV")
		}
	})

	t.Run("should have correct metadata", func(t *testing.T) {
		if tool.Name() != "get_pv_status" {
			t.Errorf("expected name 'get_pv_status', got %s", tool.Name())
		}
		if tool.SafetyLevel() != "ReadOnly" {
			t.Errorf("expected ReadOnly safety level")
		}
	})
}
