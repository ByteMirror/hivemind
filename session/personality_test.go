package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildSystemPrompt_Bootstrapped(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SOUL.md"), []byte("soul content"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "IDENTITY.md"), []byte("identity content"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "USER.md"), []byte("user content"), 0600); err != nil {
		t.Fatal(err)
	}

	state := ChatWorkspaceState{Bootstrapped: true}
	prompt, err := BuildSystemPrompt(dir, state, nil, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(prompt, "soul content") {
		t.Error("expected prompt to contain SOUL.md content")
	}
	if !strings.Contains(prompt, "identity content") {
		t.Error("expected prompt to contain IDENTITY.md content")
	}
	if !strings.Contains(prompt, "user content") {
		t.Error("expected prompt to contain USER.md content")
	}
	if !strings.Contains(prompt, "## SOUL.md") {
		t.Error("expected prompt to contain SOUL.md section header")
	}
	if !strings.Contains(prompt, "## IDENTITY.md") {
		t.Error("expected prompt to contain IDENTITY.md section header")
	}
	if !strings.Contains(prompt, "## USER.md") {
		t.Error("expected prompt to contain USER.md section header")
	}
}

func TestBuildSystemPrompt_NotBootstrapped(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "BOOTSTRAP.md"), []byte("bootstrap content"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SOUL.md"), []byte("soul content"), 0600); err != nil {
		t.Fatal(err)
	}

	state := ChatWorkspaceState{Bootstrapped: false}
	prompt, err := BuildSystemPrompt(dir, state, nil, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(prompt, "bootstrap content") {
		t.Error("expected prompt to contain BOOTSTRAP.md content")
	}
	if strings.Contains(prompt, "soul content") {
		t.Error("expected prompt NOT to contain SOUL.md content when not bootstrapped")
	}
}

func TestBuildSystemPrompt_WithMemory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SOUL.md"), []byte("soul content"), 0600); err != nil {
		t.Fatal(err)
	}

	state := ChatWorkspaceState{Bootstrapped: true}
	snippets := []string{"snippet1", "snippet2"}
	prompt, err := BuildSystemPrompt(dir, state, snippets, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(prompt, "## Recent Memory") {
		t.Error("expected prompt to contain '## Recent Memory' section")
	}
	if !strings.Contains(prompt, "snippet1") {
		t.Error("expected prompt to contain 'snippet1'")
	}
	if !strings.Contains(prompt, "snippet2") {
		t.Error("expected prompt to contain 'snippet2'")
	}
}

func TestBuildSystemPrompt_MissingFiles(t *testing.T) {
	dir := t.TempDir()

	state := ChatWorkspaceState{Bootstrapped: true}
	prompt, err := BuildSystemPrompt(dir, state, nil, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if prompt != "" {
		t.Errorf("expected empty prompt when all files missing, got: %q", prompt)
	}
}
