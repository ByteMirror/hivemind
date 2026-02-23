# Memory UI Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add memory settings to the existing settings overlay and a full-screen split-pane memory file browser with edit/delete support.

**Architecture:** Memory settings are appended as a new section in `openSettings()` / `applySettingChange()` in `app/commands.go` — no new overlay needed. The memory browser is a new `ui/memory_browser.go` component rendered as a full-screen replacement (like the automations screen) via a new `stateMemoryBrowser` state. The browser calls `session.GetMemoryManager()` (a new exported accessor) to read, write, and delete files.

**Tech Stack:** Go, Bubble Tea (`github.com/charmbracelet/bubbletea`), Bubble Tea Bubbles (`github.com/charmbracelet/bubbles/textarea` — already used in `ui/overlay/textInput.go`), Lipgloss, `memory.Manager`, `config.MemoryConfig`.

---

### Task 1: Export `GetMemoryManager` from `session/memory_state.go`

The memory browser needs to call the memory manager from `app/` and `ui/`. Currently `getMemoryManager()` is unexported.

**Files:**
- Modify: `session/memory_state.go`

**Step 1: Add the exported accessor**

In `session/memory_state.go`, after `getMemoryInjectCount()`, add:

```go
// GetMemoryManager returns the IDE-wide memory manager, or nil if disabled.
// Safe for concurrent use.
func GetMemoryManager() *memory.Manager {
	memMu.RLock()
	defer memMu.RUnlock()
	return globalMemMgr
}
```

**Step 2: Build**

```bash
go build ./...
```

Expected: no errors.

**Step 3: Commit**

```bash
git add session/memory_state.go
git commit -m "feat: export GetMemoryManager from session package"
```

---

### Task 2: Add `KeyMemoryBrowser` key binding

**Files:**
- Modify: `keys/keys.go`

**Step 1: Add the constant**

In the `const` block in `keys/keys.go`, after `KeyCommandPalette`, add:

```go
KeyMemoryBrowser // Key for opening the memory file browser
```

**Step 2: Add to `GlobalKeyStringsMap`**

In `GlobalKeyStringsMap`, add the entry (use `"M"` — Shift+M — which is currently unbound):

```go
"M": KeyMemoryBrowser,
```

**Step 3: Add to `GlobalkeyBindings`**

At the end of `GlobalkeyBindings`, before the closing brace, add:

```go
KeyMemoryBrowser: key.NewBinding(
    key.WithKeys("M"),
    key.WithHelp("M", "memory"),
),
```

**Step 4: Build**

```bash
go build ./...
```

Expected: no errors.

**Step 5: Commit**

```bash
git add keys/keys.go
git commit -m "feat: add KeyMemoryBrowser key binding (M)"
```

---

### Task 3: Memory settings in `openSettings` and `applySettingChange`

**Files:**
- Modify: `app/commands.go`

The settings overlay already supports `SettingToggle`, `SettingPicker`, and `SettingText`. We add a memory section after the existing items. Provider-specific fields are conditionally included based on the current value.

**Step 1: Add a helper that builds the memory items**

In `app/commands.go`, before `openSettings()`, add this helper (it will be called both when opening and when the provider picker changes):

```go
// buildMemorySettingItems returns settings items for the Memory section.
// Provider-specific fields are only included when relevant.
func buildMemorySettingItems(cfg *config.Config) []overlay.SettingItem {
	mem := cfg.Memory
	enabled := "false"
	provider := "none"
	claudeModel := memory.DefaultRerankModel
	openAIKey := ""
	openAIModel := "text-embedding-3-small"
	ollamaURL := "http://localhost:11434"
	ollamaModel := "nomic-embed-text"
	injectCount := "5"

	if mem != nil {
		if mem.Enabled {
			enabled = "true"
		}
		if mem.EmbeddingProvider != "" {
			provider = mem.EmbeddingProvider
		}
		if mem.ClaudeModel != "" {
			claudeModel = mem.ClaudeModel
		}
		if mem.OpenAIAPIKey != "" {
			openAIKey = mem.OpenAIAPIKey
		}
		if mem.OpenAIModel != "" {
			openAIModel = mem.OpenAIModel
		}
		if mem.OllamaURL != "" {
			ollamaURL = mem.OllamaURL
		}
		if mem.OllamaModel != "" {
			ollamaModel = mem.OllamaModel
		}
		if mem.StartupInjectCount > 0 {
			injectCount = strconv.Itoa(mem.StartupInjectCount)
		}
	}

	items := []overlay.SettingItem{
		{
			Label:       "── Memory ──",
			Description: "",
			Type:        overlay.SettingText,
			Value:       "",
			Key:         "memory_section_header",
		},
		{
			Label:       "Memory enabled",
			Description: "Persistent knowledge base shared across all agents",
			Type:        overlay.SettingToggle,
			Value:       enabled,
			Key:         "memory.enabled",
		},
		{
			Label:       "Search provider",
			Description: "claude = re-rank via local claude CLI (works with Max), openai/ollama = embeddings",
			Type:        overlay.SettingPicker,
			Value:       provider,
			Options:     []string{"none", "claude", "openai", "ollama"},
			Key:         "memory.embedding_provider",
		},
	}

	switch provider {
	case "claude":
		items = append(items, overlay.SettingItem{
			Label:       "Claude model",
			Description: "Model for re-ranking (default: claude-haiku-4-5-20251001)",
			Type:        overlay.SettingText,
			Value:       claudeModel,
			Key:         "memory.claude_model",
		})
	case "openai":
		items = append(items, overlay.SettingItem{
			Label:       "OpenAI API key",
			Description: "sk-... key from platform.openai.com",
			Type:        overlay.SettingText,
			Value:       openAIKey,
			Key:         "memory.openai_api_key",
		}, overlay.SettingItem{
			Label:       "OpenAI model",
			Description: "Embedding model (default: text-embedding-3-small)",
			Type:        overlay.SettingText,
			Value:       openAIModel,
			Key:         "memory.openai_model",
		})
	case "ollama":
		items = append(items, overlay.SettingItem{
			Label:       "Ollama URL",
			Description: "Ollama server URL (default: http://localhost:11434)",
			Type:        overlay.SettingText,
			Value:       ollamaURL,
			Key:         "memory.ollama_url",
		}, overlay.SettingItem{
			Label:       "Ollama model",
			Description: "Embedding model (default: nomic-embed-text)",
			Type:        overlay.SettingText,
			Value:       ollamaModel,
			Key:         "memory.ollama_model",
		})
	}

	items = append(items, overlay.SettingItem{
		Label:       "Startup snippets",
		Description: "Memory snippets injected into CLAUDE.md at agent start (default: 5)",
		Type:        overlay.SettingText,
		Value:       injectCount,
		Key:         "memory.startup_inject_count",
	})

	return items
}
```

Note: `memory.DefaultRerankModel` needs to be exported from `memory/reranker.go`. In that file, change:
```go
const defaultRerankModel = "claude-haiku-4-5-20251001"
```
to:
```go
// DefaultRerankModel is the default Claude model used for re-ranking.
const DefaultRerankModel = "claude-haiku-4-5-20251001"
```
and update the reference inside `NewClaudeReranker` from `defaultRerankModel` to `DefaultRerankModel`.

**Step 2: Append memory items in `openSettings()`**

At the end of `openSettings()`, replace:
```go
m.settingsOverlay = overlay.NewSettingsOverlay(items)
m.state = stateSettings
return m, nil
```
with:
```go
items = append(items, buildMemorySettingItems(m.appConfig)...)
m.settingsOverlay = overlay.NewSettingsOverlay(items)
m.state = stateSettings
return m, nil
```

**Step 3: Handle memory keys in `applySettingChange()`**

In the `switch key` block inside `applySettingChange()`, after the `"branch_prefix"` case, add:

```go
case "memory.enabled":
    m.ensureMemoryConfig()
    m.appConfig.Memory.Enabled = item.Value == "true"
    m.restartMemoryManager()
case "memory.embedding_provider":
    m.ensureMemoryConfig()
    m.appConfig.Memory.EmbeddingProvider = item.Value
    // Rebuild settings overlay to show/hide provider-specific fields.
    newItems := m.settingsOverlay.Items()[:len(m.settingsOverlay.Items())-len(buildMemorySettingItems(m.appConfig))]
    newItems = append(newItems, buildMemorySettingItems(m.appConfig)...)
    m.settingsOverlay.SetItems(newItems)
    m.restartMemoryManager()
case "memory.claude_model":
    m.ensureMemoryConfig()
    m.appConfig.Memory.ClaudeModel = item.Value
case "memory.openai_api_key":
    m.ensureMemoryConfig()
    m.appConfig.Memory.OpenAIAPIKey = item.Value
    m.restartMemoryManager()
case "memory.openai_model":
    m.ensureMemoryConfig()
    m.appConfig.Memory.OpenAIModel = item.Value
case "memory.ollama_url":
    m.ensureMemoryConfig()
    m.appConfig.Memory.OllamaURL = item.Value
case "memory.ollama_model":
    m.ensureMemoryConfig()
    m.appConfig.Memory.OllamaModel = item.Value
case "memory.startup_inject_count":
    m.ensureMemoryConfig()
    if n, err := strconv.Atoi(item.Value); err == nil && n > 0 {
        m.appConfig.Memory.StartupInjectCount = n
    }
```

**Step 4: Add `ensureMemoryConfig` and `restartMemoryManager` helpers**

After `applySettingChange`, add:

```go
// ensureMemoryConfig initialises Memory config if nil.
func (m *home) ensureMemoryConfig() {
    if m.appConfig.Memory == nil {
        m.appConfig.Memory = &config.MemoryConfig{}
    }
}

// restartMemoryManager re-initialises the memory manager from the current config.
// Called whenever memory.enabled or the provider changes.
func (m *home) restartMemoryManager() {
    mgr, err := memory.NewManagerFromConfig(m.appConfig)
    if err != nil {
        if log.WarningLog != nil {
            log.WarningLog.Printf("memory restart: %v", err)
        }
        return
    }
    injectCount := 5
    if m.appConfig.Memory != nil && m.appConfig.Memory.StartupInjectCount > 0 {
        injectCount = m.appConfig.Memory.StartupInjectCount
    }
    session.SetMemoryManager(mgr, injectCount)
}
```

**Step 5: Add `Items()` and `SetItems()` to `SettingsOverlay`**

In `ui/overlay/settingsOverlay.go`, after `GetItem`:

```go
// Items returns a copy of the current settings items slice.
func (s *SettingsOverlay) Items() []SettingItem {
    cp := make([]SettingItem, len(s.items))
    copy(cp, s.items)
    return cp
}

// SetItems replaces the settings items and clamps the selection index.
func (s *SettingsOverlay) SetItems(items []SettingItem) {
    s.items = items
    if s.selectedIdx >= len(s.items) {
        s.selectedIdx = max(0, len(s.items)-1)
    }
    s.clampViewOffset()
}
```

Add `max` helper at the bottom of the file if not present:
```go
func max(a, b int) int {
    if a > b {
        return a
    }
    return b
}
```

**Step 6: Add missing imports to `app/commands.go`**

Ensure these are in the imports:
```go
import (
    "strconv"
    // existing imports...
    "github.com/ByteMirror/hivemind/log"
    "github.com/ByteMirror/hivemind/memory"
    "github.com/ByteMirror/hivemind/session"
)
```

**Step 7: Build**

```bash
go build ./...
```

Expected: no errors.

**Step 8: Manual smoke test**

Open Hivemind → press `ctrl+p` → type "Settings" → Enter. Scroll to the Memory section. Verify:
- Toggle for "Memory enabled" is visible
- Picker for "Search provider" cycles through none/claude/openai/ollama
- When "claude" is selected, a "Claude model" text field appears
- When "openai" is selected, API key and model fields appear
- When "none" is selected, no extra fields appear

**Step 9: Commit**

```bash
git add app/commands.go ui/overlay/settingsOverlay.go memory/reranker.go
git commit -m "feat: add memory settings section to settings overlay"
```

---

### Task 4: Create `ui/memory_browser.go`

The memory browser is a standalone component. It holds a file list on the left and a content/edit pane on the right. It does NOT embed app state — it receives a `*memory.Manager` and calls it directly.

**Files:**
- Create: `ui/memory_browser.go`
- Create: `ui/memory_browser_test.go`

**Step 1: Write the test first**

Create `ui/memory_browser_test.go`:

```go
package ui

import (
    "os"
    "path/filepath"
    "testing"

    "github.com/ByteMirror/hivemind/memory"
)

func TestMemoryBrowser_Navigation(t *testing.T) {
    dir := t.TempDir()
    // Write two memory files
    os.WriteFile(filepath.Join(dir, "global.md"), []byte("# Global\nSetup info here."), 0600)
    os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# Notes\nSome notes."), 0600)

    mgr, err := memory.NewManager(dir, nil)
    if err != nil {
        t.Fatal(err)
    }
    defer mgr.Close()

    // Sync files into index
    _ = mgr.Sync("global.md")
    _ = mgr.Sync("notes.md")

    b, err := NewMemoryBrowser(mgr)
    if err != nil {
        t.Fatal(err)
    }

    // Should start with first file selected
    if b.SelectedFile() == "" {
        t.Fatal("expected a selected file")
    }

    initial := b.SelectedFile()

    // Move down and verify selection changes
    b.SelectNext()
    if b.SelectedFile() == initial {
        t.Error("expected selection to change after SelectNext")
    }

    // Move back up
    b.SelectPrev()
    if b.SelectedFile() != initial {
        t.Errorf("expected selection to return to %s, got %s", initial, b.SelectedFile())
    }
}

func TestMemoryBrowser_LoadContent(t *testing.T) {
    dir := t.TempDir()
    content := "# Global\n\nThis is the global memory file."
    os.WriteFile(filepath.Join(dir, "global.md"), []byte(content), 0600)

    mgr, err := memory.NewManager(dir, nil)
    if err != nil {
        t.Fatal(err)
    }
    defer mgr.Close()
    _ = mgr.Sync("global.md")

    b, err := NewMemoryBrowser(mgr)
    if err != nil {
        t.Fatal(err)
    }

    got := b.Content()
    if got != content {
        t.Errorf("expected content %q, got %q", content, got)
    }
}

func TestMemoryBrowser_EditAndSave(t *testing.T) {
    dir := t.TempDir()
    os.WriteFile(filepath.Join(dir, "global.md"), []byte("original"), 0600)

    mgr, err := memory.NewManager(dir, nil)
    if err != nil {
        t.Fatal(err)
    }
    defer mgr.Close()
    _ = mgr.Sync("global.md")

    b, err := NewMemoryBrowser(mgr)
    if err != nil {
        t.Fatal(err)
    }

    b.EnterEditMode()
    if !b.IsEditing() {
        t.Fatal("expected editing mode")
    }

    b.SetEditContent("updated content")
    if err := b.SaveEdit(); err != nil {
        t.Fatalf("save failed: %v", err)
    }

    // Re-load to verify
    saved, _ := os.ReadFile(filepath.Join(dir, "global.md"))
    if string(saved) != "updated content" {
        t.Errorf("expected 'updated content', got %q", string(saved))
    }
}

func TestMemoryBrowser_Delete(t *testing.T) {
    dir := t.TempDir()
    os.WriteFile(filepath.Join(dir, "temp.md"), []byte("to be deleted"), 0600)

    mgr, err := memory.NewManager(dir, nil)
    if err != nil {
        t.Fatal(err)
    }
    defer mgr.Close()
    _ = mgr.Sync("temp.md")

    b, err := NewMemoryBrowser(mgr)
    if err != nil {
        t.Fatal(err)
    }

    if err := b.DeleteSelected(); err != nil {
        t.Fatalf("delete failed: %v", err)
    }

    if _, err := os.Stat(filepath.Join(dir, "temp.md")); !os.IsNotExist(err) {
        t.Error("expected file to be deleted")
    }
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./ui/... -run "TestMemoryBrowser" -v
```

Expected: `FAIL` — `NewMemoryBrowser` not defined yet.

**Step 3: Create `ui/memory_browser.go`**

```go
package ui

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "time"

    "github.com/charmbracelet/bubbles/textarea"
    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"

    "github.com/ByteMirror/hivemind/memory"
)

// focusPane identifies which half of the split-pane has focus.
type focusPane int

const (
    focusList focusPane = iota
    focusContent
)

// MemoryBrowser is a full-screen split-pane memory file viewer and editor.
type MemoryBrowser struct {
    mgr         *memory.Manager
    files       []memory.FileInfo
    selectedIdx int
    content     string // raw file content for the selected file
    originalContent string // content before edit started

    editing  bool
    textarea textarea.Model
    focus    focusPane

    confirmDelete bool // true when waiting for delete confirmation
    width, height int
}

// NewMemoryBrowser creates a MemoryBrowser backed by the given manager.
// It immediately loads the file list and auto-selects the first file.
func NewMemoryBrowser(mgr *memory.Manager) (*MemoryBrowser, error) {
    if mgr == nil {
        return nil, fmt.Errorf("memory manager is nil")
    }
    files, err := mgr.List()
    if err != nil {
        return nil, fmt.Errorf("list memory files: %w", err)
    }

    ta := textarea.New()
    ta.ShowLineNumbers = false
    ta.Prompt = ""
    ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
    ta.CharLimit = 0
    ta.MaxHeight = 0

    b := &MemoryBrowser{
        mgr:      mgr,
        files:    files,
        textarea: ta,
        focus:    focusList,
    }

    if len(files) > 0 {
        b.loadSelected()
    }
    return b, nil
}

// SelectedFile returns the relative path of the currently selected file.
func (b *MemoryBrowser) SelectedFile() string {
    if len(b.files) == 0 || b.selectedIdx >= len(b.files) {
        return ""
    }
    return b.files[b.selectedIdx].Path
}

// Content returns the raw content of the currently loaded file.
func (b *MemoryBrowser) Content() string { return b.content }

// IsEditing returns true when the right pane is in edit mode.
func (b *MemoryBrowser) IsEditing() bool { return b.editing }

// EnterEditMode switches the right pane into an editable textarea.
func (b *MemoryBrowser) EnterEditMode() {
    b.editing = true
    b.originalContent = b.content
    b.textarea.SetValue(b.content)
    b.textarea.Focus()
    b.focus = focusContent
}

// CancelEdit discards changes and returns to browse mode.
func (b *MemoryBrowser) CancelEdit() {
    b.editing = false
    b.content = b.originalContent
    b.textarea.Blur()
    b.focus = focusList
}

// SetEditContent sets the textarea value (used in tests).
func (b *MemoryBrowser) SetEditContent(s string) {
    b.textarea.SetValue(s)
}

// SaveEdit writes the textarea content to disk and re-indexes the file.
func (b *MemoryBrowser) SaveEdit() error {
    if !b.editing {
        return nil
    }
    path := b.SelectedFile()
    if path == "" {
        return fmt.Errorf("no file selected")
    }
    newContent := b.textarea.Value()
    absPath := filepath.Join(b.mgr.Dir(), path)
    if err := os.WriteFile(absPath, []byte(newContent), 0600); err != nil {
        return fmt.Errorf("write %s: %w", path, err)
    }
    _ = b.mgr.Sync(path)
    b.content = newContent
    b.editing = false
    b.textarea.Blur()
    b.focus = focusList
    // Refresh file list so size/mtime are current.
    b.refreshFileList()
    return nil
}

// DeleteSelected deletes the selected file from disk and removes it from the list.
func (b *MemoryBrowser) DeleteSelected() error {
    path := b.SelectedFile()
    if path == "" {
        return nil
    }
    absPath := filepath.Join(b.mgr.Dir(), path)
    if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
        return fmt.Errorf("delete %s: %w", path, err)
    }
    _ = b.mgr.Sync(path) // removes from index
    b.refreshFileList()
    if b.selectedIdx >= len(b.files) {
        b.selectedIdx = max(0, len(b.files)-1)
    }
    b.loadSelected()
    return nil
}

// SelectNext moves selection down one file.
func (b *MemoryBrowser) SelectNext() {
    if b.selectedIdx < len(b.files)-1 {
        b.selectedIdx++
        b.loadSelected()
    }
}

// SelectPrev moves selection up one file.
func (b *MemoryBrowser) SelectPrev() {
    if b.selectedIdx > 0 {
        b.selectedIdx--
        b.loadSelected()
    }
}

// SetSize updates the component dimensions.
func (b *MemoryBrowser) SetSize(width, height int) {
    b.width = width
    b.height = height
    _, rightW := b.paneSizes()
    b.textarea.SetWidth(rightW - 4)
    b.textarea.SetHeight(height - 6)
}

// HandleKeyPress processes one key event.
// Returns (cmd, close): close=true means the caller should exit this screen.
func (b *MemoryBrowser) HandleKeyPress(msg tea.KeyMsg) (tea.Cmd, bool) {
    if b.editing {
        switch msg.String() {
        case "ctrl+s":
            _ = b.SaveEdit()
            return nil, false
        case "esc":
            b.CancelEdit()
            return nil, false
        default:
            var taCmd tea.Cmd
            b.textarea, taCmd = b.textarea.Update(msg)
            return taCmd, false
        }
    }

    switch msg.String() {
    case "esc":
        if b.focus == focusContent {
            b.focus = focusList
            return nil, false
        }
        return nil, true // close browser
    case "tab":
        if b.focus == focusList {
            b.focus = focusContent
        } else {
            b.focus = focusList
        }
    case "up", "k":
        if b.focus == focusList {
            b.SelectPrev()
        }
    case "down", "j":
        if b.focus == focusList {
            b.SelectNext()
        }
    case "enter":
        if b.focus == focusList {
            b.loadSelected()
        }
    case "e":
        if b.SelectedFile() != "" {
            b.EnterEditMode()
        }
    case "d":
        if b.SelectedFile() != "" {
            b.confirmDelete = true
        }
    case "y":
        if b.confirmDelete {
            _ = b.DeleteSelected()
            b.confirmDelete = false
        }
    case "n":
        b.confirmDelete = false
    }
    return nil, false
}

// Render returns the full lipgloss-styled string for the browser.
func (b *MemoryBrowser) Render() string {
    leftW, rightW := b.paneSizes()
    leftPane := b.renderList(leftW)
    rightPane := b.renderContent(rightW)

    split := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)

    hint := b.renderHint()
    return lipgloss.JoinVertical(lipgloss.Left, split, hint)
}

// --- private helpers ---

func (b *MemoryBrowser) paneSizes() (left, right int) {
    total := b.width
    if total < 40 {
        total = 80
    }
    left = total * 28 / 100
    right = total - left
    return
}

func (b *MemoryBrowser) loadSelected() {
    if len(b.files) == 0 {
        b.content = ""
        return
    }
    path := b.files[b.selectedIdx].Path
    absPath := filepath.Join(b.mgr.Dir(), path)
    data, err := os.ReadFile(absPath)
    if err != nil {
        b.content = fmt.Sprintf("(error reading file: %v)", err)
        return
    }
    b.content = string(data)
}

func (b *MemoryBrowser) refreshFileList() {
    files, err := b.mgr.List()
    if err == nil {
        b.files = files
    }
}

var (
    listBorderStyle = lipgloss.NewStyle().
            Border(lipgloss.RoundedBorder()).
            BorderForeground(lipgloss.Color("#555555"))

    listFocusedStyle = lipgloss.NewStyle().
                Border(lipgloss.RoundedBorder()).
                BorderForeground(lipgloss.Color("#F0A868"))

    selectedFileStyle = lipgloss.NewStyle().
                Foreground(lipgloss.Color("#1a1a1a")).
                Background(lipgloss.Color("#7EC8D8"))

    fileStyle = lipgloss.NewStyle().
            Foreground(lipgloss.Color("#dddddd"))

    fileMtimeStyle = lipgloss.NewStyle().
            Foreground(lipgloss.Color("#555555"))

    contentBorderStyle = lipgloss.NewStyle().
                Border(lipgloss.RoundedBorder()).
                BorderForeground(lipgloss.Color("#555555"))

    contentFocusedStyle = lipgloss.NewStyle().
                Border(lipgloss.RoundedBorder()).
                BorderForeground(lipgloss.Color("#F0A868"))

    editingBorderStyle = lipgloss.NewStyle().
                Border(lipgloss.RoundedBorder()).
                BorderForeground(lipgloss.Color("#36CFC9"))

    browserHintStyle = lipgloss.NewStyle().
                Foreground(lipgloss.Color("#555555")).
                MarginTop(0)

    titleStyle = lipgloss.NewStyle().
            Foreground(lipgloss.Color("#F0A868")).
            Bold(true)
)

func (b *MemoryBrowser) renderList(width int) string {
    innerW := width - 4 // border + padding
    if innerW < 4 {
        innerW = 4
    }

    var sb strings.Builder
    sb.WriteString(titleStyle.Render("Memory Files") + "\n\n")

    if len(b.files) == 0 {
        sb.WriteString(fileMtimeStyle.Render("(no memory files)"))
    }

    for i, f := range b.files {
        name := f.Path
        mtime := time.UnixMilli(f.UpdatedAt).Format("2006-01-02")

        // Truncate name to fit
        maxName := innerW - len(mtime) - 2
        if maxName < 4 {
            maxName = 4
        }
        if len(name) > maxName {
            name = name[:maxName-1] + "…"
        }

        padding := innerW - len(name) - len(mtime)
        if padding < 1 {
            padding = 1
        }
        line := name + strings.Repeat(" ", padding) + mtime

        if i == b.selectedIdx {
            sb.WriteString(selectedFileStyle.Width(innerW).Render(line) + "\n")
        } else {
            sb.WriteString(fileStyle.Render(line) + "\n")
        }
    }

    content := lipgloss.NewStyle().Width(innerW).Height(b.height - 5).Render(sb.String())

    borderSt := listBorderStyle
    if b.focus == focusList {
        borderSt = listFocusedStyle
    }
    return borderSt.Width(width - 2).Render(content)
}

func (b *MemoryBrowser) renderContent(width int) string {
    innerW := width - 4
    if innerW < 10 {
        innerW = 10
    }

    title := b.SelectedFile()
    if title == "" {
        title = "—"
    }
    if b.editing {
        title += " [editing]"
    }

    var body string
    if b.editing {
        b.textarea.SetWidth(innerW)
        b.textarea.SetHeight(b.height - 6)
        body = b.textarea.View()
    } else {
        body = lipgloss.NewStyle().
            Width(innerW).
            Height(b.height - 6).
            Render(b.content)
    }

    if b.confirmDelete {
        prompt := lipgloss.NewStyle().
            Foreground(lipgloss.Color("#FF6B6B")).
            Bold(true).
            Render(fmt.Sprintf("Delete %s? [y/n]", b.SelectedFile()))
        body = prompt + "\n" + body
    }

    full := titleStyle.Render(title) + "\n\n" + body

    borderSt := contentBorderStyle
    if b.editing {
        borderSt = editingBorderStyle
    } else if b.focus == focusContent {
        borderSt = contentFocusedStyle
    }
    return borderSt.Width(width - 2).Render(full)
}

func (b *MemoryBrowser) renderHint() string {
    if b.editing {
        return browserHintStyle.Render("  [ctrl+s] save  [esc] cancel edit")
    }
    if b.confirmDelete {
        return browserHintStyle.Render("  [y] confirm delete  [n] cancel")
    }
    return browserHintStyle.Render("  [e] edit  [d] delete  [tab] switch pane  [esc] close")
}

func max(a, b int) int {
    if a > b {
        return a
    }
    return b
}
```

**Step 4: Add `Dir()` method to `memory.Manager`**

The browser needs the memory directory to build absolute paths for `os.WriteFile`. In `memory/manager.go`, after `SetReranker`:

```go
// Dir returns the root directory of the memory store.
func (m *Manager) Dir() string { return m.dir }
```

**Step 5: Run tests**

```bash
go test ./ui/... -run "TestMemoryBrowser" -v
```

Expected: all 4 tests PASS.

**Step 6: Build**

```bash
go build ./...
```

Expected: no errors.

**Step 7: Commit**

```bash
git add ui/memory_browser.go ui/memory_browser_test.go memory/manager.go
git commit -m "feat: add MemoryBrowser split-pane component with edit/delete"
```

---

### Task 5: Wire `stateMemoryBrowser` into the app

**Files:**
- Modify: `app/app.go`
- Modify: `app/app_input.go`
- Modify: `app/commands.go`

**Step 1: Add state and field to `app/app.go`**

In the `state` const block, after `stateNewAutomation` (line 89), add:

```go
// stateMemoryBrowser is the state when the memory file browser is open.
stateMemoryBrowser
```

In the `home` struct, after `settingsOverlay *overlay.SettingsOverlay` (line 181), add:

```go
// memoryBrowser is the memory file browser screen.
memoryBrowser *ui.MemoryBrowser
```

**Step 2: Add to the state guard in `app_input.go`**

In `app_input.go` line 27, the long `if m.state == ...` guard that prevents global key handling when overlays are open — add `|| m.state == stateMemoryBrowser` to that condition.

**Step 3: Add the dispatcher case in `app_input.go`**

In the `handleKeyPress` switch (around line 318), after the `stateAutomations` case, add:

```go
case stateMemoryBrowser:
    return m.handleMemoryBrowserKeys(msg)
```

**Step 4: Add `handleMemoryBrowserKeys` in `app_input.go`**

After `handleSettingsKeys`, add:

```go
func (m *home) handleMemoryBrowserKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
    if m.memoryBrowser == nil {
        m.state = stateDefault
        return m, nil
    }
    cmd, close := m.memoryBrowser.HandleKeyPress(msg)
    if close {
        m.memoryBrowser = nil
        m.state = stateDefault
        return m, tea.WindowSize()
    }
    return m, cmd
}
```

**Step 5: Add `openMemoryBrowser` and command palette entry in `app/commands.go`**

In `buildCommandItems`, after the `"Settings"` entry, add:

```go
{Label: "Memory Browser", Description: "Browse, edit and delete memory files", Shortcut: "M", Category: "System", Action: "cmd_memory_browser"},
```

In `executeCommandPaletteAction`, after `case "cmd_settings":`, add:

```go
case "cmd_memory_browser":
    return m.openMemoryBrowser()
```

Then add the `openMemoryBrowser` function after `openSettings`:

```go
// openMemoryBrowser opens the full-screen memory file browser.
func (m *home) openMemoryBrowser() (tea.Model, tea.Cmd) {
    mgr := session.GetMemoryManager()
    if mgr == nil {
        m.toastManager.Push("Memory is disabled. Enable it in Settings.", 3*time.Second)
        return m, nil
    }
    browser, err := ui.NewMemoryBrowser(mgr)
    if err != nil {
        return m, m.handleError(fmt.Errorf("open memory browser: %w", err))
    }
    browser.SetSize(m.width, m.contentHeight)
    m.memoryBrowser = browser
    m.state = stateMemoryBrowser
    return m, nil
}
```

Add `"time"` to imports in `app/commands.go` if not already present.

**Step 6: Add `M` key trigger in `handleDefaultKeys` in `app_input.go`**

Find the section in `handleDefaultKeys` that handles key presses for single characters (around where `ctrl+p` opens the command palette), and add:

```go
case keys.GlobalKeyStringsMap["M"] == keys.KeyMemoryBrowser:
    return m.openMemoryBrowser()
```

Or more precisely, find the existing pattern used for other keys. Looking at the code, the default key handler uses a `switch msg.String()` pattern. Add:

```go
case "M":
    return m.openMemoryBrowser()
```

**Step 7: Add `View()` case in `app/app.go`**

In `View()`, after the `stateAutomations` case (around line 736), add:

```go
case m.state == stateMemoryBrowser && m.memoryBrowser != nil:
    result = m.memoryBrowser.Render()
```

**Step 8: Handle window resize for memory browser**

Find the `WindowSizeMsg` handler in `app/app.go` (search for `tea.WindowSizeMsg`). After the line that updates `m.contentHeight`, add:

```go
if m.memoryBrowser != nil {
    m.memoryBrowser.SetSize(msg.Width, m.contentHeight)
}
```

**Step 9: Build**

```bash
go build ./...
```

Expected: no errors.

**Step 10: Manual end-to-end test**

1. Start Hivemind with `memory.enabled: true` in config.
2. Press `M` from the main screen.
3. Verify the memory browser opens with a file list on the left and content on the right.
4. Press `↓` to move to the next file — content pane should update.
5. Press `tab` — focus border should move to the right pane.
6. Press `e` — right pane becomes editable (border turns cyan).
7. Type some text, press `ctrl+s` — changes are saved.
8. Press `d` on a file — confirmation prompt appears. Press `y` — file is deleted.
9. Press `esc` — browser closes and main screen is restored.
10. If memory is disabled and you press `M` — a toast notification appears.

**Step 11: Commit**

```bash
git add app/app.go app/app_input.go app/commands.go
git commit -m "feat: wire stateMemoryBrowser into app — M key opens memory browser"
```

---

### Task 6: Push and verify CI

```bash
git push origin fabian.urbanek/memory
```

Check that:
- `go test ./...` passes (all packages)
- `gofmt -l .` returns no files
- PR #2 CI checks go green
