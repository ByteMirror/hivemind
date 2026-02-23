package session

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ByteMirror/hivemind/config"
)

// ChatWorkspaceState holds the persisted state for a chat agent's workspace.
type ChatWorkspaceState struct {
	Bootstrapped bool `json:"bootstrapped"`
}

const workspaceStateFile = "workspace-state.json"

// GetChatsDir returns the root chats directory (~/.hivemind/chats).
// If the environment variable HIVEMIND_CHATS_DIR_OVERRIDE is set, that path
// is used instead — this is intended for use in tests only.
func GetChatsDir() (string, error) {
	if override := os.Getenv("HIVEMIND_CHATS_DIR_OVERRIDE"); override != "" {
		return override, nil
	}

	configDir, err := config.GetConfigDir()
	if err != nil {
		return "", fmt.Errorf("chat storage: get config dir: %w", err)
	}
	return filepath.Join(configDir, "chats"), nil
}

// GetAgentPersonalityDir returns the directory for a specific chat agent
// identified by slug (~/.hivemind/chats/<slug>).
func GetAgentPersonalityDir(slug string) (string, error) {
	chatsDir, err := GetChatsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(chatsDir, slug), nil
}

// EnsureAgentDir creates the agent directory and writes an initial
// workspace-state.json with {bootstrapped: false} if the file does not
// already exist. It is safe to call multiple times (idempotent).
func EnsureAgentDir(slug string) error {
	agentDir, err := GetAgentPersonalityDir(slug)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(agentDir, 0700); err != nil {
		return fmt.Errorf("chat storage: create agent dir %q: %w", agentDir, err)
	}

	stateFile := filepath.Join(agentDir, workspaceStateFile)
	if _, err := os.Stat(stateFile); err == nil {
		// File already exists — do not overwrite.
		return nil
	}

	initial := ChatWorkspaceState{Bootstrapped: false}
	data, err := json.Marshal(initial)
	if err != nil {
		return fmt.Errorf("chat storage: marshal initial state: %w", err)
	}

	if err := config.AtomicWriteFile(stateFile, data, 0600); err != nil {
		return fmt.Errorf("chat storage: write workspace-state.json: %w", err)
	}

	return nil
}

// ReadWorkspaceState reads and returns the ChatWorkspaceState for the given slug.
func ReadWorkspaceState(slug string) (ChatWorkspaceState, error) {
	agentDir, err := GetAgentPersonalityDir(slug)
	if err != nil {
		return ChatWorkspaceState{}, err
	}

	stateFile := filepath.Join(agentDir, workspaceStateFile)
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return ChatWorkspaceState{}, fmt.Errorf("chat storage: read workspace-state.json for %q: %w", slug, err)
	}

	var state ChatWorkspaceState
	if err := json.Unmarshal(data, &state); err != nil {
		return ChatWorkspaceState{}, fmt.Errorf("chat storage: parse workspace-state.json for %q: %w", slug, err)
	}

	return state, nil
}

// MarkBootstrapped sets Bootstrapped to true in the workspace-state.json for
// the given slug.
func MarkBootstrapped(slug string) error {
	agentDir, err := GetAgentPersonalityDir(slug)
	if err != nil {
		return err
	}

	state := ChatWorkspaceState{Bootstrapped: true}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("chat storage: marshal state for %q: %w", slug, err)
	}

	stateFile := filepath.Join(agentDir, workspaceStateFile)
	if err := config.AtomicWriteFile(stateFile, data, 0600); err != nil {
		return fmt.Errorf("chat storage: write workspace-state.json for %q: %w", slug, err)
	}

	return nil
}

// templateFS holds the embedded personality template files from session/templates/.
//
//go:embed templates/*
var templateFS embed.FS

// CopyTemplatesToAgentDir copies each file from the embedded templates directory
// into the agent's personality directory identified by slug. Files that already
// exist on disk are skipped so that user edits are never overwritten.
func CopyTemplatesToAgentDir(slug string) error {
	agentDir, err := GetAgentPersonalityDir(slug)
	if err != nil {
		return err
	}

	entries, err := templateFS.ReadDir("templates")
	if err != nil {
		return fmt.Errorf("chat storage: read embedded templates: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		dest := filepath.Join(agentDir, entry.Name())

		// Skip if the file already exists — never overwrite user edits.
		if _, err := os.Stat(dest); err == nil {
			continue
		}

		data, err := templateFS.ReadFile("templates/" + entry.Name())
		if err != nil {
			return fmt.Errorf("chat storage: read template %q: %w", entry.Name(), err)
		}

		if err := config.AtomicWriteFile(dest, data, 0600); err != nil {
			return fmt.Errorf("chat storage: write template %q to %q: %w", entry.Name(), dest, err)
		}
	}

	return nil
}
