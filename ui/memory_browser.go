package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
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

// memoryFile combines tree metadata with list metadata for display.
type memoryFile struct {
	Path        string
	Description string
	IsSystem    bool
	SizeBytes   int64
	UpdatedAt   int64 // Unix ms
}

// MemoryBrowser is a full-screen split-pane memory file viewer and editor.
type MemoryBrowser struct {
	mgr             *memory.Manager
	files           []memoryFile
	selectedIdx     int
	content         string // file body (frontmatter stripped) for the selected file
	originalContent string // content before edit started

	editing  bool
	textarea textarea.Model
	viewport viewport.Model
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

	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.CharLimit = 0
	ta.MaxHeight = 0

	b := &MemoryBrowser{
		mgr:      mgr,
		textarea: ta,
		viewport: viewport.New(0, 0),
		focus:    focusList,
	}

	b.refreshFileList()
	if len(b.files) > 0 {
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

// Content returns the body of the currently loaded file (frontmatter stripped).
func (b *MemoryBrowser) Content() string { return b.content }

// IsEditing returns true when the right pane is in edit mode.
func (b *MemoryBrowser) IsEditing() bool { return b.editing }

// EnterEditMode switches the right pane into an editable textarea.
func (b *MemoryBrowser) EnterEditMode() {
	b.confirmDelete = false
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

// SaveEdit writes the textarea content via the Manager and re-indexes.
func (b *MemoryBrowser) SaveEdit() error {
	if !b.editing {
		return nil
	}
	path := b.SelectedFile()
	if path == "" {
		return fmt.Errorf("no file selected")
	}
	newContent := b.textarea.Value()
	if err := b.mgr.WriteFile(path, newContent, ""); err != nil {
		return fmt.Errorf("save %s: %w", path, err)
	}
	b.content = newContent
	b.editing = false
	b.textarea.Blur()
	b.focus = focusList
	b.refreshFileList()
	return nil
}

// DeleteSelected deletes the selected file via the Manager.
func (b *MemoryBrowser) DeleteSelected() error {
	path := b.SelectedFile()
	if path == "" {
		return nil
	}
	if err := b.mgr.Delete(path); err != nil {
		return fmt.Errorf("delete %s: %w", path, err)
	}
	b.refreshFileList()
	if b.selectedIdx >= len(b.files) {
		b.selectedIdx = browserMax(0, len(b.files)-1)
	}
	b.loadSelected()
	return nil
}

// PinSelected moves the selected file into system/ (always-in-context).
func (b *MemoryBrowser) PinSelected() error {
	path := b.SelectedFile()
	if path == "" {
		return nil
	}
	if err := b.mgr.Pin(path); err != nil {
		return err
	}
	b.refreshFileList()
	b.loadSelected()
	return nil
}

// UnpinSelected moves the selected file out of system/ back to root.
func (b *MemoryBrowser) UnpinSelected() error {
	path := b.SelectedFile()
	if path == "" {
		return nil
	}
	if err := b.mgr.Unpin(path); err != nil {
		return err
	}
	b.refreshFileList()
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
	contentH := height - 6
	if contentH < 1 {
		contentH = 1
	}
	b.viewport.Width = rightW - 4
	b.viewport.Height = contentH
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
		if b.confirmDelete {
			b.confirmDelete = false
			return nil, false
		}
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
		} else if b.focus == focusContent {
			b.ScrollUp(1)
		}
	case "down", "j":
		if b.focus == focusList {
			b.SelectNext()
		} else if b.focus == focusContent {
			b.ScrollDown(1)
		}
	case "enter":
		if b.focus == focusList {
			b.loadSelected()
		}
	case "e":
		if b.SelectedFile() != "" && !b.confirmDelete {
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
	case "p":
		if b.SelectedFile() != "" && !b.confirmDelete {
			sel := b.selectedFile()
			if sel != nil && !sel.IsSystem {
				_ = b.PinSelected()
			}
		}
	case "u":
		if b.SelectedFile() != "" && !b.confirmDelete {
			sel := b.selectedFile()
			if sel != nil && sel.IsSystem {
				_ = b.UnpinSelected()
			}
		}
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

func (b *MemoryBrowser) selectedFile() *memoryFile {
	if len(b.files) == 0 || b.selectedIdx >= len(b.files) {
		return nil
	}
	return &b.files[b.selectedIdx]
}

func (b *MemoryBrowser) paneSizes() (left, right int) {
	total := b.width
	if total < 40 {
		total = 80
	}
	left = total * 30 / 100
	right = total - left
	return
}

func (b *MemoryBrowser) loadSelected() {
	if len(b.files) == 0 {
		b.content = ""
		b.viewport.SetContent("")
		return
	}
	path := b.files[b.selectedIdx].Path
	body, err := b.mgr.Read(path)
	if err != nil {
		b.content = fmt.Sprintf("(error reading file: %v)", err)
		b.viewport.SetContent(b.content)
		b.viewport.GotoTop()
		return
	}
	b.content = body
	b.viewport.SetContent(b.content)
	b.viewport.GotoTop()
}

// ScrollUp scrolls the content pane up by n lines.
func (b *MemoryBrowser) ScrollUp(n int) {
	for range n {
		b.viewport.LineUp(1)
	}
}

// ScrollDown scrolls the content pane down by n lines.
func (b *MemoryBrowser) ScrollDown(n int) {
	for range n {
		b.viewport.LineDown(1)
	}
}

func (b *MemoryBrowser) refreshFileList() {
	tree, treeErr := b.mgr.Tree()
	list, listErr := b.mgr.List()
	if treeErr != nil && listErr != nil {
		return
	}

	// Build a lookup from path → list metadata (for timestamps).
	mtimeMap := make(map[string]int64, len(list))
	for _, f := range list {
		mtimeMap[f.Path] = f.UpdatedAt
	}

	var files []memoryFile
	if treeErr == nil {
		for _, t := range tree {
			files = append(files, memoryFile{
				Path:        t.Path,
				Description: t.Description,
				IsSystem:    t.IsSystem,
				SizeBytes:   t.SizeBytes,
				UpdatedAt:   mtimeMap[t.Path],
			})
		}
	} else {
		// Fallback to flat list if tree fails.
		for _, f := range list {
			files = append(files, memoryFile{
				Path:      f.Path,
				SizeBytes: f.SizeBytes,
				UpdatedAt: f.UpdatedAt,
			})
		}
	}
	b.files = files
}

// --- styles ---

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

	browserSystemFileStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F0A868"))

	browserFileMtimeStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#555555"))

	browserDescStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#777777")).
				Italic(true)

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
		// Build the display name with system indicator.
		name := f.Path
		prefix := "  "
		if f.IsSystem {
			prefix = "\U0001F4CC " // 📌
		}

		mtime := ""
		if f.UpdatedAt > 0 {
			mtime = time.UnixMilli(f.UpdatedAt).Format("Jan 02")
		}

		// Truncate name to fit.
		maxName := innerW - len(prefix) - len(mtime) - 2
		if maxName < 4 {
			maxName = 4
		}
		if len(name) > maxName {
			name = name[:maxName-1] + "…"
		}

		padding := innerW - len(prefix) - len(name) - len(mtime)
		if padding < 1 {
			padding = 1
		}
		line := prefix + name + strings.Repeat(" ", padding) + mtime

		if i == b.selectedIdx {
			sb.WriteString(browserSelectedFileStyle.Width(innerW).Render(line) + "\n")
		} else if f.IsSystem {
			sb.WriteString(browserSystemFileStyle.Render(line) + "\n")
		} else {
			sb.WriteString(browserFileStyle.Render(line) + "\n")
		}

		// Show description on next line if available.
		if f.Description != "" {
			desc := f.Description
			maxDesc := innerW - 4
			if len(desc) > maxDesc {
				desc = desc[:maxDesc-1] + "…"
			}
			sb.WriteString(browserDescStyle.Render("    "+desc) + "\n")
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
	sel := b.selectedFile()
	if sel != nil && sel.IsSystem {
		title = "\U0001F4CC " + title // 📌
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
		b.viewport.Width = innerW
		contentH := b.height - 6
		if contentH < 1 {
			contentH = 1
		}
		b.viewport.Height = contentH
		body = b.viewport.View()
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
	sel := b.selectedFile()
	if sel != nil && sel.IsSystem {
		return browserHintStyle.Render("  [e] edit  [u] unpin  [d] delete  [tab] switch pane  [esc] close")
	}
	return browserHintStyle.Render("  [e] edit  [p] pin  [d] delete  [tab] switch pane  [esc] close")
}

func browserMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}
