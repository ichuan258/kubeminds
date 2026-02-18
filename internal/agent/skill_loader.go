package agent

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SkillLoader handles loading skills from YAML files
type SkillLoader struct {
	skills map[string]Skill
}

// NewSkillLoader creates a new SkillLoader
func NewSkillLoader() *SkillLoader {
	return &SkillLoader{
		skills: make(map[string]Skill),
	}
}

// LoadSkills loads all skills from the specified directory
func (l *SkillLoader) LoadSkills(dir string) (map[string]Skill, error) {
	// 1. First pass: Load all raw skills into a map
	rawSkills := make(map[string]Skill)

	// Helper to resolve a skill recursively
	var resolve func(name string) (Skill, error)
	resolving := make(map[string]bool) // Detect cycles

	resolve = func(name string) (Skill, error) {
		if s, ok := l.skills[name]; ok {
			return s, nil // Already resolved
		}

		// Ensure rawSkills has the skill (it should if we loaded it)
		raw, ok := rawSkills[name]
		if !ok {
			// Try to find it in the rawSkills (loaded in first pass)
			return Skill{}, fmt.Errorf("skill not found: %s", name)
		}

		if resolving[name] {
			return Skill{}, fmt.Errorf("circular inheritance detected for skill: %s", name)
		}
		resolving[name] = true
		defer func() { resolving[name] = false }()

		if raw.Parent == "" {
			// Base case: no parent
			l.skills[name] = raw
			return raw, nil
		}

		// Recursive case: resolve parent first
		parent, err := resolve(raw.Parent)
		if err != nil {
			return Skill{}, fmt.Errorf("failed to resolve parent for %s: %w", name, err)
		}

		// Merge
		merged := parent.MergeWith(&raw)
		l.skills[name] = *merged
		return *merged, nil
	}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml") {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open skill file %s: %w", path, err)
		}
		defer file.Close()

		// Allow multiple documents in one file (though usually one per file)
		decoder := yaml.NewDecoder(file)
		for {
			var skill Skill
			if err := decoder.Decode(&skill); err != nil {
				if err == io.EOF {
					break
				}
				return fmt.Errorf("failed to parse skill file %s: %w", path, err)
			}

			if skill.Name == "" {
				return fmt.Errorf("skill in %s is missing a name", path)
			}
			rawSkills[skill.Name] = skill
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	// 2. Second pass: Resolve inheritance
	l.skills = make(map[string]Skill)

	// Resolve all skills
	for name := range rawSkills {
		if _, err := resolve(name); err != nil {
			return nil, err
		}
	}

	return l.skills, nil
}
