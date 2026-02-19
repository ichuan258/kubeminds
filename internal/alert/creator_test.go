package alert

import (
	"context"
	"regexp"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	kubemindsv1alpha1 "kubeminds/api/v1alpha1"
)

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = kubemindsv1alpha1.AddToScheme(s)
	return s
}

func TestDiagnosisTaskCreator_Create(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name           string
		group          *AlertGroup
		wantKind       string
		wantNamespace  string
		wantTargetName string
		wantAlertName  string
		wantLabels     map[string]string
		wantErr        bool
	}{
		{
			name: "pod_level_alert",
			group: &AlertGroup{
				Key:          "OOMKilled/default/nginx-abc",
				AlertName:    "KubePodCrashLooping",
				Namespace:    "default",
				Pod:          "nginx-abc",
				MergedLabels: map[string]string{"severity": "critical", "reason": "OOMKilled"},
				FirstSeen:    now,
				LastSeen:     now,
				Count:        1,
			},
			wantKind:       "Pod",
			wantNamespace:  "default",
			wantTargetName: "nginx-abc",
			wantAlertName:  "KubePodCrashLooping",
			wantLabels:     map[string]string{"severity": "critical", "reason": "OOMKilled"},
		},
		{
			name: "namespace_level_alert_no_pod",
			group: &AlertGroup{
				Key:          "KubeNodeNotReady/kube-system/_",
				AlertName:    "KubeNodeNotReady",
				Namespace:    "kube-system",
				Pod:          "",
				MergedLabels: map[string]string{"severity": "warning"},
				FirstSeen:    now,
				LastSeen:     now,
				Count:        1,
			},
			wantKind:       "Namespace",
			wantNamespace:  "kube-system",
			wantTargetName: "kube-system",
			wantAlertName:  "KubeNodeNotReady",
		},
		{
			name: "alert_with_empty_labels",
			group: &AlertGroup{
				Key:          "UnknownAlert/_/_",
				AlertName:    "UnknownAlert",
				Namespace:    "",
				Pod:          "",
				MergedLabels: nil,
				FirstSeen:    now,
				LastSeen:     now,
				Count:        1,
			},
			wantKind:      "Namespace",
			wantAlertName: "UnknownAlert",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().
				WithScheme(newTestScheme()).
				Build()

			creator := NewDiagnosisTaskCreator(fakeClient, "default")
			err := creator.Create(context.Background(), tt.group)

			if (err != nil) != tt.wantErr {
				t.Fatalf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			// Retrieve the created task to verify fields.
			var list kubemindsv1alpha1.DiagnosisTaskList
			if err := fakeClient.List(context.Background(), &list); err != nil {
				t.Fatalf("failed to list DiagnosisTasks: %v", err)
			}
			if len(list.Items) != 1 {
				t.Fatalf("expected 1 DiagnosisTask, got %d", len(list.Items))
			}
			task := list.Items[0]

			if tt.wantKind != "" && task.Spec.Target.Kind != tt.wantKind {
				t.Errorf("Target.Kind = %q, want %q", task.Spec.Target.Kind, tt.wantKind)
			}
			if tt.wantTargetName != "" && task.Spec.Target.Name != tt.wantTargetName {
				t.Errorf("Target.Name = %q, want %q", task.Spec.Target.Name, tt.wantTargetName)
			}
			if tt.wantAlertName != "" && task.Spec.AlertContext.Name != tt.wantAlertName {
				t.Errorf("AlertContext.Name = %q, want %q", task.Spec.AlertContext.Name, tt.wantAlertName)
			}
			for k, v := range tt.wantLabels {
				if task.Spec.AlertContext.Labels[k] != v {
					t.Errorf("AlertContext.Labels[%q] = %q, want %q", k, task.Spec.AlertContext.Labels[k], v)
				}
			}
		})
	}
}

func TestDiagnosisTaskCreator_TaskName_K8sValid(t *testing.T) {
	now := time.Now()
	fakeClient := fake.NewClientBuilder().WithScheme(newTestScheme()).Build()
	creator := NewDiagnosisTaskCreator(fakeClient, "default")

	alertNames := []string{
		"KubePodCrashLooping",
		"KubeContainerOOMKilled",
		"node_not_ready",
		"ALERT WITH SPACES",
		"alert/with/slashes",
	}

	// K8s name must be lowercase alphanumeric and "-", no leading/trailing "-"
	validName := regexp.MustCompile(`^[a-z0-9][a-z0-9\-]*[a-z0-9]$`)

	for _, alertName := range alertNames {
		t.Run(alertName, func(t *testing.T) {
			group := &AlertGroup{
				AlertName: alertName,
				Namespace: "default",
				Pod:       "pod",
				FirstSeen: now,
				LastSeen:  now,
				Count:     1,
			}
			taskName := creator.buildTaskName(group.AlertName)
			if !validName.MatchString(taskName) {
				t.Errorf("buildTaskName(%q) = %q is not a valid K8s name", alertName, taskName)
			}
		})
	}
}

func TestDiagnosisTaskCreator_AlreadyExists_Idempotent(t *testing.T) {
	now := time.Now()
	fakeClient := fake.NewClientBuilder().WithScheme(newTestScheme()).Build()
	creator := NewDiagnosisTaskCreator(fakeClient, "default")

	group := &AlertGroup{
		AlertName:    "KubePodCrashLooping",
		Namespace:    "default",
		Pod:          "nginx-abc",
		MergedLabels: map[string]string{"severity": "critical"},
		FirstSeen:    now,
		LastSeen:     now,
		Count:        1,
	}

	// First create should succeed.
	if err := creator.Create(context.Background(), group); err != nil {
		t.Fatalf("first Create() failed: %v", err)
	}

	// Simulate AlreadyExists by creating a task with the same name.
	// (In practice the name includes a timestamp so collisions are rare;
	// this test verifies the error-handling path.)
	task := creator.buildTask(group)
	task.Name = "alert-kubepodcrashlooping-99999"
	if err := fakeClient.Create(context.Background(), task); err != nil {
		// If already created above, AlreadyExists is expected and is OK.
		_ = err
	}
	// Re-creating via Create() should not return an error.
	group2 := *group
	if err := creator.Create(context.Background(), &group2); err != nil {
		t.Errorf("Create() on already-existing task returned unexpected error: %v", err)
	}
}
