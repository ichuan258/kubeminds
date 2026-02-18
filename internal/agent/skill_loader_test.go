package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/onsi/gomega"
)

func TestSkillLoader(t *testing.T) {
	g := gomega.NewWithT(t)

	// Helper to write a skill file
	writeSkill := func(dir, filename, content string) {
		err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644)
		g.Expect(err).NotTo(gomega.HaveOccurred())
	}

	t.Run("Load valid single skill", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "skills_valid")
		g.Expect(err).NotTo(gomega.HaveOccurred())
		defer os.RemoveAll(tempDir)

		writeSkill(tempDir, "base.yaml", `
name: base_skill
description: Base skill
system_prompt: Base prompt
`)
		loader := NewSkillLoader()
		skills, err := loader.LoadSkills(tempDir)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(skills).To(gomega.HaveKey("base_skill"))
		g.Expect(skills["base_skill"].SystemPrompt).To(gomega.Equal("Base prompt"))
	})

	t.Run("Load skill with inheritance", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "skills_inherit")
		g.Expect(err).NotTo(gomega.HaveOccurred())
		defer os.RemoveAll(tempDir)

		writeSkill(tempDir, "base2.yaml", `
name: base_skill_2
description: Base skill 2
system_prompt: Base prompt 2
allowed_tools: ["tool1"]
`)
		writeSkill(tempDir, "child.yaml", `
name: child_skill
parent: base_skill_2
system_prompt: Child prompt
allowed_tools: ["tool2"]
`)
		loader := NewSkillLoader()
		skills, err := loader.LoadSkills(tempDir)
		g.Expect(err).NotTo(gomega.HaveOccurred())

		child, ok := skills["child_skill"]
		g.Expect(ok).To(gomega.BeTrue())
		g.Expect(child.SystemPrompt).To(gomega.ContainSubstring("Base prompt 2"))
		g.Expect(child.SystemPrompt).To(gomega.ContainSubstring("Child prompt"))
		g.Expect(child.AllowedTools).To(gomega.Equal([]string{"tool2"})) // Override
	})

	t.Run("Detect missing parent", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "skills_orphan")
		g.Expect(err).NotTo(gomega.HaveOccurred())
		defer os.RemoveAll(tempDir)

		writeSkill(tempDir, "orphan.yaml", `
name: orphan_skill
parent: non_existent_parent
`)
		loader := NewSkillLoader()
		if _, err := loader.LoadSkills(tempDir); err == nil {
			t.Error("Expected error for missing parent, got nil")
		} else {
			g.Expect(err.Error()).To(gomega.ContainSubstring("skill not found: non_existent_parent"))
		}
	})

	t.Run("Detect circular inheritance", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "skills_cycle")
		g.Expect(err).NotTo(gomega.HaveOccurred())
		defer os.RemoveAll(tempDir)

		writeSkill(tempDir, "cycle1.yaml", `
name: cycle1
parent: cycle2
`)
		writeSkill(tempDir, "cycle2.yaml", `
name: cycle2
parent: cycle1
`)
		loader := NewSkillLoader()
		if _, err := loader.LoadSkills(tempDir); err == nil {
			t.Error("Expected error for circular inheritance, got nil")
		} else {
			g.Expect(err.Error()).To(gomega.ContainSubstring("circular inheritance"))
		}
	})
}
