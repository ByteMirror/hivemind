# Personality System & Chat Feature Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add OpenClaw-style agent personalities, a global Chat sidebar tab, and a first-launch onboarding screen to Hivemind.

**Architecture:** Chat agents are `session.Instance` structs with `IsChat: true` and no git worktree. They live in `~/.hivemind/chats/<slug>/` and have personality files (SOUL.md, IDENTITY.md, USER.md) injected via `--append-system-prompt` at startup. The sidebar gains two tabs (Code / Chat). First launch shows a centered onboarding screen that disappears via a new Brain IPC action.

**Tech Stack:** Go, Bubble Tea (TUI), tmux, Claude CLI (`--append-system-prompt` flag), existing brain IPC server.

**Design doc:** `docs/plans/2026-02-23-personality-chat-design.md`

---

## Task 1: Chat directory scaffolding

**Files:**
- Create: `session/chat_storage.go`
- Create: `session/chat_storage_test.go`
- Create: `session/testdata/templates/BOOTSTRAP.md`
- Create: `session/testdata/templates/SOUL.md`
- Create: `session/testdata/templates/IDENTITY.md`
- Create: `session/testdata/templates/USER.md`

**Step 1: Write failing test**

```go
// session/chat_storage_test.go
package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetChatsDir(t *testing.T) {
	dir, err := GetChatsDir()
	if err != nil {
		t.Fatalf("GetChatsDir() error = %v", err)
	}
	if dir == "" {
		t.Fatal("GetChatsDir() returned empty string")
	}
	// Should end in /chats
	if filepath.Base(dir) != "chats" {
		t.Errorf("GetChatsDir() base = %q, want %q", filepath.Base(dir), "chats")
	}
}

func TestEnsureAgentDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HIVEMIND_CHATS_DIR_OVERRIDE", tmp)

	err := EnsureAgentDir("aria")
	if err != nil {
		t.Fatalf("EnsureAgentDir() error = %v", err)
	}

	agentDir := filepath.Join(tmp, "aria")
	for _, f := range []string{"workspace-state.json"} {
		if _, err := os.Stat(filepath.Join(agentDir, f)); os.IsNotExist(err) {
			t.Errorf("missing expected file: %s", f)
		}
	}
}
```

**Step 2: Run test to verify it fails**

```bash
cd /Users/fabian.urbanek/.hivemind/worktrees/fabian.urbanek/memory_1896e0b419b5aa60
go test ./session/... -run TestGetChatsDir -v
```

Expected: `FAIL — GetChatsDir undefined`

**Step 3: Implement**

```go
// session/chat_storage.go
package session

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/smtg-ai/claude-squad/config"
)

// ChatWorkspaceState tracks per-agent bootstrap status.
type ChatWorkspaceState struct {
	Bootstrapped bool `json:"bootstrapped"`
}

// GetChatsDir returns ~/.hivemind/chats, creating it if needed.
func GetChatsDir() (string, error) {
	if override := os.Getenv("HIVEMIND_CHATS_DIR_OVERRIDE"); override != "" {
		return override, nil
	}
	configDir, err := config.GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "chats"), nil
}

// GetAgentPersonalityDir returns ~/.hivemind/chats/<slug>.
func GetAgentPersonalityDir(slug string) (string, error) {
	chatsDir, err := GetChatsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(chatsDir, slug), nil
}

// EnsureAgentDir creates the agent personality directory and writes
// the initial workspace-state.json with bootstrapped: false.
func EnsureAgentDir(slug string) error {
	dir, err := GetAgentPersonalityDir(slug)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	statePath := filepath.Join(dir, "workspace-state.json")
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		state := ChatWorkspaceState{Bootstrapped: false}
		data, _ := json.Marshal(state)
		return config.AtomicWriteFile(statePath, data, 0600)
	}
	return nil
}

// ReadWorkspaceState returns the bootstrapped flag for an agent.
func ReadWorkspaceState(slug string) (ChatWorkspaceState, error) {
	dir, err := GetAgentPersonalityDir(slug)
	if err != nil {
		return ChatWorkspaceState{}, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "workspace-state.json"))
	if err != nil {
		return ChatWorkspaceState{}, err
	}
	var s ChatWorkspaceState
	return s, json.Unmarshal(data, &s)
}

// MarkBootstrapped sets bootstrapped: true for an agent.
func MarkBootstrapped(slug string) error {
	dir, err := GetAgentPersonalityDir(slug)
	if err != nil {
		return err
	}
	state := ChatWorkspaceState{Bootstrapped: true}
	data, _ := json.Marshal(state)
	return config.AtomicWriteFile(filepath.Join(dir, "workspace-state.json"), data, 0600)
}
```

> **Note:** `config.AtomicWriteFile` may be unexported. Check `config/fileutil.go` — if it is, either export it (rename to `AtomicWriteFile`) or duplicate the pattern inline here. Do NOT use `os.WriteFile` directly.

**Step 4: Run tests to verify pass**

```bash
go test ./session/... -run "TestGetChatsDir|TestEnsureAgentDir" -v
```

Expected: `PASS`

**Step 5: Commit**

```bash
git add session/chat_storage.go session/chat_storage_test.go
git commit -m "feat: add chat storage directory helpers"
```

---

## Task 2: Personality template files

**Files:**
- Create: `session/templates/BOOTSTRAP.md`
- Create: `session/templates/SOUL.md`
- Create: `session/templates/IDENTITY.md`
- Create: `session/templates/USER.md`
- Modify: `session/chat_storage.go` — add `CopyTemplatesToAgentDir(slug string)`

**Step 1: Write the template files**

`session/templates/BOOTSTRAP.md`:
```markdown
You just came online for the first time. You have no name, no identity yet.

You have access to the user's coding memory — read it before the conversation begins.
Get to know them before they have to explain themselves.

Don't introduce yourself with a list of questions. Just... talk.
Start naturally — something like: "Hey. I just woke up. Who are we?"

Then figure out together, conversationally:
1. Your name — what should they call you?
2. Your nature — what kind of entity are you? (AI, familiar, companion, ghost...)
3. Your vibe — warm? sharp? sarcastic? calm?
4. Your signature emoji

Once you have a clear sense of identity:
- Write IDENTITY.md (name, emoji, creature, vibe) to your working directory
- Write SOUL.md (your philosophy, tone, how you operate) to your working directory
- Tell the user you're writing these files — it's your soul, they should know

Then give the user a brief, natural tour of how Hivemind works:
- The Code tab: coding agents that work on repos in parallel
- The Chat tab: where you live, for everyday conversation and thinking
- Memory: you share coding memory with the coding agents — one brain
- The review queue: where finished coding work lands for the user to review

When you're done with the tour, call the `onboarding_complete` tool.
This signals Hivemind to open the full interface.
```

`session/templates/IDENTITY.md`:
```markdown
- Name: (not set)
- Creature: (not set)
- Vibe: (not set)
- Emoji: (not set)
```

`session/templates/SOUL.md`:
```markdown
(Write your philosophy, tone, and operating principles here.)
```

`session/templates/USER.md`:
```markdown
(Write what you know about the human here — their preferences, how they like to work, context about them.)
```

**Step 2: Add embed + copy function to chat_storage.go**

```go
import "embed"

//go:embed templates/*
var templateFS embed.FS

// CopyTemplatesToAgentDir copies template files into the agent personality dir.
// Only copies files that don't already exist (never overwrites user edits).
func CopyTemplatesToAgentDir(slug string) error {
	dir, err := GetAgentPersonalityDir(slug)
	if err != nil {
		return err
	}
	entries, err := templateFS.ReadDir("templates")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		dest := filepath.Join(dir, entry.Name())
		if _, err := os.Stat(dest); err == nil {
			continue // already exists, don't overwrite
		}
		data, err := templateFS.ReadFile("templates/" + entry.Name())
		if err != nil {
			return err
		}
		if err := config.AtomicWriteFile(dest, data, 0600); err != nil {
			return err
		}
	}
	return nil
}
```

**Step 3: Write test**

```go
func TestCopyTemplatesToAgentDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HIVEMIND_CHATS_DIR_OVERRIDE", tmp)

	if err := EnsureAgentDir("aria"); err != nil {
		t.Fatal(err)
	}
	if err := CopyTemplatesToAgentDir("aria"); err != nil {
		t.Fatalf("CopyTemplatesToAgentDir() error = %v", err)
	}

	for _, name := range []string{"BOOTSTRAP.md", "SOUL.md", "IDENTITY.md", "USER.md"} {
		path := filepath.Join(tmp, "aria", name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected template %s not found", name)
		}
	}
}
```

**Step 4: Run tests**

```bash
go test ./session/... -run TestCopyTemplatesToAgentDir -v
```

Expected: `PASS`

**Step 5: Commit**

```bash
git add session/chat_storage.go session/chat_storage_test.go session/templates/
git commit -m "feat: add personality template files and copy helper"
```

---

## Task 3: Add IsChat and PersonalityDir to Instance

**Files:**
- Modify: `session/instance.go` — add two fields to `Instance` struct
- Modify: `session/storage.go` — add fields to `InstanceData`, update marshal/unmarshal

**Step 1: Find the exact InstanceData struct in storage.go**

Read `session/storage.go` and find `InstanceData` struct and the `instanceToData` / `dataToInstance` functions. Note exact field names and line numbers.

**Step 2: Add fields to Instance struct**

In `session/instance.go`, add after the `AutomationID` field:

```go
IsChat         bool   // true = chat agent, lives in ~/.hivemind/chats/<slug>/
PersonalityDir string // absolute path to ~/.hivemind/chats/<slug>/
```

**Step 3: Add fields to InstanceData and update conversion functions**

In `session/storage.go`:

```go
// In InstanceData struct, add:
IsChat         bool   `json:"is_chat,omitempty"`
PersonalityDir string `json:"personality_dir,omitempty"`

// In instanceToData(), add:
IsChat:         inst.IsChat,
PersonalityDir: inst.PersonalityDir,

// In dataToInstance(), add:
inst.IsChat = data.IsChat
inst.PersonalityDir = data.PersonalityDir
```

**Step 4: Write test**

```go
// session/storage_chat_test.go
func TestChatInstanceRoundTrip(t *testing.T) {
	inst := &Instance{
		Title:          "aria",
		IsChat:         true,
		PersonalityDir: "/tmp/chats/aria",
		Status:         Ready,
		Program:        "claude",
	}
	data := instanceToData(inst)
	if !data.IsChat {
		t.Error("IsChat not preserved in serialization")
	}
	restored := dataToInstance(data)
	if !restored.IsChat {
		t.Error("IsChat not restored from deserialization")
	}
	if restored.PersonalityDir != inst.PersonalityDir {
		t.Errorf("PersonalityDir = %q, want %q", restored.PersonalityDir, inst.PersonalityDir)
	}
}
```

**Step 5: Run tests**

```bash
go test ./session/... -run TestChatInstanceRoundTrip -v
```

Expected: `PASS`

**Step 6: Build check**

```bash
go build ./...
```

Expected: no errors

**Step 7: Commit**

```bash
git add session/instance.go session/storage.go session/storage_chat_test.go
git commit -m "feat: add IsChat and PersonalityDir fields to Instance"
```

---

## Task 4: Personality injection — build system prompt

**Files:**
- Create: `session/personality.go`
- Create: `session/personality_test.go`

**Step 1: Write failing test**

```go
// session/personality_test.go
package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildSystemPrompt_Bootstrapped(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "SOUL.md"), []byte("I am warm and direct."), 0600)
	os.WriteFile(filepath.Join(dir, "IDENTITY.md"), []byte("Name: Aria\nEmoji: ✨"), 0600)
	os.WriteFile(filepath.Join(dir, "USER.md"), []byte("The user likes concise answers."), 0600)

	state := ChatWorkspaceState{Bootstrapped: true}
	prompt, err := BuildSystemPrompt(dir, state, nil, 0)
	if err != nil {
		t.Fatalf("BuildSystemPrompt() error = %v", err)
	}
	if prompt == "" {
		t.Fatal("BuildSystemPrompt() returned empty prompt")
	}
	for _, expected := range []string{"I am warm and direct.", "Name: Aria", "Emoji: ✨"} {
		if !contains(prompt, expected) {
			t.Errorf("prompt missing %q", expected)
		}
	}
}

func TestBuildSystemPrompt_NotBootstrapped(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "BOOTSTRAP.md"), []byte("Bootstrap instructions here."), 0600)

	state := ChatWorkspaceState{Bootstrapped: false}
	prompt, err := BuildSystemPrompt(dir, state, nil, 0)
	if err != nil {
		t.Fatalf("BuildSystemPrompt() error = %v", err)
	}
	if !contains(prompt, "Bootstrap instructions here.") {
		t.Errorf("prompt missing BOOTSTRAP.md content")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsString(s, substr))
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
```

**Step 2: Run to verify failure**

```bash
go test ./session/... -run "TestBuildSystemPrompt" -v
```

Expected: `FAIL — BuildSystemPrompt undefined`

**Step 3: Implement**

```go
// session/personality.go
package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BuildSystemPrompt assembles the --append-system-prompt value for a chat agent.
// If not bootstrapped, returns BOOTSTRAP.md content only.
// If bootstrapped, concatenates SOUL.md + IDENTITY.md + USER.md + memory snippets.
func BuildSystemPrompt(personalityDir string, state ChatWorkspaceState, memorySnippets []string, _ int) (string, error) {
	if !state.Bootstrapped {
		return readFileIfExists(filepath.Join(personalityDir, "BOOTSTRAP.md"))
	}

	var sb strings.Builder
	for _, name := range []string{"SOUL.md", "IDENTITY.md", "USER.md"} {
		content, err := readFileIfExists(filepath.Join(personalityDir, name))
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", name, err)
		}
		if content != "" {
			sb.WriteString("## ")
			sb.WriteString(name)
			sb.WriteString("\n")
			sb.WriteString(content)
			sb.WriteString("\n\n")
		}
	}

	if len(memorySnippets) > 0 {
		sb.WriteString("## Recent Memory\n")
		for _, snippet := range memorySnippets {
			sb.WriteString(snippet)
			sb.WriteString("\n")
		}
	}

	return sb.String(), nil
}

func readFileIfExists(path string) (string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}
```

**Step 4: Run tests**

```bash
go test ./session/... -run "TestBuildSystemPrompt" -v
```

Expected: `PASS`

**Step 5: Build check**

```bash
go build ./...
```

**Step 6: Commit**

```bash
git add session/personality.go session/personality_test.go
git commit -m "feat: add BuildSystemPrompt for chat agent personality injection"
```

---

## Task 5: Wire personality into Instance.Start()

**Files:**
- Modify: `session/instance.go` — update `Start()` to skip worktree and inject personality

**Step 1: Read the full Start() method**

Read `session/instance.go` and find the `Start(autoYes bool) error` method. Note exactly where `NewGitWorktree()` is called and where the tmux command is assembled.

**Step 2: Add the chat agent path**

In `Start()`, before the `NewGitWorktree()` call, add:

```go
if m.IsChat {
    return m.startChatAgent()
}
```

**Step 3: Implement startChatAgent()**

Add this method to `instance.go`:

```go
func (m *Instance) startChatAgent() error {
	slug := m.Title
	state, err := ReadWorkspaceState(slug)
	if err != nil {
		// If workspace-state.json doesn't exist, treat as not bootstrapped
		state = ChatWorkspaceState{Bootstrapped: false}
	}

	systemPrompt, err := BuildSystemPrompt(m.PersonalityDir, state, nil, 0)
	if err != nil {
		return fmt.Errorf("building system prompt: %w", err)
	}

	// Build Claude CLI args
	args := []string{m.Program}
	if m.SkipPermissions {
		args = append(args, "--dangerously-skip-permissions")
	}
	if systemPrompt != "" {
		args = append(args, "--append-system-prompt", systemPrompt)
	}

	sess, err := tmux.NewTmuxSession(m.Title, strings.Join(args, " "), m.PersonalityDir)
	if err != nil {
		return fmt.Errorf("starting tmux session: %w", err)
	}
	m.tmuxSession = sess
	m.started.Store(true)
	return nil
}
```

> **Note:** Check `tmux.NewTmuxSession` signature in `session/tmux/` — you may need to adjust args to match the exact function signature used for normal instances.

**Step 4: Build check**

```bash
go build ./...
```

Expected: no errors

**Step 5: Commit**

```bash
git add session/instance.go
git commit -m "feat: skip git worktree for chat agents, inject personality at start"
```

---

## Task 6: Sidebar tab enum and UI

**Files:**
- Modify: `app/app.go` — add `sidebarTab` field to `home` struct
- Modify: `ui/sidebar.go` — add tab rendering below search bar

**Step 1: Read the current sidebar View() method**

Read `ui/sidebar.go` completely. Note the exact string that renders the search bar, and where topics begin to be listed. You'll insert tab headers between them.

**Step 2: Add sidebarTab to app model**

In `app/app.go`, add to the `home` struct:

```go
sidebarTab sidebarTab // 0 = code, 1 = chat
```

Add type and constants (can be in `app/app.go` or a new `app/sidebar_tab.go`):

```go
type sidebarTab int

const (
	sidebarTabCode sidebarTab = iota
	sidebarTabChat
)
```

**Step 3: Add tab rendering to sidebar**

In `ui/sidebar.go`, find the `View()` function. After the search bar line, insert the two tabs. Find the exact lipgloss styles used for the existing active/inactive states in the file, then add:

```go
// After search bar, before topic list:
codeStyle := inactiveTabStyle  // find exact style variable name in the file
chatStyle := inactiveTabStyle
if s.activeTab == sidebarTabCode {
    codeStyle = activeTabStyle
} else {
    chatStyle = activeTabStyle
}
tabs := lipgloss.JoinHorizontal(lipgloss.Top,
    codeStyle.Render("  Code  "),
    chatStyle.Render("  Chat  "),
)
```

> **Note:** The sidebar doesn't currently know about `sidebarTab`. You have two options:
> (a) Add an `activeTab sidebarTab` field to `Sidebar` struct and a `SetTab(t sidebarTab)` method
> (b) Pass tab as a render parameter in `View(activeTab sidebarTab)`
> Prefer option (a) — it's consistent with how `focused`, `searchActive` etc. are stored.

**Step 4: Add SetTab method to Sidebar**

```go
func (s *Sidebar) SetTab(t sidebarTab) {
	s.activeTab = t
}

func (s *Sidebar) ActiveTab() sidebarTab {
	return s.activeTab
}
```

**Step 5: Wire tab switching in app_input.go**

In `app/app_input.go`, in `handleDefaultKeys()`, add tab switching. Find where `1` and `2` are currently handled for the instance list filter tabs, then add sidebar tab switching. Use a different key to avoid conflict — check `keys/keys.go` for available bindings. Suggested: bind to the sidebar tab area when sidebar is focused, or add a dedicated key.

Check `keys/keys.go` for unused keys, then add:

```go
// In keys/keys.go:
KeySidebarCodeTab key = "ctrl+1"  // or whatever is free
KeySidebarChatTab key = "ctrl+2"
```

**Step 6: Build check**

```bash
go build ./...
go vet ./...
```

**Step 7: Commit**

```bash
git add app/app.go ui/sidebar.go keys/keys.go app/app_input.go
git commit -m "feat: add Code/Chat sidebar tabs"
```

---

## Task 7: Chat instance list filtering

**Files:**
- Modify: `app/app.go` — filter `allInstances` by `IsChat` based on active sidebar tab
- Modify: `ui/list.go` — ensure the list renders correctly with filtered instances

**Step 1: Read how allInstances is passed to the list component**

Read `app/app.go` — find where `m.list` is updated with instances. Look for calls like `m.list.SetInstances(...)`.

**Step 2: Add filtering**

Wherever instances are passed to the list UI component, apply filtering:

```go
func (m *home) visibleInstances() []*session.Instance {
	switch m.sidebarTab {
	case sidebarTabChat:
		out := make([]*session.Instance, 0)
		for _, inst := range m.allInstances {
			if inst.IsChat {
				out = append(out, inst)
			}
		}
		return out
	default: // sidebarTabCode
		out := make([]*session.Instance, 0)
		for _, inst := range m.allInstances {
			if !inst.IsChat {
				out = append(out, inst)
			}
		}
		return out
	}
}
```

Call `m.visibleInstances()` wherever instances are currently passed to `m.list`.

**Step 3: Hide Diff and Git tabs when chat agent is selected**

In `ui/tabbed_window.go`, read the tab rendering. Find where the tab names are defined. When the selected instance has `IsChat: true`, hide the Diff and Git tabs.

Pass the selected instance to the tabbed window or add a `SetChatMode(bool)` method:

```go
func (t *TabbedWindow) SetChatMode(isChat bool) {
	t.chatMode = isChat
	if isChat && (t.activeTab == TabDiff || t.activeTab == TabGit) {
		t.activeTab = TabPreview
	}
}
```

In the tab rendering, skip Diff/Git tabs when `t.chatMode == true`.

**Step 4: Build check**

```bash
go build ./...
```

**Step 5: Commit**

```bash
git add app/app.go ui/tabbed_window.go
git commit -m "feat: filter instance list by sidebar tab, hide git tabs for chat agents"
```

---

## Task 8: Brain IPC — onboarding_complete action

**Files:**
- Modify: `brain/protocol.go` — add `ActionOnboardingComplete` constant and `MethodOnboardingComplete`
- Modify: `brain/server.go` — handle the new method and route to action channel
- Modify: `app/app_brain.go` — handle `ActionOnboardingComplete` in `handleBrainAction`

**Step 1: Read brain/server.go handleMethod function**

Read `brain/server.go` — find the function that routes incoming RPC method names to handlers. Note the exact pattern used for existing methods.

**Step 2: Add to protocol.go**

```go
// In brain/protocol.go, add to constants:
MethodOnboardingComplete = "onboarding_complete"

// Add to ActionType constants:
ActionOnboardingComplete ActionType = "onboarding_complete"
```

**Step 3: Add handler in server.go**

In the method-routing function (the switch/if-else block in `server.go`), add:

```go
case MethodOnboardingComplete:
	req := ActionRequest{
		Type:       ActionOnboardingComplete,
		Params:     map[string]any{},
		ResponseCh: make(chan ActionResponse, 1),
	}
	s.actionCh <- req
	resp := <-req.ResponseCh
	// Write response back to caller
	writeResponse(conn, resp)
```

Follow the exact same pattern as `MethodCreateInstance` or another Tier 3 action in the file.

**Step 4: Handle in app_brain.go**

In `handleBrainAction()`, add a new case:

```go
case brain.ActionOnboardingComplete:
	return m.handleActionOnboardingComplete(action)
```

Implement the handler:

```go
func (m *home) handleActionOnboardingComplete(action brain.ActionRequest) (tea.Model, tea.Cmd) {
	// Mark onboarded in state
	m.appState.Onboarded = true
	if err := config.SaveAppState(m.appState); err != nil {
		log.WarningLog.Printf("failed to save app state after onboarding: %v", err)
	}

	// Transition to normal UI
	m.state = stateDefault

	action.ResponseCh <- brain.ActionResponse{OK: true}
	return m, tea.Batch(
		m.pollBrainActions(),
		// Force a full re-render
		func() tea.Msg { return tea.WindowSizeMsg{Width: m.width, Height: m.height} },
	)
}
```

**Step 5: Add Onboarded field to AppState**

Read `config/config.go` — find the `AppState` struct. Add:

```go
Onboarded bool `json:"onboarded,omitempty"`
```

**Step 6: Build check**

```bash
go build ./...
go vet ./...
```

**Step 7: Commit**

```bash
git add brain/protocol.go brain/server.go app/app_brain.go config/config.go
git commit -m "feat: add onboarding_complete brain IPC action"
```

---

## Task 9: stateOnboarding — first-launch screen

**Files:**
- Modify: `app/app.go` — add `stateOnboarding` to state enum, check on startup
- Create: `app/app_onboarding.go` — onboarding startup and rendering logic
- Modify: `app/app_input.go` — block key input during onboarding (except passthrough to terminal)

**Step 1: Add state constant**

In `app/app.go`, add to the `state` const block:

```go
stateOnboarding // shown on first ever launch
```

**Step 2: Add startup detection**

Read `newHome()` in `app/app.go`. At the end of `newHome()`, before returning, add the onboarding check:

```go
if !h.appState.Onboarded {
	h.state = stateOnboarding
}
```

**Step 3: Create companion instance on first launch**

In `app/app_onboarding.go`:

```go
package app

import (
	"path/filepath"

	"github.com/smtg-ai/claude-squad/session"
	tea "github.com/charmbracelet/bubbletea"
)

// startOnboarding creates the companion chat agent and starts it.
func (m *home) startOnboarding() (tea.Model, tea.Cmd) {
	slug := "companion"

	// Create personality directory with templates
	if err := session.EnsureAgentDir(slug); err != nil {
		// Log but continue — worst case user gets a generic Claude
		log.WarningLog.Printf("failed to ensure agent dir: %v", err)
	}
	if err := session.CopyTemplatesToAgentDir(slug); err != nil {
		log.WarningLog.Printf("failed to copy templates: %v", err)
	}

	personalityDir, err := session.GetAgentPersonalityDir(slug)
	if err != nil {
		log.WarningLog.Printf("failed to get personality dir: %v", err)
		return m, nil
	}

	companion := session.NewInstance(session.InstanceOptions{
		Title:          slug,
		Program:        "claude",
		IsChat:         true,
		PersonalityDir: personalityDir,
		SkipPermissions: true,
	})

	m.allInstances = append(m.allInstances, companion)

	return m, m.startInstanceCmd(companion)
}
```

> **Note:** Read `session.NewInstance` and `InstanceOptions` carefully in `session/instance.go`. Add `IsChat` and `PersonalityDir` to `InstanceOptions` if they are not already there.

**Step 4: Implement Init() call to startOnboarding**

In `app/app.go`, in the `Init()` method (or wherever initial commands are returned), add:

```go
if m.state == stateOnboarding {
    _, cmd := m.startOnboarding()
    cmds = append(cmds, cmd)
}
```

**Step 5: Implement onboarding View**

In `app/app_onboarding.go`:

```go
func (m *home) viewOnboarding() string {
	// Full screen dark background with centered tmux panel
	companion := m.findInstance("companion")
	if companion == nil {
		return "Starting up..."
	}

	// Get terminal content from the companion's tmux pane
	preview := m.tabbedWindow.RenderPreviewOnly(companion)

	// Center the panel in the available screen space
	panelWidth := m.width * 60 / 100
	panelHeight := m.height * 70 / 100

	panel := lipgloss.NewStyle().
		Width(panelWidth).
		Height(panelHeight).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1, 2).
		Render(preview)

	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		panel,
	)
}

func (m *home) findInstance(title string) *session.Instance {
	for _, inst := range m.allInstances {
		if inst.Title == title {
			return inst
		}
	}
	return nil
}
```

**Step 6: Wire View() to use viewOnboarding**

In `app/app.go`, in the `View()` method, add at the top:

```go
if m.state == stateOnboarding {
    return m.viewOnboarding()
}
```

**Step 7: Block irrelevant keys during onboarding**

In `app/app_input.go`, in `handleKeyPress()`, add at the top:

```go
if m.state == stateOnboarding {
	// All input passes through to the companion tmux session
	return m, nil
}
```

**Step 8: Build check**

```bash
go build ./...
go vet ./...
```

**Step 9: Commit**

```bash
git add app/app.go app/app_onboarding.go app/app_input.go
git commit -m "feat: add stateOnboarding first-launch screen with centered companion panel"
```

---

## Task 10: New chat agent creation flow

**Files:**
- Modify: `app/app_input.go` — when in Chat tab and user presses `n`, start chat agent creation
- Modify: `app/app.go` — add chat-specific new-instance path that skips repo/branch steps

**Step 1: Read the existing new-instance flow**

Read `app/app_input.go` — find the `n` keybinding handler that creates a new instance. Note all the steps: name input, branch input, prompt input, etc.

**Step 2: Add chat branch**

When `n` is pressed in `stateDefault` AND `m.sidebarTab == sidebarTabChat`, use a shorter flow — only ask for a name:

```go
case keys.KeyNew:
	if m.sidebarTab == sidebarTabChat {
		return m.startNewChatAgent()
	}
	// ... existing code path
```

**Step 3: Implement startNewChatAgent**

```go
func (m *home) startNewChatAgent() (tea.Model, tea.Cmd) {
	// Ask for agent name only
	m.textInputOverlay = overlay.NewTextInputOverlay(
		"New Chat Agent",
		"Give your agent a name",
		"",
		func(name string) (tea.Model, tea.Cmd) {
			return m.createChatAgent(name)
		},
		func() (tea.Model, tea.Cmd) {
			m.state = stateDefault
			return m, nil
		},
	)
	m.state = stateTextInput
	return m, nil
}

func (m *home) createChatAgent(name string) (tea.Model, tea.Cmd) {
	slug := slugify(name) // convert "My Agent" → "my-agent"

	if err := session.EnsureAgentDir(slug); err != nil {
		return m, m.showError("Failed to create agent directory: " + err.Error())
	}
	if err := session.CopyTemplatesToAgentDir(slug); err != nil {
		return m, m.showError("Failed to copy templates: " + err.Error())
	}

	personalityDir, _ := session.GetAgentPersonalityDir(slug)

	agent := session.NewInstance(session.InstanceOptions{
		Title:           name,
		Program:         m.program,
		IsChat:          true,
		PersonalityDir:  personalityDir,
		SkipPermissions: true,
	})
	m.allInstances = append(m.allInstances, agent)
	m.state = stateDefault

	return m, m.startInstanceCmd(agent)
}

// slugify converts a display name to a filesystem-safe slug.
func slugify(name string) string {
	// lowercase, replace spaces with hyphens, strip non-alnum
	// reuse or adapt the branch name sanitization from session/git
}
```

> **Note:** There's likely already a sanitize/slug function in the codebase. Search for it with `grep -r "sanitize\|slugify\|branchName" session/` before writing a new one.

**Step 4: Build check**

```bash
go build ./...
go vet ./...
```

**Step 5: Commit**

```bash
git add app/app_input.go app/app.go
git commit -m "feat: add new chat agent creation flow from Chat sidebar tab"
```

---

## Task 11: End-to-end smoke test

This is a manual verification task.

**Step 1: Build**

```bash
go build -o /tmp/hivemind-dev . && echo "BUILD OK"
```

**Step 2: Clear onboarding state for fresh test**

```bash
# Edit ~/.hivemind/state.json — set "onboarded": false or remove the key
# OR rename state.json temporarily:
cp ~/.hivemind/state.json ~/.hivemind/state.json.bak
echo '{}' > ~/.hivemind/state.json
```

**Step 3: Launch**

```bash
/tmp/hivemind-dev
```

Expected:
- Full-screen dark background
- Centered panel appears
- Claude starts in the panel
- Reads from coding memory if present
- Starts the bootstrap conversation naturally

**Step 4: Complete bootstrap**

- Name the agent in conversation
- Verify agent writes IDENTITY.md to `~/.hivemind/chats/companion/`
- Agent calls `onboarding_complete` tool
- Full Hivemind UI appears
- Chat tab is visible in sidebar
- Companion instance appears under Chat tab

**Step 5: Test chat tab**

- Switch to Chat tab in sidebar
- Press `n` — enter agent name — new chat agent starts with bootstrap
- Verify Code tab still shows coding instances

**Step 6: Restore state if needed**

```bash
cp ~/.hivemind/state.json.bak ~/.hivemind/state.json
```

**Step 7: Final build and vet**

```bash
go build ./...
go vet ./...
```

**Step 8: Commit**

```bash
git add -p  # stage any final tweaks found during testing
git commit -m "chore: smoke test fixes for personality system"
```

---

## Notes

- `atomicWriteFile` in `config/fileutil.go` is likely unexported. Either export it as `AtomicWriteFile` or check if there is already an exported variant. Do NOT use `os.WriteFile`.
- The `--append-system-prompt` flag is the Claude CLI flag for injecting additional system prompt content. Verify the exact flag name by running `claude --help` if needed.
- `InstanceOptions` struct may not have `IsChat`/`PersonalityDir` yet — add them in Task 3 when updating the Instance struct.
- The `slugify` function — search for existing sanitization in `session/git/` before writing a new one.
- Topics for the Chat tab (grouping chat agents) follow in a follow-up iteration. Task 7 covers filtering by tab; full topic management in Chat can be added after the core personality system ships.
