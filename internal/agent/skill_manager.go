package agent

import (
	"kubeminds/api/v1alpha1"
)

// SkillManager handles skill matching and selection
type SkillManager struct {
	skills map[string]Skill
}

// NewSkillManager creates a new SkillManager with default skills
func NewSkillManager() *SkillManager {
	sm := &SkillManager{
		skills: make(map[string]Skill),
	}
	sm.Register(BaseSkill)
	sm.Register(OOMSkill)
	return sm
}

// Register adds a skill to the manager
func (sm *SkillManager) Register(skill Skill) {
	sm.skills[skill.Name] = skill
}

// Match selects the most appropriate skill for a given task
// For MVP, this is a simple heuristic based on the task description or target
func (sm *SkillManager) Match(task *v1alpha1.DiagnosisTask) Skill {
	// Simple matching logic based on target Kind or specific keywords in goal (not yet in CRD)
	// For MVP, we'll default to BaseSkill unless specified otherwise.
	// Ideally, DiagnosisTask should have a field for SkillName or the Controller should infer it.

	// Placeholder logic: Check if any part of the spec hints at OOM
	// This would require access to Pod status or events, which we don't have here yet.
	// So for now, we just return BaseSkill.
	// Future: Controller should pass in relevant labels/events to help selection.

	return BaseSkill
}

// GetSkillByName retrieves a skill by name
func (sm *SkillManager) GetSkillByName(name string) (Skill, bool) {
	skill, ok := sm.skills[name]
	return skill, ok
}
