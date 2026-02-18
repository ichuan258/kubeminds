package agent

import (
	"log/slog"
	"testing"

	"kubeminds/api/v1alpha1"
)

func TestSkillManager_Match(t *testing.T) {
	tests := []struct {
		name         string
		task         *v1alpha1.DiagnosisTask
		expectedSkill string
	}{
		{
			name: "Should match OOMSkill when reason is OOMKilled",
			task: &v1alpha1.DiagnosisTask{
				Spec: v1alpha1.DiagnosisTaskSpec{
					AlertContext: &v1alpha1.AlertContext{
						Labels: map[string]string{
							"reason": "OOMKilled",
						},
					},
				},
			},
			expectedSkill: "oom_diagnosis",
		},
		{
			name: "Should match OOMSkill when alertname is KubeContainerOOMKilled",
			task: &v1alpha1.DiagnosisTask{
				Spec: v1alpha1.DiagnosisTaskSpec{
					AlertContext: &v1alpha1.AlertContext{
						Labels: map[string]string{
							"alertname": "KubeContainerOOMKilled",
						},
					},
				},
			},
			expectedSkill: "oom_diagnosis",
		},
		{
			name: "Should match BaseSkill when no matching labels found",
			task: &v1alpha1.DiagnosisTask{
				Spec: v1alpha1.DiagnosisTaskSpec{
					AlertContext: &v1alpha1.AlertContext{
						Labels: map[string]string{
							"severity": "critical",
						},
					},
				},
			},
			expectedSkill: "base_skill",
		},
		{
			name: "Should match BaseSkill when AlertContext is nil",
			task: &v1alpha1.DiagnosisTask{
				Spec: v1alpha1.DiagnosisTaskSpec{
					AlertContext: nil,
				},
			},
			expectedSkill: "base_skill",
		},
	}

	sm, err := NewSkillManager("", slog.Default())
	if err != nil {
		t.Fatalf("failed to create skill manager: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skill := sm.Match(tt.task)
			if skill.Name != tt.expectedSkill {
				t.Errorf("Match() skill = %v, want %v", skill.Name, tt.expectedSkill)
			}
		})
	}
}
