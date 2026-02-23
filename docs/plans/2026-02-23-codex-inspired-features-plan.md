# Codex-Inspired Features Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add Review Queue, Inline Diff Comments, Skills/Task Templates, and Automations to Hivemind — matching Codex's UX quality.

**Architecture:** Four milestones in dependency order: M1 (Review Queue) and M3 (Skills) are independent and can be built in parallel; M2 (Inline Diff Comments) refines the review workflow; M4 (Automations) consumes both M1 and M3. All features are additive — no existing behaviour changes.

**Tech Stack:** Go, Bubble Tea (TUI), lipgloss (styling), gopkg.in/yaml.v3 (skill frontmatter parsing), existing `ui/overlay/` components, existing `session/tmux` send-keys infrastructure, existing `daemon/` tick loop.

---

## Milestone 1: Review Queue

Automation-triggered instances that finish land in a dedicated "Review Queue" section of the sidebar. The user can commit, create a PR, send feedback back to the agent, checkout, or discard — all without leaving the TUI.

---

### Task 1.1: Add review fields to `InstanceData` (serialization struct)

**Files:**
- Modify: `session/storage.go:12-28`

**Step 1: Add fields to `InstanceData`**

In `session/storage.go`, add three fields to the `InstanceData` struct after `ParentTitle`:

```go
AutomationID  string     `json:"automation_id,omitempty"`
PendingReview bool       `json:"pending_review,omitempty"`
CompletedAt   *time.Time `json:"completed_at,omitempty"`
```

**Step 2: Build**

```bash
go build ./...
```

Expected: compiles cleanly.

**Step 3: Commit**

```bash
git add session/storage.go
git commit -m "feat(review-queue): add AutomationID/PendingReview/CompletedAt to InstanceData"
```

---

### Task 1.2: Add review fields to `Instance` and wire serialization

**Files:**
- Modify: `session/instance.go` (Instance struct, InstanceOptions, ToInstanceData, FromInstanceData, NewInstance)
- Test: `session/instance_test.go`

**Step 1: Write the failing test**

Add to `session/instance_test.go` (create file if it doesn't exist):

```go
func TestInstance_ReviewFields_RoundTrip(t *testing.T) {
    now := time.Now().Truncate(time.Second)
    inst := &Instance{
        Title:        "test",
        AutomationID: "auto-1",
        PendingReview: true,
        CompletedAt:  &now,
    }
    data := inst.ToInstanceData()

    if data.AutomationID != "auto-1" {
        t.Errorf("AutomationID: got %q, want %q", data.AutomationID, "auto-1")
    }
    if !data.PendingReview {
        t.Error("PendingReview: expected true")
    }
    if data.CompletedAt == nil || !data.CompletedAt.Equal(now) {
        t.Errorf("CompletedAt: got %v, want %v", data.CompletedAt, now)
    }

    restored, err := FromInstanceData(data)
    if err != nil {
        t.Fatalf("FromInstanceData: %v", err)
    }
    if restored.AutomationID != "auto-1" {
        t.Errorf("restored AutomationID: got %q", restored.AutomationID)
    }
    if !restored.PendingReview {
        t.Error("restored PendingReview: expected true")
    }
}
```

**Step 2: Run to verify it fails**

```bash
go test ./session/... -run TestInstance_ReviewFields_RoundTrip -v
```

Expected: compile error — `AutomationID` not defined on `Instance`.

**Step 3: Add fields to `Instance` struct**

In `session/instance.go`, add after `ParentTitle string`:

```go
// AutomationID is set when this instance was spawned by an automation.
// Empty for manually-created instances.
AutomationID string
// PendingReview is true when this automation-triggered instance has finished
// and is waiting for the user to review its diff.
PendingReview bool
// CompletedAt is when the instance transitioned Running → Ready as an automation result.
CompletedAt *time.Time
```

**Step 4: Wire `ToInstanceData`**

In `ToInstanceData()`, add after `ParentTitle: i.ParentTitle,`:

```go
AutomationID:  i.AutomationID,
PendingReview: i.PendingReview,
CompletedAt:   i.CompletedAt,
```

**Step 5: Wire `FromInstanceData`**

In `FromInstanceData()`, add after `ParentTitle: data.ParentTitle,`:

```go
AutomationID:  data.AutomationID,
PendingReview: data.PendingReview,
CompletedAt:   data.CompletedAt,
```

**Step 6: Add `AutomationID` to `InstanceOptions` and `NewInstance`**

In `InstanceOptions` struct, add:

```go
AutomationID string
```

In `NewInstance()` return literal, add:

```go
AutomationID: opts.AutomationID,
```

**Step 7: Run test**

```bash
go test ./session/... -run TestInstance_ReviewFields_RoundTrip -v
```

Expected: PASS.

**Step 8: Build and vet**

```bash
go build ./... && go vet ./...
```

**Step 9: Commit**

```bash
git add session/instance.go session/instance_test.go
git commit -m "feat(review-queue): add review fields to Instance struct with round-trip serialization"
```

---

### Task 1.3: Set `PendingReview` when automation-triggered instance finishes

**Files:**
- Modify: `session/instance.go:269` (`SetStatus`)
- Test: `session/instance_test.go`

**Step 1: Write the failing test**

```go
func TestSetStatus_SetsReviewForAutomationInstance(t *testing.T) {
    inst := &Instance{
        Title:        "auto-agent",
        AutomationID: "auto-42",
        Status:       Running,
    }
    inst.SetStatus(Ready)

    if !inst.PendingReview {
        t.Error("PendingReview should be true for automation instance")
    }
    if inst.CompletedAt == nil {
        t.Error("CompletedAt should be set")
    }
}

func TestSetStatus_NoReviewForManualInstance(t *testing.T) {
    inst := &Instance{
        Title:  "manual-agent",
        Status: Running,
    }
    inst.SetStatus(Ready)

    if inst.PendingReview {
        t.Error("PendingReview should be false for manual instance")
    }
}
```

**Step 2: Run to verify failure**

```bash
go test ./session/... -run TestSetStatus_ -v
```

Expected: FAIL — `PendingReview` stays false for automation instance.

**Step 3: Update `SetStatus`**

In `session/instance.go`, update `SetStatus` so the `Running → Ready` block reads:

```go
if i.Status == Running && status == Ready {
    i.Notified = true
    SendNotification("Hivemind", fmt.Sprintf("'%s' has finished", i.Title))
    if i.AutomationID != "" {
        now := time.Now()
        i.PendingReview = true
        i.CompletedAt = &now
    }
}
```

**Step 4: Run tests**

```bash
go test ./session/... -run TestSetStatus_ -v
```

Expected: both PASS.

**Step 5: Build**

```bash
go build ./...
```

**Step 6: Commit**

```bash
git add session/instance.go session/instance_test.go
git commit -m "feat(review-queue): set PendingReview when automation instance finishes"
```

---

### Task 1.4: Wire `AutomationID` through brain `CreateInstance` path

**Files:**
- Modify: `brain/protocol.go` (`CreateInstanceParams`)
- Modify: `app/app_brain.go` (where `CreateInstanceParams` is consumed to create an instance)

**Step 1: Add `AutomationID` to `CreateInstanceParams`**

In `brain/protocol.go`, add to `CreateInstanceParams`:

```go
// AutomationID links this instance to the automation that spawned it.
// When set, the instance will enter the Review Queue on completion.
AutomationID string `json:"automation_id,omitempty"`
```

**Step 2: Find where `CreateInstanceParams` is used to call `NewInstance`**

```bash
grep -n "CreateInstanceParams\|\.Title.*params\|opts\.Title\|AutomationID" app/app_brain.go
```

**Step 3: Pass `AutomationID` into `InstanceOptions`**

In `app/app_brain.go`, in the `NewInstance` call, add:

```go
AutomationID: params.AutomationID,
```

**Step 4: Build and vet**

```bash
go build ./... && go vet ./...
```

**Step 5: Commit**

```bash
git add brain/protocol.go app/app_brain.go
git commit -m "feat(review-queue): thread AutomationID through brain CreateInstance path"
```

---

### Task 1.5: Render Review Queue section in sidebar

**Files:**
- Modify: `ui/list_renderer.go`
- Modify: `ui/list.go` (`View()` method)

**Step 1: Add review section header styles**

In `ui/list_renderer.go`, add new styles near the top of the file (after existing style vars):

```go
var (
    reviewSectionStyle = lipgloss.NewStyle().
        Foreground(lipgloss.Color("#F0A868")).
        Bold(true)
    reviewItemStyle = lipgloss.NewStyle().
        Foreground(lipgloss.Color("#22c55e"))
    reviewItemSelectedStyle = lipgloss.NewStyle().
        Background(lipgloss.Color("#22c55e")).
        Foreground(lipgloss.Color("#1a1a1a")).
        Bold(true)
)
```

**Step 2: Add `RenderReviewSection` function**

Add to `ui/list_renderer.go`:

```go
// RenderReviewSection renders the "REVIEW QUEUE" header + items for instances
// with PendingReview == true. Returns empty string if no review items.
func RenderReviewSection(instances []*session.Instance, selectedTitle string, width int) string {
    var pending []*session.Instance
    for _, inst := range instances {
        if inst.PendingReview {
            pending = append(pending, inst)
        }
    }
    if len(pending) == 0 {
        return ""
    }

    var b strings.Builder
    header := reviewSectionStyle.Render("─ REVIEW QUEUE ")
    b.WriteString(header + "\n")

    for _, inst := range pending {
        isSelected := inst.Title == selectedTitle

        // Build stats string
        stats := ""
        if ds := inst.GetDiffStats(); ds != nil && (ds.Added > 0 || ds.Removed > 0) {
            stats = fmt.Sprintf(" +%d/-%d", ds.Added, ds.Removed)
        }

        age := ""
        if inst.CompletedAt != nil {
            d := time.Since(*inst.CompletedAt)
            switch {
            case d < time.Minute:
                age = "just now"
            case d < time.Hour:
                age = fmt.Sprintf("%dm ago", int(d.Minutes()))
            default:
                age = fmt.Sprintf("%dh ago", int(d.Hours()))
            }
        }

        label := fmt.Sprintf("✓ %-20s%s  %s", inst.Title, stats, age)
        label = runewidth.Truncate(label, width-2, "…")

        if isSelected {
            b.WriteString(reviewItemSelectedStyle.Render(" "+label) + "\n")
        } else {
            b.WriteString(reviewItemStyle.Render(" "+label) + "\n")
        }
    }
    return b.String()
}
```

**Step 3: Call `RenderReviewSection` in the list `View()`**

In `ui/list.go`, find the `View()` (or `String()`) method. Prepend the review section output above the main instance list:

```go
reviewSection := RenderReviewSection(l.GetInstances(), selectedTitle, l.width)
// then prepend reviewSection to the rendered output
```

(Exact integration depends on how `View()` builds its string — prepend `reviewSection` before the running instances block.)

**Step 4: Build**

```bash
go build ./...
```

**Step 5: Commit**

```bash
git add ui/list_renderer.go ui/list.go
git commit -m "feat(review-queue): render Review Queue section in sidebar"
```

---

### Task 1.6: Add review action key handlers

**Files:**
- Modify: `app/app_input.go`
- Modify: `app/app_actions.go`
- Modify: `app/app.go` (add `stateReviewSendBack` to state enum)

**Step 1: Add new app state**

In `app/app.go`, add to the state `const` block:

```go
// stateReviewSendBack is when the user is typing feedback to send back to a review-queue agent.
stateReviewSendBack
```

**Step 2: Add action functions to `app_actions.go`**

```go
// discardReviewInstance removes the worktree of a PendingReview instance and
// clears its review state. The instance itself is not deleted from the list.
func (m *home) discardReviewInstance(instance *session.Instance) (tea.Model, tea.Cmd) {
    instance.PendingReview = false
    // Reuse the existing kill/pause flow to clean up the worktree.
    // The instance will remain in the list as Paused or can be killed normally.
    return m, nil
}

// clearReviewState marks the instance as no longer pending review.
func clearReviewState(instance *session.Instance) {
    instance.PendingReview = false
    instance.CompletedAt = nil
}
```

**Step 3: Add `handleReviewKeys` to `app_input.go`**

Wire up keys when the selected instance has `PendingReview == true` and the user is in `stateDefault`:

```go
// handleReviewActions handles c/p/s/o/d for a PendingReview instance.
// Call this from handleDefaultKeys when selected.PendingReview is true.
func (m *home) handleReviewActions(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
    selected := m.list.GetSelectedInstance()
    if selected == nil || !selected.PendingReview {
        return m, nil
    }

    switch msg.String() {
    case "s": // send back
        m.state = stateReviewSendBack
        m.textInputOverlay = overlay.NewTextInputOverlay("Send feedback to agent", "")
        return m, nil

    case "d": // discard
        clearReviewState(selected)
        m.toasts.Push(overlay.ToastInfo, fmt.Sprintf("Discarded review for '%s'", selected.Title))
        return m, nil

    case "c": // commit
        clearReviewState(selected)
        m.state = statePRTitle // reuse existing commit flow or open PR
        m.textInputOverlay = overlay.NewTextInputOverlay("Commit message", "")
        return m, nil

    case "p": // create PR — reuse existing PR flow
        clearReviewState(selected)
        m.state = statePRTitle
        m.textInputOverlay = overlay.NewTextInputOverlay("PR title", "")
        return m, nil
    }
    return m, nil
}
```

**Step 4: Add `handleReviewSendBackKeys`**

```go
func (m *home) handleReviewSendBackKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
    switch msg.String() {
    case "esc":
        m.state = stateDefault
        m.textInputOverlay = nil
        return m, nil
    case "enter":
        feedback := m.textInputOverlay.GetValue()
        m.textInputOverlay = nil
        m.state = stateDefault
        selected := m.list.GetSelectedInstance()
        if selected != nil && feedback != "" {
            _ = selected.SendPrompt(feedback)
            clearReviewState(selected)
        }
        return m, nil
    default:
        m.textInputOverlay.HandleKey(msg)
        return m, nil
    }
}
```

**Step 5: Route in `handleKeyPress`**

In the `switch m.state` dispatcher in `app_input.go`, add:

```go
case stateReviewSendBack:
    return m.handleReviewSendBackKeys(msg)
```

Also in `handleDefaultKeys`, before existing key handling, add:

```go
if selected := m.list.GetSelectedInstance(); selected != nil && selected.PendingReview {
    if model, cmd := m.handleReviewActions(msg); model != m {
        return model, cmd
    }
}
```

**Step 6: Show review hints in key hint bar**

In `ui/menu.go` or wherever key hints are rendered, add a `review` mode hint showing `c: commit  p: PR  s: send back  d: discard` when `selected.PendingReview`.

**Step 7: Build**

```bash
go build ./...
```

**Step 8: Commit**

```bash
git add app/app.go app/app_input.go app/app_actions.go
git commit -m "feat(review-queue): add review action keys (commit/PR/send-back/discard)"
```

---

## Milestone 3: Skills / Task Templates

Skills live in `~/.hivemind/skills/*.md` (global only). When creating a new instance, the user can press `Tab` to open a skill picker. The selected skill's instructions are prepended to the prompt, context files are appended, and an optional setup script runs before the agent starts.

---

### Task 3.1: Create `config/skills.go` with parsing

**Files:**
- Create: `config/skills.go`
- Create: `config/skills_test.go`

**Step 1: Add yaml.v3 dependency**

```bash
go get gopkg.in/yaml.v3
```

**Step 2: Write failing tests**

Create `config/skills_test.go`:

```go
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
```

**Step 3: Run to verify failure**

```bash
go test ./config/... -run TestLoadSkills -v
```

Expected: compile error — `config.LoadSkillsFrom` not found.

**Step 4: Create `config/skills.go`**

```go
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
    // Trim optional trailing newline from separator
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
```

**Step 5: Run tests**

```bash
go test ./config/... -run TestLoadSkills -v
```

Expected: all PASS.

**Step 6: Build and vet**

```bash
go build ./... && go vet ./...
```

**Step 7: Commit**

```bash
git add config/skills.go config/skills_test.go go.mod go.sum
git commit -m "feat(skills): add Skill type and LoadSkillsFrom parser with YAML frontmatter"
```

---

### Task 3.2: Add skill picker to instance creation flow

**Files:**
- Modify: `app/app.go` (add `stateSkillPicker` state, add `pendingSkill` field to `home`)
- Modify: `app/app_input.go` (`handleNewKeys` — open picker after name entry)

**Step 1: Add state and field**

In `app/app.go` state enum, add:

```go
// stateSkillPicker is when the user is choosing a skill for a new instance.
stateSkillPicker
```

In the `home` struct, add:

```go
pendingSkill *config.Skill // skill selected during instance creation
```

**Step 2: Load skills when entering `stateNew`**

In `app_input.go`, in the section where `stateNew` is set (when user presses `n`), load skills and store them:

```go
skills, _ := config.LoadSkills()
m.cachedSkills = skills // add cachedSkills []*config.Skill to home struct
```

**Step 3: After name entry, offer skill picker via `Tab`**

In `handleNewKeys` (or the equivalent in `app_input.go`), after the user submits the instance name but before creation, handle `Tab`:

```go
case "tab":
    if len(m.cachedSkills) == 0 {
        return m, nil // no skills, skip
    }
    items := make([]overlay.PickerItem, len(m.cachedSkills))
    for i, sk := range m.cachedSkills {
        items[i] = overlay.PickerItem{
            Title:       sk.Name,
            Description: sk.Description,
        }
    }
    m.pickerOverlay = overlay.NewPickerOverlay("Select skill (Tab to skip)", items)
    m.state = stateSkillPicker
    return m, nil
```

**Step 4: Handle skill picker submission**

Add `handleSkillPickerKeys` to `app_input.go`:

```go
func (m *home) handleSkillPickerKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
    done := m.pickerOverlay.HandleKeyPress(msg)
    if done {
        if m.pickerOverlay.IsSubmitted() {
            idx := m.pickerOverlay.SelectedIndex()
            if idx >= 0 && idx < len(m.cachedSkills) {
                m.pendingSkill = m.cachedSkills[idx]
            }
        }
        m.pickerOverlay = nil
        m.state = stateDefault
        // proceed to create instance with m.pendingSkill applied
        return m.createInstanceWithPendingSkill()
    }
    return m, nil
}
```

Route in `handleKeyPress`:

```go
case stateSkillPicker:
    return m.handleSkillPickerKeys(msg)
```

**Step 5: Build**

```bash
go build ./...
```

**Step 6: Commit**

```bash
git add app/app.go app/app_input.go
git commit -m "feat(skills): add skill picker step to instance creation flow"
```

---

### Task 3.3: Apply skill to instance (instructions, context files, setup script)

**Files:**
- Modify: `app/app_input.go` or `app/app_actions.go` (`createInstanceWithPendingSkill`)
- Modify: `session/instance_lifecycle.go` (run setup script before agent starts)

**Step 1: Implement `createInstanceWithPendingSkill`**

In `app/app_actions.go`, add:

```go
// createInstanceWithPendingSkill creates a new instance, prepending the pending
// skill's instructions and context to the prompt, then clears m.pendingSkill.
func (m *home) createInstanceWithPendingSkill() (tea.Model, tea.Cmd) {
    skill := m.pendingSkill
    m.pendingSkill = nil

    prompt := m.pendingPrompt // the prompt the user typed
    if skill != nil {
        var parts []string

        // 1. Skill instructions
        if skill.Instructions != "" {
            parts = append(parts, skill.Instructions)
        }

        // 2. Context files
        for _, f := range skill.ContextFiles {
            expanded := expandTilde(f)
            content, err := os.ReadFile(expanded)
            if err == nil {
                parts = append(parts, fmt.Sprintf("<context file=%q>\n%s\n</context>", f, string(content)))
            }
        }

        if len(parts) > 0 {
            prompt = strings.Join(parts, "\n\n") + "\n\n" + prompt
        }
    }

    // Continue with normal instance creation using the enriched prompt.
    // Pass skill.SetupScript via InstanceOptions so lifecycle can run it.
    opts := session.InstanceOptions{
        // ... existing fields populated from m.pendingName, path, etc. ...
        SetupScript: "",
    }
    if skill != nil {
        opts.SetupScript = skill.SetupScript
    }
    // ... create instance ...
    return m, nil
}

func expandTilde(path string) string {
    if strings.HasPrefix(path, "~/") {
        home, _ := os.UserHomeDir()
        return filepath.Join(home, path[2:])
    }
    return path
}
```

**Step 2: Add `SetupScript` to `InstanceOptions`**

In `session/instance.go`, add to `InstanceOptions`:

```go
SetupScript string
```

**Step 3: Run setup script in `instance_lifecycle.go`**

In `session/instance_lifecycle.go`, find `Start()`. After the worktree is set up but before the tmux session launches:

```go
if opts.SetupScript != "" {
    cmd := exec.Command("sh", "-c", opts.SetupScript)
    cmd.Dir = instance.Path
    out, err := cmd.CombinedOutput()
    if err != nil {
        log.WarningLog.Printf("setup script failed for %s: %v\n%s", instance.Title, err, out)
    }
    // Append output as context to the prompt
    if len(out) > 0 {
        instance.pendingPromptSuffix = fmt.Sprintf("\n\n<setup-output>\n%s\n</setup-output>", string(out))
    }
}
```

**Step 4: Build**

```bash
go build ./...
```

**Step 5: Commit**

```bash
git add app/app_actions.go session/instance.go session/instance_lifecycle.go
git commit -m "feat(skills): apply skill instructions, context files, and setup script to instance"
```

---

## Milestone 2: Inline Diff Comments

In the diff view, the user enters comment mode (`v`), moves a cursor through diff lines (`j`/`k`), adds comments (`c`), and sends all comments back to the agent (`Enter`).

---

### Task 2.1: Add `LineComment` type and comment state to `DiffPane`

**Files:**
- Modify: `ui/diff.go`
- Create: `ui/diff_comments_test.go`

**Step 1: Write failing tests**

Create `ui/diff_comments_test.go`:

```go
package ui

import (
    "strings"
    "testing"
)

func TestDiffPane_AddComment(t *testing.T) {
    d := NewDiffPane()
    d.AddComment("main.go", 5, "+", "newCode()", "this should use the existing helper")

    comments := d.GetComments()
    if len(comments["main.go"]) != 1 {
        t.Fatalf("expected 1 comment, got %d", len(comments["main.go"]))
    }
    c := comments["main.go"][0]
    if c.Line != 5 || c.Comment != "this should use the existing helper" {
        t.Errorf("unexpected comment: %+v", c)
    }
}

func TestDiffPane_FormatCommentsMessage(t *testing.T) {
    d := NewDiffPane()
    d.AddComment("auth/handler.go", 42, "+", `token := r.Header.Get("Auth")`, `use "Authorization"`)
    d.AddComment("utils/parse.go", 18, "-", `return nil`, "don't remove this nil check")

    msg := d.FormatCommentsMessage()
    if !strings.Contains(msg, "auth/handler.go") {
        t.Error("expected auth/handler.go in message")
    }
    if !strings.Contains(msg, `use "Authorization"`) {
        t.Error("expected comment text in message")
    }
    if !strings.Contains(msg, "Please address these") {
        t.Error("expected closing instruction in message")
    }
}

func TestDiffPane_ClearComments(t *testing.T) {
    d := NewDiffPane()
    d.AddComment("main.go", 1, "+", "code", "comment")
    d.ClearComments()

    if len(d.GetComments()) != 0 {
        t.Error("expected comments cleared")
    }
}
```

**Step 2: Run to verify failure**

```bash
go test ./ui/... -run TestDiffPane_ -v
```

Expected: compile error — `AddComment`, `GetComments`, etc. not defined.

**Step 3: Add `LineComment` and comment fields to `DiffPane`**

In `ui/diff.go`, add the type and update the struct:

```go
// LineComment is an annotation attached to a specific line in the diff.
type LineComment struct {
    File    string // relative file path
    Line    int    // 0-based index into the rendered diff content
    Marker  string // "+", "-", or " "
    Code    string // the line being commented on (trimmed)
    Comment string // user's comment text
}
```

In `DiffPane`, add after `selectedFile int`:

```go
// Comment mode
commentMode    bool
commentCursor  int                     // 0-based line index in current diff view
comments       map[string][]LineComment // filePath → comments
```

**Step 4: Add comment methods**

```go
func (d *DiffPane) AddComment(file string, line int, marker, code, comment string) {
    if d.comments == nil {
        d.comments = make(map[string][]LineComment)
    }
    d.comments[file] = append(d.comments[file], LineComment{
        File: file, Line: line, Marker: marker, Code: code, Comment: comment,
    })
}

func (d *DiffPane) GetComments() map[string][]LineComment {
    if d.comments == nil {
        return map[string][]LineComment{}
    }
    return d.comments
}

func (d *DiffPane) ClearComments() {
    d.comments = nil
}

func (d *DiffPane) HasComments() bool {
    for _, cs := range d.comments {
        if len(cs) > 0 {
            return true
        }
    }
    return false
}

// FormatCommentsMessage formats all comments as a structured prompt message
// suitable for injection into the agent's terminal.
func (d *DiffPane) FormatCommentsMessage() string {
    if !d.HasComments() {
        return ""
    }
    var b strings.Builder
    b.WriteString("Code review feedback on your changes:\n\n")
    for file, cs := range d.comments {
        for _, c := range cs {
            b.WriteString(fmt.Sprintf("[%s +%d] `%s`\n  → %s\n\n", file, c.Line, c.Code, c.Comment))
        }
    }
    b.WriteString("Please address these comments and continue.")
    return b.String()
}
```

**Step 5: Run tests**

```bash
go test ./ui/... -run TestDiffPane_ -v
```

Expected: all PASS.

**Step 6: Build and vet**

```bash
go build ./... && go vet ./...
```

**Step 7: Commit**

```bash
git add ui/diff.go ui/diff_comments_test.go
git commit -m "feat(inline-comments): add LineComment type and comment methods to DiffPane"
```

---

### Task 2.2: Add comment mode cursor and rendering to `DiffPane`

**Files:**
- Modify: `ui/diff.go`

**Step 1: Add cursor methods**

```go
func (d *DiffPane) EnterCommentMode() {
    d.commentMode = true
    d.commentCursor = 0
}

func (d *DiffPane) ExitCommentMode() {
    d.commentMode = false
}

func (d *DiffPane) IsCommentMode() bool {
    return d.commentMode
}

func (d *DiffPane) CommentCursorDown() {
    // Count visible lines in current diff content
    lines := strings.Split(d.viewport.View(), "\n")
    if d.commentCursor < len(lines)-1 {
        d.commentCursor++
    }
}

func (d *DiffPane) CommentCursorUp() {
    if d.commentCursor > 0 {
        d.commentCursor--
    }
}

// GetCursorLineInfo returns the file, marker, and code for the current cursor line.
// Returns empty strings if line is not a diff line (e.g. hunk header).
func (d *DiffPane) GetCursorLineInfo() (file, marker, code string, lineIdx int) {
    // Use the selected file path
    if d.selectedFile >= 0 && d.selectedFile < len(d.files) {
        file = d.files[d.selectedFile].path
    }
    diff := ""
    if d.selectedFile < 0 {
        diff = d.fullDiff
    } else if d.selectedFile < len(d.files) {
        diff = d.files[d.selectedFile].diff
    }
    lines := strings.Split(diff, "\n")
    if d.commentCursor < len(lines) {
        line := lines[d.commentCursor]
        if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
            return file, "+", strings.TrimPrefix(line, "+"), d.commentCursor
        }
        if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
            return file, "-", strings.TrimPrefix(line, "-"), d.commentCursor
        }
        return file, " ", line, d.commentCursor
    }
    return file, " ", "", d.commentCursor
}
```

**Step 2: Update `rebuildViewport` to render cursor and comment annotations**

In `rebuildViewport()`, when `d.commentMode` is true, post-process the colorized diff to:
- Highlight the cursor line with a `▶` indicator
- Inject comment annotations below commented lines using amber style:

```go
commentAnnotationStyle := lipgloss.NewStyle().
    Foreground(lipgloss.Color("#F0A868")).
    Italic(true)
```

Render each annotation as:
```
  │ ★ <comment text>
```

(Implementation detail: build a `map[int]string` of `lineIndex → comment` from `d.comments[currentFile]`, then annotate when rebuilding the viewport content.)

**Step 3: Build**

```bash
go build ./...
```

**Step 4: Commit**

```bash
git add ui/diff.go
git commit -m "feat(inline-comments): add comment mode cursor tracking and annotation rendering"
```

---

### Task 2.3: Add comment mode key handlers in `app/app_input.go`

**Files:**
- Modify: `app/app_input.go`
- Modify: `app/app.go` (add `stateInlineComment` state)
- Modify: `ui/tabbed_window.go` (add `GetDiffPane()` getter)

**Step 1: Add `GetDiffPane()` to `TabbedWindow`**

In `ui/tabbed_window.go`:

```go
// GetDiffPane returns the DiffPane for direct interaction (comment mode).
func (w *TabbedWindow) GetDiffPane() *DiffPane {
    return w.diff
}
```

**Step 2: Add app state**

In `app/app.go`:

```go
// stateInlineComment is when the user is typing a comment for a diff line.
stateInlineComment
```

**Step 3: Add comment key handler**

In `app/app_input.go`:

```go
func (m *home) handleDiffCommentKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
    diff := m.tabbedWindow.GetDiffPane()

    switch msg.String() {
    case "v": // toggle comment mode
        if diff.IsCommentMode() {
            diff.ExitCommentMode()
        } else {
            diff.EnterCommentMode()
        }
    case "j", "down":
        if diff.IsCommentMode() {
            diff.CommentCursorDown()
        } else {
            diff.ScrollDown()
        }
    case "k", "up":
        if diff.IsCommentMode() {
            diff.CommentCursorUp()
        } else {
            diff.ScrollUp()
        }
    case "c":
        if diff.IsCommentMode() {
            m.state = stateInlineComment
            m.textInputOverlay = overlay.NewTextInputOverlay("Add comment", "")
        }
    case "x":
        diff.ClearComments()
        m.toasts.Push(overlay.ToastInfo, "Comments cleared")
    case "enter":
        if diff.HasComments() {
            selected := m.list.GetSelectedInstance()
            if selected != nil {
                msg := diff.FormatCommentsMessage()
                _ = selected.SendPrompt(msg)
                diff.ClearComments()
                diff.ExitCommentMode()
                m.toasts.Push(overlay.ToastSuccess, "Feedback sent to agent")
            }
        }
    }
    return m, nil
}

func (m *home) handleInlineCommentInputKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
    switch msg.String() {
    case "esc":
        m.state = stateDefault
        m.textInputOverlay = nil
    case "enter":
        comment := m.textInputOverlay.GetValue()
        m.textInputOverlay = nil
        m.state = stateDefault
        if comment != "" {
            diff := m.tabbedWindow.GetDiffPane()
            file, marker, code, lineIdx := diff.GetCursorLineInfo()
            diff.AddComment(file, lineIdx, marker, code, comment)
        }
    default:
        m.textInputOverlay.HandleKey(msg)
    }
    return m, nil
}
```

**Step 4: Route in `handleKeyPress`**

Add cases:

```go
case stateInlineComment:
    return m.handleInlineCommentInputKeys(msg)
```

Also call `m.handleDiffCommentKeys(msg)` from the diff-focused key handling path.

**Step 5: Build**

```bash
go build ./...
```

**Step 6: Commit**

```bash
git add app/app.go app/app_input.go ui/tabbed_window.go
git commit -m "feat(inline-comments): add comment mode key handlers and send-feedback flow"
```

---

## Milestone 4: Automations / Scheduled Tasks

Automations run agents on a schedule (hourly, daily, every Nh, @HH:MM). The existing daemon tick loop triggers them. Results land in the Review Queue automatically via `AutomationID`.

---

### Task 4.1: Create `config/automations.go` with schedule parsing

**Files:**
- Create: `config/automations.go`
- Create: `config/automations_test.go`

**Step 1: Write failing tests**

Create `config/automations_test.go`:

```go
package config_test

import (
    "testing"
    "time"

    "github.com/ByteMirror/hivemind/config"
)

func TestParseSchedule(t *testing.T) {
    cases := []struct {
        input    string
        wantErr  bool
        validate func(t *testing.T, interval time.Duration, dailyAt *time.Time)
    }{
        {
            input: "hourly",
            validate: func(t *testing.T, interval time.Duration, dailyAt *time.Time) {
                if interval != time.Hour {
                    t.Errorf("hourly: got %v", interval)
                }
            },
        },
        {
            input: "daily",
            validate: func(t *testing.T, interval time.Duration, dailyAt *time.Time) {
                if interval != 24*time.Hour {
                    t.Errorf("daily: got %v", interval)
                }
            },
        },
        {
            input: "every 4h",
            validate: func(t *testing.T, interval time.Duration, dailyAt *time.Time) {
                if interval != 4*time.Hour {
                    t.Errorf("every 4h: got %v", interval)
                }
            },
        },
        {
            input: "every 30m",
            validate: func(t *testing.T, interval time.Duration, dailyAt *time.Time) {
                if interval != 30*time.Minute {
                    t.Errorf("every 30m: got %v", interval)
                }
            },
        },
        {
            input: "@06:00",
            validate: func(t *testing.T, interval time.Duration, dailyAt *time.Time) {
                if dailyAt == nil {
                    t.Fatal("expected dailyAt, got nil")
                }
                if dailyAt.Hour() != 6 || dailyAt.Minute() != 0 {
                    t.Errorf("@06:00: got %v:%v", dailyAt.Hour(), dailyAt.Minute())
                }
            },
        },
        {
            input:   "every 0h",
            wantErr: true,
        },
    }

    for _, tc := range cases {
        t.Run(tc.input, func(t *testing.T) {
            interval, dailyAt, err := config.ParseSchedule(tc.input)
            if tc.wantErr {
                if err == nil {
                    t.Error("expected error, got nil")
                }
                return
            }
            if err != nil {
                t.Fatalf("unexpected error: %v", err)
            }
            tc.validate(t, interval, dailyAt)
        })
    }
}

func TestNextRunTime_IntervalBased(t *testing.T) {
    now := time.Now()
    auto := &config.Automation{
        Schedule: "every 2h",
        LastRun:  &now,
    }
    next, err := config.NextRunTime(auto)
    if err != nil {
        t.Fatal(err)
    }
    expected := now.Add(2 * time.Hour)
    diff := next.Sub(expected)
    if diff < 0 {
        diff = -diff
    }
    if diff > time.Second {
        t.Errorf("NextRunTime: got %v, want ~%v", next, expected)
    }
}
```

**Step 2: Run to verify failure**

```bash
go test ./config/... -run TestParseSchedule -v
go test ./config/... -run TestNextRunTime -v
```

Expected: compile error.

**Step 3: Create `config/automations.go`**

```go
package config

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "regexp"
    "strconv"
    "strings"
    "time"

    "github.com/google/uuid"
)

// Automation defines a scheduled background task.
type Automation struct {
    ID           string     `json:"id"`
    Name         string     `json:"name"`
    Instructions string     `json:"instructions"`
    SkillName    string     `json:"skill_name,omitempty"`
    Schedule     string     `json:"schedule"` // "hourly", "daily", "every 4h", "@06:00"
    RepoPath     string     `json:"repo_path"`
    Enabled      bool       `json:"enabled"`
    LastRun      *time.Time `json:"last_run,omitempty"`
    NextRun      time.Time  `json:"next_run"`
    CreatedAt    time.Time  `json:"created_at"`
}

// ParseSchedule parses a human-friendly schedule string.
// Returns (interval, nil, nil) for interval-based schedules,
// or (0, &time.Time, nil) for @HH:MM daily schedules.
func ParseSchedule(s string) (time.Duration, *time.Time, error) {
    s = strings.TrimSpace(strings.ToLower(s))
    switch s {
    case "hourly":
        return time.Hour, nil, nil
    case "daily":
        return 24 * time.Hour, nil, nil
    case "weekly":
        return 7 * 24 * time.Hour, nil, nil
    }

    // every Nh or every Nm
    if strings.HasPrefix(s, "every ") {
        part := strings.TrimPrefix(s, "every ")
        re := regexp.MustCompile(`^(\d+)(h|m)$`)
        m := re.FindStringSubmatch(part)
        if m == nil {
            return 0, nil, fmt.Errorf("invalid schedule %q: expected 'every Nh' or 'every Nm'", s)
        }
        n, _ := strconv.Atoi(m[1])
        if n <= 0 {
            return 0, nil, fmt.Errorf("invalid schedule %q: N must be > 0", s)
        }
        unit := m[2]
        if unit == "h" {
            return time.Duration(n) * time.Hour, nil, nil
        }
        return time.Duration(n) * time.Minute, nil, nil
    }

    // @HH:MM
    if strings.HasPrefix(s, "@") {
        parts := strings.Split(strings.TrimPrefix(s, "@"), ":")
        if len(parts) != 2 {
            return 0, nil, fmt.Errorf("invalid schedule %q: expected '@HH:MM'", s)
        }
        h, err1 := strconv.Atoi(parts[0])
        min, err2 := strconv.Atoi(parts[1])
        if err1 != nil || err2 != nil || h < 0 || h > 23 || min < 0 || min > 59 {
            return 0, nil, fmt.Errorf("invalid schedule %q: invalid time", s)
        }
        t := time.Date(0, 1, 1, h, min, 0, 0, time.Local)
        return 0, &t, nil
    }

    return 0, nil, fmt.Errorf("unknown schedule format %q", s)
}

// NextRunTime computes the next run time for an automation.
func NextRunTime(a *Automation) (time.Time, error) {
    interval, dailyAt, err := ParseSchedule(a.Schedule)
    if err != nil {
        return time.Time{}, err
    }
    now := time.Now()

    if dailyAt != nil {
        // Daily at a specific time: find the next occurrence of HH:MM today or tomorrow
        candidate := time.Date(now.Year(), now.Month(), now.Day(),
            dailyAt.Hour(), dailyAt.Minute(), 0, 0, time.Local)
        if !candidate.After(now) {
            candidate = candidate.Add(24 * time.Hour)
        }
        return candidate, nil
    }

    // Interval-based: base off last run, or now if never run
    base := now
    if a.LastRun != nil {
        base = *a.LastRun
    }
    return base.Add(interval), nil
}

// automationsFileName returns the path to automations.json.
func automationsFileName() (string, error) {
    dir, err := GetConfigDir()
    if err != nil {
        return "", err
    }
    return filepath.Join(dir, "automations.json"), nil
}

// LoadAutomations reads automations from disk. Returns empty slice if file doesn't exist.
func LoadAutomations() ([]*Automation, error) {
    path, err := automationsFileName()
    if err != nil {
        return nil, err
    }
    data, err := os.ReadFile(path)
    if os.IsNotExist(err) {
        return nil, nil
    }
    if err != nil {
        return nil, fmt.Errorf("read automations: %w", err)
    }
    var automations []*Automation
    if err := json.Unmarshal(data, &automations); err != nil {
        return nil, fmt.Errorf("parse automations: %w", err)
    }
    return automations, nil
}

// SaveAutomations writes automations to disk atomically.
func SaveAutomations(automations []*Automation) error {
    path, err := automationsFileName()
    if err != nil {
        return err
    }
    data, err := json.MarshalIndent(automations, "", "  ")
    if err != nil {
        return fmt.Errorf("marshal automations: %w", err)
    }
    return atomicWriteFile(path, data, 0600)
}

// NewAutomation creates a new Automation with a generated ID and computed first NextRun.
func NewAutomation(name, instructions, skillName, schedule, repoPath string) (*Automation, error) {
    a := &Automation{
        ID:           uuid.New().String(),
        Name:         name,
        Instructions: instructions,
        SkillName:    skillName,
        Schedule:     schedule,
        RepoPath:     repoPath,
        Enabled:      true,
        CreatedAt:    time.Now(),
    }
    next, err := NextRunTime(a)
    if err != nil {
        return nil, err
    }
    a.NextRun = next
    return a, nil
}
```

Note: add `github.com/google/uuid` with `go get github.com/google/uuid` (or generate IDs with `fmt.Sprintf("%d", time.Now().UnixNano())` if you want to avoid the dependency — the UUID package is already common in Go projects).

**Step 4: Run tests**

```bash
go test ./config/... -run TestParseSchedule -v
go test ./config/... -run TestNextRunTime -v
```

Expected: all PASS.

**Step 5: Build and vet**

```bash
go build ./... && go vet ./...
```

**Step 6: Commit**

```bash
git add config/automations.go config/automations_test.go go.mod go.sum
git commit -m "feat(automations): add Automation type, ParseSchedule, NextRunTime, Load/Save"
```

---

### Task 4.2: Extend daemon to trigger due automations

**Files:**
- Modify: `daemon/daemon.go`

**Step 1: Add automation check to tick loop**

In `daemon/daemon.go`, after existing AutoYes instance loop inside the goroutine, add:

```go
// Check for due automations
automations, err := config.LoadAutomations()
if err == nil {
    for _, auto := range automations {
        if !auto.Enabled || time.Now().Before(auto.NextRun) {
            continue
        }
        if brainServer != nil {
            triggerAutomation(auto, brainServer)
        }
        now := time.Now()
        auto.LastRun = &now
        next, err := config.NextRunTime(auto)
        if err == nil {
            auto.NextRun = next
        }
    }
    _ = config.SaveAutomations(automations)
}
```

**Step 2: Implement `triggerAutomation`**

```go
func triggerAutomation(auto *config.Automation, srv *brain.Server) {
    title := fmt.Sprintf("%s-%s", auto.Name, time.Now().Format("0102-1504"))
    params := brain.CreateInstanceParams{
        Title:        title,
        Prompt:       auto.Instructions,
        Topic:        auto.Name,
        AutomationID: auto.ID,
    }
    skipTrue := true
    params.SkipPermissions = &skipTrue

    _, err := srv.CreateInstanceDirect(params)
    if err != nil {
        log.WarningLog.Printf("automation %q: failed to create instance: %v", auto.Name, err)
    }
}
```

**Step 3: Add `CreateInstanceDirect` to `brain.Server`**

The brain server already handles `CreateInstance` actions from agents via IPC. Add a direct Go method so the daemon (same process as brain server in daemon mode) can call it without the socket:

In `brain/server.go`, add:

```go
// CreateInstanceDirect enqueues a CreateInstance action from the daemon.
// The TUI's polling loop will pick it up just like an agent-triggered creation.
func (s *Server) CreateInstanceDirect(params CreateInstanceParams) (CreateInstanceResult, error) {
    return s.manager.HandleCreateInstance(params)
}
```

(Adjust to match how `HandleCreateInstance` is actually wired — look at the existing IPC handler in `brain/server.go` to mirror the same call path.)

**Step 4: Build**

```bash
go build ./...
```

**Step 5: Commit**

```bash
git add daemon/daemon.go brain/server.go
git commit -m "feat(automations): trigger due automations in daemon tick loop"
```

---

### Task 4.3: Add Automation Manager TUI screen

**Files:**
- Create: `ui/automations_list.go`
- Modify: `app/app.go` (add `stateAutomations`, `automations []*config.Automation` to `home`)
- Modify: `app/app_input.go` (`handleAutomationsKeys`)

**Step 1: Create `ui/automations_list.go`**

```go
package ui

import (
    "fmt"
    "strings"
    "time"

    "github.com/ByteMirror/hivemind/config"
    "github.com/charmbracelet/lipgloss"
)

var (
    autoHeaderStyle = lipgloss.NewStyle().Bold(true).
            Foreground(lipgloss.Color("#F0A868"))
    autoOnStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e"))
    autoOffStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
    autoSelStyle = lipgloss.NewStyle().
            Background(lipgloss.Color("#F0A868")).
            Foreground(lipgloss.Color("#1a1a1a")).Bold(true)
)

// RenderAutomationsList renders the full automations manager panel.
func RenderAutomationsList(automations []*config.Automation, selectedIdx, width int) string {
    var b strings.Builder

    header := autoHeaderStyle.Render("  AUTOMATIONS")
    b.WriteString(header + "\n\n")

    if len(automations) == 0 {
        b.WriteString("  No automations yet. Press n to create one.\n")
    }

    colName := 22
    colSched := 14
    colLast := 14
    colNext := 12

    // Header row
    b.WriteString(fmt.Sprintf("  %-*s %-*s %-*s %-*s %s\n",
        colName, "Name", colSched, "Schedule", colLast, "Last Run", colNext, "Next Run", "Status"))
    b.WriteString("  " + strings.Repeat("─", width-4) + "\n")

    for i, a := range automations {
        lastRun := "never"
        if a.LastRun != nil {
            lastRun = humanDuration(time.Since(*a.LastRun)) + " ago"
        }
        nextRun := humanDuration(time.Until(a.NextRun))
        if time.Until(a.NextRun) < 0 {
            nextRun = "overdue"
        }

        status := autoOnStyle.Render("● on")
        if !a.Enabled {
            status = autoOffStyle.Render("○ off")
        }

        row := fmt.Sprintf("  %-*s %-*s %-*s %-*s %s",
            colName, truncate(a.Name, colName),
            colSched, truncate(a.Schedule, colSched),
            colLast, truncate(lastRun, colLast),
            colNext, truncate(nextRun, colNext),
            status)

        if i == selectedIdx {
            b.WriteString(autoSelStyle.Render(row) + "\n")
        } else {
            b.WriteString(row + "\n")
        }
    }

    b.WriteString("\n")
    b.WriteString("  n: new  e: edit  space: toggle  r: run now  d: delete  Esc: back\n")
    return b.String()
}

func humanDuration(d time.Duration) string {
    if d < 0 {
        d = -d
    }
    switch {
    case d < time.Minute:
        return fmt.Sprintf("%ds", int(d.Seconds()))
    case d < time.Hour:
        return fmt.Sprintf("%dm", int(d.Minutes()))
    case d < 24*time.Hour:
        return fmt.Sprintf("%dh", int(d.Hours()))
    default:
        return fmt.Sprintf("%dd", int(d.Hours()/24))
    }
}

func truncate(s string, n int) string {
    if len(s) <= n {
        return s
    }
    return s[:n-1] + "…"
}
```

**Step 2: Add state and fields to `app/app.go`**

```go
stateAutomations        // viewing the automations manager
stateNewAutomation      // multi-step automation creation
stateEditAutomation     // editing an existing automation
```

In `home` struct:

```go
automations    []*config.Automation
autoSelectedIdx int
autoCreating   *config.Automation // in-progress automation during creation
autoCreateStep int                // 0=name, 1=instructions, 2=skill, 3=schedule, 4=repo
```

**Step 3: Add `handleAutomationsKeys` to `app_input.go`**

```go
func (m *home) handleAutomationsKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
    switch msg.String() {
    case "esc", "q":
        m.state = stateDefault
    case "j", "down":
        if m.autoSelectedIdx < len(m.automations)-1 {
            m.autoSelectedIdx++
        }
    case "k", "up":
        if m.autoSelectedIdx > 0 {
            m.autoSelectedIdx--
        }
    case "n":
        m.autoCreating = &config.Automation{}
        m.autoCreateStep = 0
        m.state = stateNewAutomation
        m.textInputOverlay = overlay.NewTextInputOverlay("Automation name", "")
    case " ": // toggle
        if m.autoSelectedIdx < len(m.automations) {
            m.automations[m.autoSelectedIdx].Enabled = !m.automations[m.autoSelectedIdx].Enabled
            _ = config.SaveAutomations(m.automations)
        }
    case "r": // run now
        if m.autoSelectedIdx < len(m.automations) {
            auto := m.automations[m.autoSelectedIdx]
            // reset NextRun to now so daemon triggers it on next tick
            auto.NextRun = time.Now()
            _ = config.SaveAutomations(m.automations)
            m.toasts.Push(overlay.ToastInfo, fmt.Sprintf("'%s' will run on next daemon tick", auto.Name))
        }
    case "d": // delete with confirm
        if m.autoSelectedIdx < len(m.automations) {
            name := m.automations[m.autoSelectedIdx].Name
            m.state = stateConfirm
            m.confirmOverlay = overlay.NewConfirmationOverlay(
                fmt.Sprintf("Delete automation '%s'?", name), "delete", "cancel")
            // store pending delete index for confirm handler
        }
    }
    return m, nil
}
```

**Step 4: Add automation creation step handler**

```go
func (m *home) handleNewAutomationKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
    switch msg.String() {
    case "esc":
        m.state = stateAutomations
        m.textInputOverlay = nil
        m.autoCreating = nil
    case "enter":
        val := m.textInputOverlay.GetValue()
        m.textInputOverlay = nil
        switch m.autoCreateStep {
        case 0: // name
            m.autoCreating.Name = val
            m.autoCreateStep = 1
            m.textInputOverlay = overlay.NewTextInputOverlay("Instructions (what should the agent do?)", "")
        case 1: // instructions
            m.autoCreating.Instructions = val
            m.autoCreateStep = 2
            m.textInputOverlay = overlay.NewTextInputOverlay("Schedule (hourly, daily, every 4h, @06:00)", "")
        case 2: // schedule (skipping skill for now — Tab flow same as instances)
            m.autoCreating.Schedule = val
            m.autoCreateStep = 3
            // Use current repo path
            m.autoCreating.RepoPath = m.getCurrentRepoPath()
            // Create the automation
            auto, err := config.NewAutomation(
                m.autoCreating.Name, m.autoCreating.Instructions,
                "", m.autoCreating.Schedule, m.autoCreating.RepoPath)
            if err != nil {
                m.toasts.Push(overlay.ToastError, fmt.Sprintf("Invalid schedule: %v", err))
                m.autoCreateStep = 2
                m.textInputOverlay = overlay.NewTextInputOverlay("Schedule (hourly, daily, every 4h, @06:00)", "")
                return m, nil
            }
            m.automations = append(m.automations, auto)
            _ = config.SaveAutomations(m.automations)
            m.toasts.Push(overlay.ToastSuccess, fmt.Sprintf("Automation '%s' created", auto.Name))
            m.state = stateAutomations
            m.autoCreating = nil
        }
    default:
        if m.textInputOverlay != nil {
            m.textInputOverlay.HandleKey(msg)
        }
    }
    return m, nil
}
```

**Step 5: Load automations on startup**

In `app/app.go`, in the `Init()` or startup code:

```go
if autos, err := config.LoadAutomations(); err == nil {
    m.automations = autos
}
```

**Step 6: Wire `View()` to render automations screen**

In the `View()` method of `home`, add a case:

```go
case m.state == stateAutomations || m.state == stateNewAutomation:
    mainView = ui.RenderAutomationsList(m.automations, m.autoSelectedIdx, m.width)
    if m.textInputOverlay != nil {
        mainView = overlay.PlaceOverlay(0, 0, m.textInputOverlay.Render(), mainView, true, true)
    }
```

**Step 7: Add `A` key to open automations from default state**

In `handleDefaultKeys`:

```go
case "A":
    m.state = stateAutomations
    return m, nil
```

**Step 8: Build**

```bash
go build ./...
```

**Step 9: Run all tests**

```bash
go test ./...
```

**Step 10: Commit**

```bash
git add ui/automations_list.go app/app.go app/app_input.go
git commit -m "feat(automations): add Automation Manager TUI screen with create/toggle/delete/run-now"
```

---

### Task 4.4: Final integration test and cleanup

**Step 1: Build and run all tests**

```bash
go build ./... && go vet ./... && go test ./...
```

Expected: all pass, no vet warnings.

**Step 2: Smoke test (manual)**

Run `hivemind` in a git repo:
1. Press `A` — automation manager opens, empty list, hint shows.
2. Press `n`, enter name `test-audit`, instructions `run go vet ./...`, schedule `every 1h`.
3. Automation appears in list with `● on` and correct next run time.
4. Press `space` — toggles to `○ off`.
5. Press `Esc` — returns to main view.
6. In diff view with a running instance: press `v` — comment mode cursor appears.
7. Press `c` — text overlay opens. Enter a comment. It appears inline in amber.
8. Press `Enter` — feedback formatted and sent to agent.

**Step 3: Commit anything final**

```bash
git add -A
git commit -m "feat: complete Codex-inspired features (review queue, inline comments, skills, automations)"
```

---

## Summary of new files

| File | Purpose |
|------|---------|
| `config/skills.go` | `Skill` type, `LoadSkills`, `LoadSkillsFrom`, frontmatter parser |
| `config/skills_test.go` | Unit tests for skill loading |
| `config/automations.go` | `Automation` type, `ParseSchedule`, `NextRunTime`, Load/Save |
| `config/automations_test.go` | Unit tests for schedule parsing |
| `ui/automations_list.go` | Automation manager TUI renderer |
| `ui/diff_comments_test.go` | Unit tests for inline diff comments |

## Summary of modified files

| File | Change |
|------|--------|
| `session/storage.go` | `AutomationID`, `PendingReview`, `CompletedAt` in `InstanceData` |
| `session/instance.go` | Same fields on `Instance`; `SetStatus` sets review on finish; `SetupScript` in `InstanceOptions` |
| `session/instance_lifecycle.go` | Run `SetupScript` before agent starts |
| `brain/protocol.go` | `AutomationID` in `CreateInstanceParams` |
| `brain/server.go` | `CreateInstanceDirect` for daemon use |
| `app/app_brain.go` | Thread `AutomationID` to `InstanceOptions` |
| `app/app.go` | New states + `home` fields for all features |
| `app/app_input.go` | Key handlers for review queue, inline comments, automations |
| `app/app_actions.go` | `discardReviewInstance`, `clearReviewState`, `createInstanceWithPendingSkill` |
| `ui/list_renderer.go` | `RenderReviewSection` |
| `ui/list.go` | Prepend review section in `View()` |
| `ui/diff.go` | `LineComment`, comment mode, cursor, annotation rendering, `FormatCommentsMessage` |
| `ui/tabbed_window.go` | `GetDiffPane()` getter |
| `daemon/daemon.go` | Automation schedule check in tick loop |
