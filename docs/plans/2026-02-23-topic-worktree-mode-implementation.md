# Topic Worktree Mode Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace `SharedWorktree bool` on `Topic` with a `TopicWorktreeMode` enum (`per_instance`, `shared`, `main_repo`) and wire it through the full stack so users pick a mode from a 3-item picker when creating a topic.

**Architecture:** Add the enum type and helpers in `session/topic.go`, add a backward-compat JSON migration shim in `session/topic_storage.go`, add `StartInMainRepo()` + `GetWorkingPath()` to the instance lifecycle, then update the app layer (input handler, action dispatchers, state helpers) to use the new mode everywhere `SharedWorktree` was used.

**Tech Stack:** Go, Bubble Tea (TUI), tmux, git worktrees.

---

### Task 1: Add `TopicWorktreeMode` enum to `session/topic.go`

**Files:**
- Modify: `session/topic.go`
- Create: `session/topic_test.go`

**Step 1: Write the failing test**

Create `session/topic_test.go`:

```go
package session

import (
	"testing"
)

func TestTopicWorktreeMode_Helpers(t *testing.T) {
	tests := []struct {
		mode       TopicWorktreeMode
		wantShared bool
		wantMain   bool
	}{
		{TopicWorktreeModePerInstance, false, false},
		{TopicWorktreeModeShared, true, false},
		{TopicWorktreeModeMainRepo, false, true},
	}
	for _, tc := range tests {
		topic := &Topic{WorktreeMode: tc.mode}
		if got := topic.IsSharedWorktree(); got != tc.wantShared {
			t.Errorf("mode %q IsSharedWorktree(): got %v, want %v", tc.mode, got, tc.wantShared)
		}
		if got := topic.IsMainRepo(); got != tc.wantMain {
			t.Errorf("mode %q IsMainRepo(): got %v, want %v", tc.mode, got, tc.wantMain)
		}
	}
}

func TestNewTopic_DefaultMode(t *testing.T) {
	topic := NewTopic(TopicOptions{Name: "t", Path: "/repo"})
	if topic.WorktreeMode != TopicWorktreeModePerInstance {
		t.Errorf("default WorktreeMode: got %q, want %q", topic.WorktreeMode, TopicWorktreeModePerInstance)
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./session/ -run TestTopicWorktreeMode -v
```
Expected: compile error — `TopicWorktreeMode`, `WorktreeMode`, `IsSharedWorktree`, `IsMainRepo` undefined.

**Step 3: Implement in `session/topic.go`**

Replace the current `Topic` struct and related code. The full replacement for the file:

```go
package session

import (
	"fmt"
	"time"

	"github.com/ByteMirror/hivemind/session/git"
)

// TopicTask is a single item in the topic's todo list.
type TopicTask struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Done bool   `json:"done"`
}

// NewTopicTask creates a TopicTask with a unique ID generated from the current time.
func NewTopicTask(text string) TopicTask {
	return TopicTask{
		ID:   fmt.Sprintf("%d", time.Now().UnixNano()),
		Text: text,
	}
}

// TopicWorktreeMode controls how instances in a topic interact with git.
type TopicWorktreeMode string

const (
	// TopicWorktreeModePerInstance gives each instance its own branch + worktree directory.
	TopicWorktreeModePerInstance TopicWorktreeMode = "per_instance"
	// TopicWorktreeModeShared makes all instances share one branch + worktree directory.
	TopicWorktreeModeShared TopicWorktreeMode = "shared"
	// TopicWorktreeModeMainRepo runs instances directly in the repo directory with no worktree.
	TopicWorktreeModeMainRepo TopicWorktreeMode = "main_repo"
)

// Topic groups related instances, optionally sharing a single git worktree.
type Topic struct {
	Name         string
	WorktreeMode TopicWorktreeMode
	AutoYes      bool
	Branch       string
	Path         string
	CreatedAt    time.Time
	Notes        string
	Tasks        []TopicTask
	gitWorktree  *git.GitWorktree
	started      bool
}

// IsSharedWorktree reports whether all instances in this topic share one worktree.
func (t *Topic) IsSharedWorktree() bool {
	return t.WorktreeMode == TopicWorktreeModeShared
}

// IsMainRepo reports whether instances in this topic run directly in the repo directory.
func (t *Topic) IsMainRepo() bool {
	return t.WorktreeMode == TopicWorktreeModeMainRepo
}

type TopicOptions struct {
	Name         string
	WorktreeMode TopicWorktreeMode
	Path         string
}

func NewTopic(opts TopicOptions) *Topic {
	mode := opts.WorktreeMode
	if mode == "" {
		mode = TopicWorktreeModePerInstance
	}
	return &Topic{
		Name:         opts.Name,
		WorktreeMode: mode,
		Path:         opts.Path,
		CreatedAt:    time.Now(),
	}
}

func (t *Topic) Setup() error {
	if t.WorktreeMode != TopicWorktreeModeShared {
		t.started = true
		return nil
	}
	gitWorktree, branchName, err := git.NewGitWorktree(t.Path, t.Name)
	if err != nil {
		return fmt.Errorf("failed to create topic worktree: %w", err)
	}
	if err := gitWorktree.Setup(); err != nil {
		return fmt.Errorf("failed to setup topic worktree: %w", err)
	}
	t.gitWorktree = gitWorktree
	t.Branch = branchName
	t.started = true
	return nil
}

func (t *Topic) GetWorktreePath() string {
	if t.gitWorktree == nil {
		return ""
	}
	return t.gitWorktree.GetWorktreePath()
}

func (t *Topic) GetGitWorktree() *git.GitWorktree {
	return t.gitWorktree
}

func (t *Topic) Started() bool {
	return t.started
}

func (t *Topic) Cleanup() error {
	if t.gitWorktree == nil {
		return nil
	}
	return t.gitWorktree.Cleanup()
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./session/ -run TestTopicWorktreeMode -v
go test ./session/ -run TestNewTopic_DefaultMode -v
```
Expected: PASS both.

**Step 5: Verify nothing else broke**

```bash
go build ./...
```
Expected: compile errors in `topic_storage.go`, `app/app_input.go`, etc. — that's expected and will be fixed in subsequent tasks.

**Step 6: Commit**

```bash
git add session/topic.go session/topic_test.go
git commit -m "feat: add TopicWorktreeMode enum replacing SharedWorktree bool"
```

---

### Task 2: Storage migration shim in `session/topic_storage.go`

**Files:**
- Modify: `session/topic_storage.go`
- Create: `session/topic_storage_test.go`

**Step 1: Write the failing tests**

Create `session/topic_storage_test.go`:

```go
package session

import (
	"testing"
	"time"
)

func TestFromTopicData_MigratesLegacySharedWorktree(t *testing.T) {
	// Old JSON had shared_worktree:true — should become Shared mode
	data := TopicData{
		Name:           "my-topic",
		SharedWorktree: true,
		Path:           "/repo",
		CreatedAt:      time.Now(),
	}
	topic := FromTopicData(data)
	if topic.WorktreeMode != TopicWorktreeModeShared {
		t.Errorf("legacy shared_worktree:true → got %q, want %q", topic.WorktreeMode, TopicWorktreeModeShared)
	}
}

func TestFromTopicData_MigratesLegacyNonShared(t *testing.T) {
	// Old JSON had shared_worktree:false — should become PerInstance mode
	data := TopicData{
		Name:           "my-topic",
		SharedWorktree: false,
		Path:           "/repo",
		CreatedAt:      time.Now(),
	}
	topic := FromTopicData(data)
	if topic.WorktreeMode != TopicWorktreeModePerInstance {
		t.Errorf("legacy shared_worktree:false → got %q, want %q", topic.WorktreeMode, TopicWorktreeModePerInstance)
	}
}

func TestFromTopicData_UsesExplicitWorktreeMode(t *testing.T) {
	// New JSON has worktree_mode set — should be used directly, ignoring shared_worktree
	data := TopicData{
		Name:         "my-topic",
		WorktreeMode: TopicWorktreeModeMainRepo,
		Path:         "/repo",
		CreatedAt:    time.Now(),
	}
	topic := FromTopicData(data)
	if topic.WorktreeMode != TopicWorktreeModeMainRepo {
		t.Errorf("explicit worktree_mode → got %q, want %q", topic.WorktreeMode, TopicWorktreeModeMainRepo)
	}
}

func TestToTopicData_RoundTrip(t *testing.T) {
	original := NewTopic(TopicOptions{
		Name:         "round-trip",
		WorktreeMode: TopicWorktreeModeMainRepo,
		Path:         "/repo",
	})
	data := original.ToTopicData()
	if data.WorktreeMode != TopicWorktreeModeMainRepo {
		t.Errorf("ToTopicData WorktreeMode: got %q, want %q", data.WorktreeMode, TopicWorktreeModeMainRepo)
	}
	restored := FromTopicData(data)
	if restored.WorktreeMode != TopicWorktreeModeMainRepo {
		t.Errorf("round-trip WorktreeMode: got %q, want %q", restored.WorktreeMode, TopicWorktreeModeMainRepo)
	}
}
```

**Step 2: Run to verify they fail**

```bash
go test ./session/ -run TestFromTopicData -v
go test ./session/ -run TestToTopicData -v
```
Expected: compile errors — `WorktreeMode` field missing from `TopicData`.

**Step 3: Replace `session/topic_storage.go`**

```go
package session

import (
	"time"

	"github.com/ByteMirror/hivemind/session/git"
)

// TopicData represents the serializable data of a Topic.
type TopicData struct {
	Name         string            `json:"name"`
	WorktreeMode TopicWorktreeMode `json:"worktree_mode,omitempty"`
	// SharedWorktree is the legacy field kept for reading old JSON files.
	// New topics write worktree_mode instead. FromTopicData migrates this automatically.
	SharedWorktree bool            `json:"shared_worktree"`
	AutoYes        bool            `json:"auto_yes"`
	Branch         string          `json:"branch,omitempty"`
	Path           string          `json:"path"`
	CreatedAt      time.Time       `json:"created_at"`
	Worktree       GitWorktreeData `json:"worktree,omitempty"`
	Notes          string          `json:"notes,omitempty"`
	Tasks          []TopicTask     `json:"tasks,omitempty"`
}

// ToTopicData converts a Topic to its serializable form.
func (t *Topic) ToTopicData() TopicData {
	data := TopicData{
		Name:         t.Name,
		WorktreeMode: t.WorktreeMode,
		AutoYes:      t.AutoYes,
		Branch:       t.Branch,
		Path:         t.Path,
		CreatedAt:    t.CreatedAt,
		Notes:        t.Notes,
		Tasks:        t.Tasks,
	}
	if t.gitWorktree != nil {
		data.Worktree = GitWorktreeData{
			RepoPath:      t.gitWorktree.GetRepoPath(),
			WorktreePath:  t.gitWorktree.GetWorktreePath(),
			SessionName:   t.Name,
			BranchName:    t.gitWorktree.GetBranchName(),
			BaseCommitSHA: t.gitWorktree.GetBaseCommitSHA(),
		}
	}
	return data
}

// FromTopicData creates a Topic from serialized data.
// It migrates the legacy shared_worktree bool field when worktree_mode is absent.
func FromTopicData(data TopicData) *Topic {
	mode := data.WorktreeMode
	if mode == "" {
		// Migrate legacy bool field
		if data.SharedWorktree {
			mode = TopicWorktreeModeShared
		} else {
			mode = TopicWorktreeModePerInstance
		}
	}

	topic := &Topic{
		Name:         data.Name,
		WorktreeMode: mode,
		AutoYes:      data.AutoYes,
		Branch:       data.Branch,
		Path:         data.Path,
		CreatedAt:    data.CreatedAt,
		Notes:        data.Notes,
		Tasks:        data.Tasks,
		started:      true,
	}
	if mode == TopicWorktreeModeShared && data.Worktree.WorktreePath != "" {
		topic.gitWorktree = git.NewGitWorktreeFromStorage(
			data.Worktree.RepoPath,
			data.Worktree.WorktreePath,
			data.Worktree.SessionName,
			data.Worktree.BranchName,
			data.Worktree.BaseCommitSHA,
		)
	}
	return topic
}
```

**Step 4: Run tests**

```bash
go test ./session/ -run TestFromTopicData -v
go test ./session/ -run TestToTopicData -v
```
Expected: all PASS.

**Step 5: Run all session tests**

```bash
go test ./session/...
```
Expected: all pass.

**Step 6: Commit**

```bash
git add session/topic_storage.go session/topic_storage_test.go
git commit -m "feat: add worktree_mode field to TopicData with legacy migration shim"
```

---

### Task 3: Add `GetWorkingPath()` and `mainRepo` to Instance

**Files:**
- Modify: `session/instance.go` (add `mainRepo bool` field)
- Modify: `session/instance_session.go` (add `GetWorkingPath()`)
- Modify: `session/instance_test.go` (add test)

**Step 1: Write the failing test**

Add to `session/instance_test.go`:

```go
func TestInstance_GetWorkingPath(t *testing.T) {
	t.Run("returns repo path when no worktree", func(t *testing.T) {
		inst := &Instance{Path: "/my/repo"}
		inst.started.Store(true)
		if got := inst.GetWorkingPath(); got != "/my/repo" {
			t.Errorf("got %q, want /my/repo", got)
		}
	})
}
```

**Step 2: Verify it fails**

```bash
go test ./session/ -run TestInstance_GetWorkingPath -v
```
Expected: compile error — `GetWorkingPath` undefined.

**Step 3: Add field and method**

In `session/instance.go`, add `mainRepo bool` alongside `sharedWorktree bool`:
```go
// mainRepo is true if this instance runs directly in the repo directory (no worktree).
mainRepo bool
```

In `session/instance_session.go`, add after `GetGitWorktree()`:
```go
// GetWorkingPath returns the working directory for this instance.
// For instances with a git worktree, this is the worktree path.
// For main-repo instances, this is the repo path.
func (i *Instance) GetWorkingPath() string {
	if i.gitWorktree != nil {
		return i.gitWorktree.GetWorktreePath()
	}
	return i.Path
}
```

**Step 4: Run tests**

```bash
go test ./session/ -run TestInstance_GetWorkingPath -v
```
Expected: PASS.

**Step 5: Commit**

```bash
git add session/instance.go session/instance_session.go session/instance_test.go
git commit -m "feat: add mainRepo flag and GetWorkingPath() to Instance"
```

---

### Task 4: Add `StartInMainRepo()` and update `Pause()`/`Resume()`

**Files:**
- Modify: `session/instance_lifecycle.go`

No unit tests for this task — lifecycle methods depend on tmux and are integration-level. Build verification and manual test cover it.

**Step 1: Add `StartInMainRepo()` method**

Add after `StartInSharedWorktree()` in `session/instance_lifecycle.go`:

```go
// StartInMainRepo starts the instance directly in the repository directory.
// Unlike Start(), this does NOT create a git worktree or a new branch.
func (i *Instance) StartInMainRepo() error {
	if i.Title == "" {
		return ErrTitleEmpty
	}

	i.LoadingTotal = 5
	i.LoadingMessage = "Initializing..."
	i.setLoadingProgress(1, "Preparing session...")

	i.mainRepo = true

	var tmuxSession *tmux.TmuxSession
	if i.tmuxSession != nil {
		tmuxSession = i.tmuxSession
	} else {
		tmuxSession = tmux.NewTmuxSession(i.Title, i.Program, i.SkipPermissions)
	}
	tmuxSession.ProgressFunc = func(stage int, desc string) {
		i.setLoadingProgress(1+stage, desc)
	}
	i.tmuxSession = tmuxSession

	if isClaudeProgram(i.Program) {
		repoPath := i.Path
		title := i.Title
		go func() {
			if err := registerMCPServer(repoPath, repoPath, title); err != nil {
				log.WarningLog.Printf("failed to write MCP config: %v", err)
			}
		}()
	}

	var setupErr error
	defer func() {
		if setupErr != nil {
			if cleanupErr := i.Kill(); cleanupErr != nil {
				setupErr = fmt.Errorf("%v (cleanup error: %v)", setupErr, cleanupErr)
			}
		} else {
			i.started.Store(true)
		}
	}()

	i.setLoadingProgress(3, "Starting tmux session...")
	if err := i.tmuxSession.Start(i.Path); err != nil {
		setupErr = fmt.Errorf("failed to start session in main repo: %w", err)
		return setupErr
	}

	i.SetStatus(Running)
	return nil
}
```

**Step 2: Guard `Pause()` against nil worktree**

In `Pause()`, the existing `!i.sharedWorktree` guard needs a `!i.mainRepo` guard too. Find this block:

```go
	if !i.sharedWorktree {
		// Check if there are any changes to commit
		if dirty, err := i.gitWorktree.IsDirty(); err != nil {
```

Change the guard to:

```go
	if !i.sharedWorktree && !i.mainRepo {
		// Check if there are any changes to commit
		if dirty, err := i.gitWorktree.IsDirty(); err != nil {
```

Also find the second `!i.sharedWorktree` block in `Pause()`:

```go
	if !i.sharedWorktree {
		// Check if worktree exists before trying to remove it
		if _, err := os.Stat(i.gitWorktree.GetWorktreePath()); err == nil {
```

Change to:

```go
	if !i.sharedWorktree && !i.mainRepo {
		// Check if worktree exists before trying to remove it
		if _, err := os.Stat(i.gitWorktree.GetWorktreePath()); err == nil {
```

Also fix the `clipboard.WriteAll` call at the end of `Pause()`:

```go
	_ = clipboard.WriteAll(i.gitWorktree.GetBranchName())
```

Wrap it:

```go
	if i.gitWorktree != nil {
		_ = clipboard.WriteAll(i.gitWorktree.GetBranchName())
	}
```

**Step 3: Guard `Resume()` against nil worktree**

At the top of `Resume()`, add an early return for main-repo instances since there's no worktree to restore:

```go
func (i *Instance) Resume() error {
	if !i.started.Load() {
		return ErrInstanceNotStarted
	}
	if i.Status != Paused && i.Status != Loading {
		return fmt.Errorf("can only resume paused instances")
	}

	i.tmuxDead.Store(false)

	// Main-repo instances have no worktree to set up; just restart the tmux session.
	if i.mainRepo {
		i.LoadingTotal = 2
		i.setLoadingProgress(1, "Restoring session...")
		if i.tmuxSession.DoesSessionExist() {
			if err := i.tmuxSession.Restore(); err != nil {
				if err := i.tmuxSession.Start(i.Path); err != nil {
					return fmt.Errorf("failed to restart main-repo session: %w", err)
				}
			}
		} else {
			if err := i.tmuxSession.Start(i.Path); err != nil {
				return fmt.Errorf("failed to restart main-repo session: %w", err)
			}
		}
		i.setLoadingProgress(2, "Ready")
		i.SetStatus(Running)
		return nil
	}

	// ... rest of existing Resume() code unchanged ...
```

**Step 4: Build to verify**

```bash
go build ./...
```
Expected: compile errors only in `app/` (still references `SharedWorktree`). No errors in `session/`.

**Step 5: Commit**

```bash
git add session/instance_lifecycle.go
git commit -m "feat: add StartInMainRepo() and guard Pause/Resume against nil worktree"
```

---

### Task 5: Update `app/app_state.go`

**Files:**
- Modify: `app/app_state.go`

Two changes:
1. `topicMeta()` — use `IsSharedWorktree()` instead of `.SharedWorktree`
2. All `selected.GetGitWorktree()` callers — switch to `selected.GetWorkingPath()`

**Step 1: Update `topicMeta()`**

Find:
```go
		if t.SharedWorktree {
			shared[t.Name] = true
		}
```

Replace with:
```go
		if t.IsSharedWorktree() {
			shared[t.Name] = true
		}
```

**Step 2: Replace `GetGitWorktree()` callers with `GetWorkingPath()`**

There are five locations. Each has the pattern:
```go
worktree, err := selected.GetGitWorktree()
if err != nil {
    return m.handleError(err)
}
// ... then uses worktree.GetWorktreePath()
```

Replace each with a direct call using `GetWorkingPath()`. For example:

In `enterGitFocusMode()` (~line 171):
```go
// BEFORE:
worktree, err := selected.GetGitWorktree()
if err != nil {
    return m.handleError(err)
}
gitPane.Attach(worktree.GetWorktreePath(), selected.Title)

// AFTER:
gitPane.Attach(selected.GetWorkingPath(), selected.Title)
```

In `enterTerminalFocusMode()` (~line 197):
```go
// BEFORE:
worktree, err := selected.GetGitWorktree()
if err != nil {
    return m.handleError(err)
}
termPane.Attach(worktree.GetWorktreePath(), selected.Title)

// AFTER:
termPane.Attach(selected.GetWorkingPath(), selected.Title)
```

In `openFileInTerminal()` (~line 231):
```go
// BEFORE:
worktree, err := selected.GetGitWorktree()
if err != nil {
    return m, m.handleError(err)
}
fullPath := filepath.Join(worktree.GetWorktreePath(), relativePath)
// ...
termPane.Attach(worktree.GetWorktreePath(), selected.Title)

// AFTER:
workingPath := selected.GetWorkingPath()
fullPath := filepath.Join(workingPath, relativePath)
// ...
termPane.Attach(workingPath, selected.Title)
```

In `attachGitTab()` (~line 851):
```go
// BEFORE:
worktree, err := selected.GetGitWorktree()
if err != nil {
    return m.handleError(err)
}
gitPane.Attach(worktree.GetWorktreePath(), selected.Title)

// AFTER:
gitPane.Attach(selected.GetWorkingPath(), selected.Title)
```

In `spawnTerminalTab()` (~line 869):
```go
// BEFORE:
worktree, err := selected.GetGitWorktree()
if err != nil {
    return m.handleError(err)
}
termPane.Attach(worktree.GetWorktreePath(), selected.Title)

// AFTER:
termPane.Attach(selected.GetWorkingPath(), selected.Title)
```

**Step 3: Build**

```bash
go build ./...
```
Expected: remaining compile errors only in `app/app_input.go`, `app/app_actions.go`, `app/app_brain.go`.

**Step 4: Commit**

```bash
git add app/app_state.go
git commit -m "refactor: use IsSharedWorktree() and GetWorkingPath() in app_state"
```

---

### Task 6: Update topic creation UI in `app/app_input.go`

**Files:**
- Modify: `app/app_input.go`

Four changes:
1. `handleNewTopicKeys` — replace `ConfirmationOverlay` with `PickerOverlay`
2. `handleNewTopicConfirmKeys` — replace Y/N logic with picker value → mode mapping
3. Instance start dispatch (`~line 488`) — add `MainRepo` branch
4. `SharedWorktree` checks → `IsSharedWorktree()`

**Step 1: Update `handleNewTopicKeys`**

Find:
```go
		// Show shared worktree confirmation
		m.textInputOverlay = nil
		m.confirmationOverlay = overlay.NewConfirmationOverlay(
			fmt.Sprintf("Create shared worktree for topic '%s'?\nAll instances will share one branch and directory.", m.pendingTopicName),
		)
		m.confirmationOverlay.SetWidth(60)
		m.state = stateNewTopicConfirm
		return m, nil
```

Replace with:
```go
		// Show worktree mode picker
		m.textInputOverlay = nil
		m.pickerOverlay = overlay.NewPickerOverlay(
			fmt.Sprintf("Worktree mode for '%s'", m.pendingTopicName),
			[]string{
				"Per-instance worktrees",
				"Shared worktree",
				"Main repo (no worktree)",
			},
		)
		m.state = stateNewTopicConfirm
		return m, nil
```

**Step 2: Replace `handleNewTopicConfirmKeys`**

Replace the entire function:

```go
func (m *home) handleNewTopicConfirmKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.pickerOverlay == nil {
		m.state = stateDefault
		return m, nil
	}
	shouldClose := m.pickerOverlay.HandleKeyPress(msg)
	if !shouldClose {
		return m, nil
	}

	if !m.pickerOverlay.IsSubmitted() {
		// Cancelled
		m.pickerOverlay = nil
		m.pendingTopicName = ""
		m.pendingTopicRepoPath = ""
		m.state = stateDefault
		m.menu.SetState(ui.StateDefault)
		return m, tea.WindowSize()
	}

	var mode session.TopicWorktreeMode
	switch m.pickerOverlay.Value() {
	case "Shared worktree":
		mode = session.TopicWorktreeModeShared
	case "Main repo (no worktree)":
		mode = session.TopicWorktreeModeMainRepo
	default:
		mode = session.TopicWorktreeModePerInstance
	}
	m.pickerOverlay = nil

	topicRepoPath := m.pendingTopicRepoPath
	if topicRepoPath == "" {
		topicRepoPath = m.activeRepoPaths[0]
	}
	topic := session.NewTopic(session.TopicOptions{
		Name:         m.pendingTopicName,
		WorktreeMode: mode,
		Path:         topicRepoPath,
	})
	if err := topic.Setup(); err != nil {
		m.pendingTopicName = ""
		m.state = stateDefault
		m.menu.SetState(ui.StateDefault)
		return m, m.handleError(err)
	}
	m.allTopics = append(m.allTopics, topic)
	m.topics = append(m.topics, topic)
	m.updateSidebarItems()
	if err := m.saveAllTopics(); err != nil {
		return m, m.handleError(err)
	}
	m.pendingTopicName = ""
	m.pendingTopicRepoPath = ""
	m.state = stateDefault
	m.menu.SetState(ui.StateDefault)
	return m, tea.WindowSize()
}
```

**Step 3: Update instance start dispatch (~line 488)**

Find:
```go
		startCmd := func() tea.Msg {
			var startErr error
			if topic != nil && topic.SharedWorktree && topic.Started() {
				startErr = instance.StartInSharedWorktree(topic.GetGitWorktree(), topic.Branch)
			} else {
				startErr = instance.Start(true)
			}
```

Replace with:
```go
		startCmd := func() tea.Msg {
			var startErr error
			switch {
			case topic != nil && topic.IsSharedWorktree() && topic.Started():
				startErr = instance.StartInSharedWorktree(topic.GetGitWorktree(), topic.Branch)
			case topic != nil && topic.IsMainRepo():
				startErr = instance.StartInMainRepo()
			default:
				startErr = instance.Start(true)
			}
```

**Step 4: Update remaining `SharedWorktree` checks in this file**

Find (context menu, ~line 267):
```go
		if topic.SharedWorktree {
			items = append(items, overlay.ContextMenuItem{Label: "Push branch", Action: "push_topic"})
		}
```
Replace with:
```go
		if topic.IsSharedWorktree() {
			items = append(items, overlay.ContextMenuItem{Label: "Push branch", Action: "push_topic"})
		}
```

Find (move prevention, ~line 1554):
```go
			if t.Name == selected.TopicName && t.SharedWorktree {
				return m, m.handleError(fmt.Errorf("cannot move instances in shared-worktree topics"))
```
Replace with:
```go
			if t.Name == selected.TopicName && t.IsSharedWorktree() {
				return m, m.handleError(fmt.Errorf("cannot move instances in shared-worktree topics"))
```

**Step 5: Build**

```bash
go build ./...
```
Expected: errors only in `app/app_actions.go` and `app/app_brain.go`.

**Step 6: Commit**

```bash
git add app/app_input.go
git commit -m "feat: replace Y/N worktree confirm with 3-way mode picker, wire MainRepo dispatch"
```

---

### Task 7: Update `app/app_actions.go` and `app/app_brain.go`

**Files:**
- Modify: `app/app_actions.go`
- Modify: `app/app_brain.go`

**Step 1: `app/app_actions.go`**

Find (~line 302):
```go
		if topic.SharedWorktree {
			items = append(items, overlay.ContextMenuItem{Label: "Push branch", Action: "push_topic"})
		}
```
Replace with:
```go
		if topic.IsSharedWorktree() {
			items = append(items, overlay.ContextMenuItem{Label: "Push branch", Action: "push_topic"})
		}
```

**Step 2: `app/app_brain.go`**

Find (~line 134):
```go
		if topicObj != nil && topicObj.SharedWorktree && topicObj.Started() {
			startErr = instance.StartInSharedWorktree(topicObj.GetGitWorktree(), topicObj.Branch)
		} else {
			startErr = instance.Start(true)
		}
```
Replace with:
```go
		switch {
		case topicObj != nil && topicObj.IsSharedWorktree() && topicObj.Started():
			startErr = instance.StartInSharedWorktree(topicObj.GetGitWorktree(), topicObj.Branch)
		case topicObj != nil && topicObj.IsMainRepo():
			startErr = instance.StartInMainRepo()
		default:
			startErr = instance.Start(true)
		}
```

**Step 3: Build everything cleanly**

```bash
go build ./...
```
Expected: zero errors.

**Step 4: Run all tests**

```bash
go test ./...
```
Expected: all pass.

**Step 5: Commit**

```bash
git add app/app_actions.go app/app_brain.go
git commit -m "refactor: update remaining SharedWorktree refs in actions and brain"
```

---

### Task 8: Final verification

**Step 1: Full build + test**

```bash
go build ./... && go test ./... && go vet ./...
```
Expected: zero errors, zero failures, zero vet warnings.

**Step 2: Manual smoke test**

1. Run hivemind: `go run . --path /tmp/test-repo` (or any git repo)
2. Press `T` to create a new topic
3. Enter a topic name, press Enter
4. Verify the picker shows three options: "Per-instance worktrees", "Shared worktree", "Main repo (no worktree)"
5. Select "Main repo (no worktree)", press Enter — topic created
6. Press `N` to create an instance in that topic
7. Verify the instance starts in the repo directory (not a worktree path)
8. Repeat for "Per-instance worktrees" and "Shared worktree" to confirm existing behaviour unchanged
9. Kill hivemind, re-launch — verify topics survive restart with correct mode (migration works)

**Step 3: Final commit**

```bash
git add -A
git commit -m "chore: final verification pass for topic worktree mode feature"
```
