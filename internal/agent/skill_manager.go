package agent

import (
	"log/slog"
	"os"

	"kubeminds/api/v1alpha1"
)

// SkillManager handles skill matching and selection
type SkillManager struct {
	skills map[string]Skill
	logger *slog.Logger
}

// NewSkillManager creates a new SkillManager loading skills from the specified directory
func NewSkillManager(skillDir string, logger *slog.Logger) (*SkillManager, error) {
	if logger == nil {
		logger = slog.Default()
	}

	sm := &SkillManager{
		skills: make(map[string]Skill),
		logger: logger,
	}

	// 1. Load from YAML files
	loader := NewSkillLoader()
	if _, err := os.Stat(skillDir); err == nil {
		loadedSkills, err := loader.LoadSkills(skillDir)
		if err != nil {
			return nil, err
		}
		for _, skill := range loadedSkills {
			sm.Register(skill)
		}
		logger.Info("Loaded skills from directory", "dir", skillDir, "count", len(loadedSkills))
	} else {
		logger.Warn("Skill directory not found, using built-in fallback skills", "dir", skillDir)
		// Fallback to built-in skills if directory doesn't exist
		sm.Register(BaseSkill)
		sm.Register(OOMSkill)
	}

	return sm, nil
}

// Register adds a skill to the manager
func (sm *SkillManager) Register(skill Skill) {
	sm.skills[skill.Name] = skill
}

// Match selects the most appropriate skill for a given task
func (sm *SkillManager) Match(task *v1alpha1.DiagnosisTask) Skill {
	// 1. Iterate over all skills and check their triggers
	for _, skill := range sm.skills {
		for _, trigger := range skill.Triggers {
			if sm.matchesTrigger(task, trigger) {
				sm.logger.Info("Matched skill via trigger", "skill", skill.Name, "trigger", trigger)
				return skill
			}
		}
	}

	// 2. Fallback logic for legacy hardcoded OOM check (if no YAML triggers matched)
	if task.Spec.AlertContext != nil {
		labels := task.Spec.AlertContext.Labels
		if labels["reason"] == "OOMKilled" || labels["alertname"] == "KubeContainerOOMKilled" {
			if skill, ok := sm.GetSkillByName("oom_diagnosis"); ok {
				return skill
			}
		}
	}

	// 3. Fallback to BaseSkill
	if skill, ok := sm.GetSkillByName("base_skill"); ok {
		return skill
	}

	// Absolute fallback if base_skill is missing (should not happen if loaded correctly)
	return BaseSkill
}

// matchesTrigger checks if a task matches a trigger rule
func (sm *SkillManager) matchesTrigger(task *v1alpha1.DiagnosisTask, trigger TriggerRule) bool {
	if task.Spec.AlertContext == nil {
		return false
	}

	context := task.Spec.AlertContext

	// Check AlertName match
	if trigger.AlertName != "" && context.Name != trigger.AlertName {
		return false
	}

	// Check Labels match
	for k, v := range trigger.Labels {
		if context.Labels[k] != v {
			return false
		}
	}

	return true
}

// GetSkillByName retrieves a skill by name
func (sm *SkillManager) GetSkillByName(name string) (Skill, bool) {
	skill, ok := sm.skills[name]
	return skill, ok
}

// ListSkills returns all registered skills
func (sm *SkillManager) ListSkills() []Skill {
	skills := make([]Skill, 0, len(sm.skills))
	for _, s := range sm.skills {
		skills = append(skills, s)
	}
	return skills
}
