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
	mgr             *memory.Manager
	files           []memory.FileInfo
	selectedIdx     int
	content         string // raw file content for the selected file
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
		b.selectedIdx = browserMax(0, len(b.files)-1)
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
	browserListBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#555555"))

	browserListFocusedStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#F0A868"))

	browserSelectedFileStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#1a1a1a")).
					Background(lipgloss.Color("#7EC8D8"))

	browserFileStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#dddddd"))

	browserFileMtimeStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#555555"))

	browserContentBorderStyle = lipgloss.NewStyle().
					Border(lipgloss.RoundedBorder()).
					BorderForeground(lipgloss.Color("#555555"))

	browserContentFocusedStyle = lipgloss.NewStyle().
					Border(lipgloss.RoundedBorder()).
					BorderForeground(lipgloss.Color("#F0A868"))

	browserEditingBorderStyle = lipgloss.NewStyle().
					Border(lipgloss.RoundedBorder()).
					BorderForeground(lipgloss.Color("#36CFC9"))

	browserHintStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#555555")).
				MarginTop(0)

	browserTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F0A868")).
				Bold(true)
)

func (b *MemoryBrowser) renderList(width int) string {
	innerW := width - 4 // border + padding
	if innerW < 4 {
		innerW = 4
	}

	var sb strings.Builder
	sb.WriteString(browserTitleStyle.Render("Memory Files") + "\n\n")

	if len(b.files) == 0 {
		sb.WriteString(browserFileMtimeStyle.Render("(no memory files)"))
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
			sb.WriteString(browserSelectedFileStyle.Width(innerW).Render(line) + "\n")
		} else {
			sb.WriteString(browserFileStyle.Render(line) + "\n")
		}
	}

	content := lipgloss.NewStyle().Width(innerW).Height(b.height - 5).Render(sb.String())

	borderSt := browserListBorderStyle
	if b.focus == focusList {
		borderSt = browserListFocusedStyle
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

	full := browserTitleStyle.Render(title) + "\n\n" + body

	borderSt := browserContentBorderStyle
	if b.editing {
		borderSt = browserEditingBorderStyle
	} else if b.focus == focusContent {
		borderSt = browserContentFocusedStyle
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

func browserMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}
