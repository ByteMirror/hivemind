package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ByteMirror/hivemind/config"
)

func TestLoadSkills_ParsesFrontmatter(t *testing.T) {
	dir := t.TempDir()
	content := `---
name: "code-review"
description: "Review code for issues"
context_files:
  - "~/notes.md"
setup_script: "echo ready"
---
You are a code reviewer. Focus on correctness.
`
	if err := os.WriteFile(filepath.Join(dir, "review.md"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	skills, err := config.LoadSkillsFrom(dir)
	if err != nil {
		t.Fatalf("LoadSkillsFrom: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	s := skills[0]
	if s.Name != "code-review" {
		t.Errorf("Name: got %q", s.Name)
	}
	if s.Description != "Review code for issues" {
		t.Errorf("Description: got %q", s.Description)
	}
	if len(s.ContextFiles) != 1 || s.ContextFiles[0] != "~/notes.md" {
		t.Errorf("ContextFiles: got %v", s.ContextFiles)
	}
	if s.SetupScript != "echo ready" {
		t.Errorf("SetupScript: got %q", s.SetupScript)
	}
	if s.Instructions != "You are a code reviewer. Focus on correctness.\n" {
		t.Errorf("Instructions: got %q", s.Instructions)
	}
}

func TestLoadSkills_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	skills, err := config.LoadSkillsFrom(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(skills))
	}
}

func TestLoadSkills_SkipsMalformedYAML(t *testing.T) {
	dir := t.TempDir()
	// malformed frontmatter
	bad := "---\nname: [unclosed\n---\nSome instructions\n"
	if err := os.WriteFile(filepath.Join(dir, "bad.md"), []byte(bad), 0600); err != nil {
		t.Fatal(err)
	}
	// valid skill
	good := "---\nname: good\ndescription: works\n---\nDo the thing.\n"
	if err := os.WriteFile(filepath.Join(dir, "good.md"), []byte(good), 0600); err != nil {
		t.Fatal(err)
	}

	skills, err := config.LoadSkillsFrom(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 1 {
		t.Errorf("expected 1 valid skill, got %d", len(skills))
	}
	if skills[0].Name != "good" {
		t.Errorf("expected 'good', got %q", skills[0].Name)
	}
}
