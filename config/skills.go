package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Skill represents a reusable task template stored in ~/.hivemind/skills/.
type Skill struct {
	Name         string   // display name
	Description  string   // short description shown in picker
	ContextFiles []string // absolute or ~-relative paths to include as context
	SetupScript  string   // shell command to run before agent starts
	Instructions string   // the markdown body — prepended to the agent prompt
	SourceFile   string   // path to the .md file (for display)
}

type skillFrontmatter struct {
	Name         string   `yaml:"name"`
	Description  string   `yaml:"description"`
	ContextFiles []string `yaml:"context_files"`
	SetupScript  string   `yaml:"setup_script"`
}

// LoadSkills loads all skills from ~/.hivemind/skills/.
func LoadSkills() ([]Skill, error) {
	dir, err := GetConfigDir()
	if err != nil {
		return nil, fmt.Errorf("skills: get config dir: %w", err)
	}
	return LoadSkillsFrom(filepath.Join(dir, "skills"))
}

// LoadSkillsFrom loads all *.md skill files from the given directory.
// Files with malformed frontmatter are skipped with a warning log.
func LoadSkillsFrom(dir string) ([]Skill, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read skills dir: %w", err)
	}

	var skills []Skill
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		skill, err := parseSkillFile(data, path)
		if err != nil {
			// skip malformed files
			continue
		}
		skills = append(skills, skill)
	}
	return skills, nil
}

// parseSkillFile parses a skill .md file with YAML frontmatter.
func parseSkillFile(data []byte, sourcePath string) (Skill, error) {
	const sep = "---"
	s := string(data)

	if !strings.HasPrefix(s, sep) {
		return Skill{}, fmt.Errorf("missing frontmatter")
	}
	// Find closing ---
	rest := s[len(sep):]
	idx := strings.Index(rest, "\n"+sep)
	if idx < 0 {
		return Skill{}, fmt.Errorf("unclosed frontmatter")
	}
	frontmatterRaw := rest[:idx]
	body := rest[idx+len("\n"+sep):]
	// Trim optional leading newline after separator
	body = strings.TrimPrefix(body, "\n")

	var fm skillFrontmatter
	if err := yaml.NewDecoder(bytes.NewBufferString(frontmatterRaw)).Decode(&fm); err != nil {
		return Skill{}, fmt.Errorf("parse frontmatter: %w", err)
	}

	name := fm.Name
	if name == "" {
		// Fall back to filename without extension
		base := filepath.Base(sourcePath)
		name = strings.TrimSuffix(base, ".md")
	}

	return Skill{
		Name:         name,
		Description:  fm.Description,
		ContextFiles: fm.ContextFiles,
		SetupScript:  fm.SetupScript,
		Instructions: body,
		SourceFile:   sourcePath,
	}, nil
}
