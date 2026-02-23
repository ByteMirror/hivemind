# Memory UI — Design

**Date**: 2026-02-23
**Status**: Approved
**Scope**: Memory settings in the existing settings overlay + full-screen memory file browser with edit/delete

---

## Problem

The memory system requires manual JSON editing of `~/.hivemind/config.json` to enable and configure. There is also no way to browse, edit, or delete memory files from within the TUI. Both need to be fixed to make memory feel like a first-class feature.

---

## Part 1: Memory Settings (in existing settings overlay)

### Approach

Extend `app/commands.go:openSettings()` to append a "Memory" section to the existing `SettingsOverlay`. The overlay already supports `SettingToggle`, `SettingPicker`, and `SettingText` item types — no new overlay component needed.

### Items

| Key | Type | Label | Notes |
|-----|------|-------|-------|
| `memory.enabled` | `SettingToggle` | `Memory enabled` | |
| `memory.embedding_provider` | `SettingPicker` | `Search provider` | Options: `none`, `claude`, `openai`, `ollama` |
| `memory.claude_model` | `SettingText` | `Claude model` | Only shown when provider = `claude` |
| `memory.openai_api_key` | `SettingText` | `OpenAI API key` | Only shown when provider = `openai` |
| `memory.openai_model` | `SettingText` | `OpenAI model` | Only shown when provider = `openai` |
| `memory.ollama_url` | `SettingText` | `Ollama URL` | Only shown when provider = `ollama` |
| `memory.ollama_model` | `SettingText` | `Ollama model` | Only shown when provider = `ollama` |
| `memory.startup_inject_count` | `SettingText` | `Startup snippets` | Number as string; validated on apply |

### Dynamic Visibility

Items whose visibility depends on the selected provider are filtered inside `openSettings()` when building the item list. When the provider picker changes, `applySettingChange("memory.embedding_provider")` rebuilds the overlay with the updated item list.

### Live Restart

When `memory.enabled` or `memory.embedding_provider` changes, `applySettingChange` calls `memory.NewManagerFromConfig(m.appConfig)` and re-calls `session.SetMemoryManager(...)` so the new config takes effect immediately without a TUI restart.

---

## Part 2: Memory Browser

### Layout

Full-screen state (`stateMemoryBrowser`) that replaces the main content area, following the same pattern as `stateAutomations`.

```
┌─ Memory Files ──────────┬─ global.md ──────────────────────────────────┐
│                         │                                                │
│ > global.md       2.1KB │  # My Setup                                   │
│   2026-02-23      3.4KB │                                                │
│   2026-02-22      1.2KB │  MacBook Pro M3 Max. macOS Sequoia.           │
│                         │  Terminal: Ghostty. Shell: zsh.               │
│                         │  Language preference: Go for backend.          │
│                         │                                                │
└─────────────────────────┴──────────────────────────────────────────────┘
  [e] edit  [d] delete  [tab] switch pane  [esc] close
```

In edit mode, the right pane becomes an editable `textarea` (from `github.com/charmbracelet/bubbles/textarea`):

```
┌─ Memory Files ──────────┬─ global.md [editing] ────────────────────────┐
│                         │                                                │
│ > global.md       2.1KB │  # My Setup                                   │
│   2026-02-23      3.4KB │                                                │
│   2026-02-22      1.2KB │  MacBook Pro M3 Max. macOS Sequoia.█          │
│                         │  Terminal: Ghostty. Shell: zsh.               │
│                         │                                                │
└─────────────────────────┴──────────────────────────────────────────────┘
  [ctrl+s] save  [esc] cancel edit
```

### Component: `ui/memory_browser.go`

```go
type MemoryBrowser struct {
    files       []memory.FileInfo  // from manager.List()
    selectedIdx int
    content     string             // raw file content
    textarea    textarea.Model     // bubbles textarea
    editing     bool
    focusRight  bool               // tab toggles pane focus
    width       int
    height      int
    mgr         *memory.Manager
}

func NewMemoryBrowser(mgr *memory.Manager) (*MemoryBrowser, error)
func (b *MemoryBrowser) HandleKeyPress(msg tea.KeyMsg) (cmd tea.Cmd, close bool)
func (b *MemoryBrowser) SetSize(width, height int)
func (b *MemoryBrowser) Render() string
func (b *MemoryBrowser) loadSelected() error      // reads file content
func (b *MemoryBrowser) saveSelected() error      // writes + re-indexes
func (b *MemoryBrowser) deleteSelected() error    // deletes file + re-indexes
```

### Key Bindings

| Key | Pane focus | Action |
|-----|-----------|--------|
| `↑` / `↓` | left | Navigate file list |
| `Enter` | left | Load selected file into right pane |
| `Tab` | any | Toggle focus between left and right |
| `e` | left or right (browse) | Enter edit mode |
| `Ctrl+S` | right (edit) | Save edits, re-index |
| `Esc` | right (edit) | Cancel edit, restore original |
| `Esc` | left | Close memory browser |
| `d` | left | Open `ConfirmationOverlay` → delete file on confirm |

### App Integration

**`app/app.go`**:
- Add `stateMemoryBrowser` to `state` enum
- Add `memoryBrowser *ui.MemoryBrowser` and `confirmDeleteMemory *overlay.ConfirmationOverlay` fields to `home`

**`app/app_input.go`**:
- Add `case stateMemoryBrowser:` → `m.handleMemoryBrowserKeys(msg)`

**`app/commands.go`**:
- Add `openMemoryBrowser()` — calls `ui.NewMemoryBrowser(session.GetMemoryManager())`, sets state

**`app/app.go` View()**:
- Add case rendering `m.memoryBrowser.Render()` as the full content area when `m.state == stateMemoryBrowser`

### Opening the Browser

Add key binding `M` (shift+m) in `handleDefaultKeys` to call `openMemoryBrowser()`. Show it in the bottom menu bar with a label like `[M] memory`.

### Delete Flow

1. User presses `d` in browser
2. `handleMemoryBrowserKeys` opens `ConfirmationOverlay("Delete global.md?")`
3. State becomes `stateMemoryBrowser` + overlay active flag
4. On confirm: `manager` deletes the file from disk and re-syncs the index; file list refreshes
5. On cancel: overlay dismissed, back to normal browse

---

## Key Files Changed / Created

| File | Change |
|------|--------|
| `ui/memory_browser.go` | New: `MemoryBrowser` component |
| `app/app.go` | Add `stateMemoryBrowser`, `memoryBrowser` field, `View()` case |
| `app/app_input.go` | Add `handleMemoryBrowserKeys()` |
| `app/commands.go` | Add `openMemoryBrowser()`, extend `openSettings()` and `applySettingChange()` |
| `session/memory_state.go` | Export `GetMemoryManager()` accessor |
| `keys/keys.go` | Add `OpenMemoryBrowser` key binding |

---

## Out of Scope

- Creating new memory files from within the browser (use agents for that)
- Inline search within the browser (covered by `memory_search` MCP tool)
- Syntax highlighting for Markdown in the preview pane
