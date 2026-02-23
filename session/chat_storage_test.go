package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGetChatsDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HIVEMIND_CHATS_DIR_OVERRIDE", tmp)

	got, err := GetChatsDir()
	if err != nil {
		t.Fatalf("GetChatsDir() unexpected error: %v", err)
	}
	if got != tmp {
		t.Errorf("GetChatsDir() = %q, want %q", got, tmp)
	}
}

func TestGetChatsDir_Default(t *testing.T) {
	// Ensure override is unset for this sub-test.
	t.Setenv("HIVEMIND_CHATS_DIR_OVERRIDE", "")

	got, err := GetChatsDir()
	if err != nil {
		t.Fatalf("GetChatsDir() unexpected error: %v", err)
	}
	if got == "" {
		t.Error("GetChatsDir() returned empty string")
	}
	// Should end with /.hivemind/chats
	if filepath.Base(got) != "chats" {
		t.Errorf("GetChatsDir() base = %q, want %q", filepath.Base(got), "chats")
	}
}

func TestEnsureAgentDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HIVEMIND_CHATS_DIR_OVERRIDE", tmp)

	const slug = "test-agent"

	if err := EnsureAgentDir(slug); err != nil {
		t.Fatalf("EnsureAgentDir() unexpected error: %v", err)
	}

	agentDir := filepath.Join(tmp, slug)
	info, err := os.Stat(agentDir)
	if err != nil {
		t.Fatalf("agent dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("agent dir is not a directory")
	}

	stateFile := filepath.Join(agentDir, "workspace-state.json")
	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("workspace-state.json not created: %v", err)
	}

	var state ChatWorkspaceState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("failed to parse workspace-state.json: %v", err)
	}
	if state.Bootstrapped {
		t.Error("initial Bootstrapped should be false")
	}
}

func TestEnsureAgentDir_Idempotent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HIVEMIND_CHATS_DIR_OVERRIDE", tmp)

	const slug = "idempotent-agent"

	// Call twice — should not error or overwrite existing state.
	if err := EnsureAgentDir(slug); err != nil {
		t.Fatalf("first EnsureAgentDir() error: %v", err)
	}

	// Mark bootstrapped via the proper API.
	if err := MarkBootstrapped(slug); err != nil {
		t.Fatalf("MarkBootstrapped() error: %v", err)
	}

	// Second call must not overwrite the existing file.
	if err := EnsureAgentDir(slug); err != nil {
		t.Fatalf("second EnsureAgentDir() error: %v", err)
	}

	stateFile := filepath.Join(tmp, slug, "workspace-state.json")
	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("failed to read state file: %v", err)
	}
	var state ChatWorkspaceState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("failed to parse workspace-state.json: %v", err)
	}
	if !state.Bootstrapped {
		t.Error("EnsureAgentDir must not overwrite existing workspace-state.json")
	}
}

func TestReadWorkspaceState(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HIVEMIND_CHATS_DIR_OVERRIDE", tmp)

	const slug = "read-state-agent"

	if err := EnsureAgentDir(slug); err != nil {
		t.Fatalf("EnsureAgentDir() error: %v", err)
	}

	// Initial state should have Bootstrapped = false.
	state, err := ReadWorkspaceState(slug)
	if err != nil {
		t.Fatalf("ReadWorkspaceState() unexpected error: %v", err)
	}
	if state.Bootstrapped {
		t.Error("initial Bootstrapped should be false")
	}

	// Mark bootstrapped and read back.
	if err := MarkBootstrapped(slug); err != nil {
		t.Fatalf("MarkBootstrapped() unexpected error: %v", err)
	}

	state, err = ReadWorkspaceState(slug)
	if err != nil {
		t.Fatalf("ReadWorkspaceState() after MarkBootstrapped error: %v", err)
	}
	if !state.Bootstrapped {
		t.Error("Bootstrapped should be true after MarkBootstrapped()")
	}
}
