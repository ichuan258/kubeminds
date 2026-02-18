package tools

import (
	"context"
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetServiceSpecTool(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "default",
			},
			Spec: corev1.ServiceSpec{
				ClusterIP: "10.0.0.1",
				Ports: []corev1.ServicePort{
					{
						Port:       80,
						TargetPort: intstr.FromInt(8080),
					},
				},
			},
		},
	)

	tool := NewGetServiceSpecTool(client)

	t.Run("should return service spec", func(t *testing.T) {
		args := ServiceArgs{Namespace: "default", ServiceName: "test-service"}
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
		if !contains(result, "10.0.0.1") {
			t.Fatalf("expected ClusterIP in result")
		}
	})

	t.Run("should fail for non-existent service", func(t *testing.T) {
		args := ServiceArgs{Namespace: "default", ServiceName: "non-existent"}
		argsJSON, _ := json.Marshal(args)
		_, err := tool.Execute(context.Background(), string(argsJSON))

		if err == nil {
			t.Fatalf("expected error for non-existent service")
		}
	})

	t.Run("should have correct metadata", func(t *testing.T) {
		if tool.Name() != "get_service_spec" {
			t.Errorf("expected name 'get_service_spec', got %s", tool.Name())
		}
		if tool.SafetyLevel() != "ReadOnly" {
			t.Errorf("expected ReadOnly safety level")
		}
	})
}

func TestGetEndpointsTool(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "default",
			},
			Subsets: []corev1.EndpointSubset{
				{
					Addresses: []corev1.EndpointAddress{
						{
							IP: "10.0.1.1",
						},
					},
					Ports: []corev1.EndpointPort{
						{
							Port: 8080,
						},
					},
				},
			},
		},
	)

	tool := NewGetEndpointsTool(client)

	t.Run("should return endpoints", func(t *testing.T) {
		args := ServiceArgs{Namespace: "default", ServiceName: "test-service"}
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
		if !contains(result, "10.0.1.1") {
			t.Fatalf("expected endpoint IP in result")
		}
	})

	t.Run("should fail for non-existent endpoints", func(t *testing.T) {
		args := ServiceArgs{Namespace: "default", ServiceName: "non-existent"}
		argsJSON, _ := json.Marshal(args)
		_, err := tool.Execute(context.Background(), string(argsJSON))

		if err == nil {
			t.Fatalf("expected error for non-existent endpoints")
		}
	})

	t.Run("should have correct metadata", func(t *testing.T) {
		if tool.Name() != "get_endpoints" {
			t.Errorf("expected name 'get_endpoints', got %s", tool.Name())
		}
		if tool.SafetyLevel() != "ReadOnly" {
			t.Errorf("expected ReadOnly safety level")
		}
	})
}
