package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ByteMirror/hivemind/session"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

var (
	AdditionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e"))
	DeletionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#ef4444"))
	HunkStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#0ea5e9"))

	fileItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F0A868"))
	fileItemSelectedStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#F0A868")).
				Foreground(lipgloss.Color("#1a1a1a")).
				Bold(true)
	fileItemDimStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#999999", Dark: "#666666"})
	filePanelBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.AdaptiveColor{Light: "#d0d0d0", Dark: "#333333"})
	filePanelBorderFocusedStyle = lipgloss.NewStyle().
					Border(lipgloss.RoundedBorder()).
					BorderForeground(lipgloss.Color("#F0A868"))
	diffHeaderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F0A868")).
			Bold(true)
	diffHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#aaaaaa", Dark: "#555555"})
)

// fileChunk holds a single file's parsed diff data.
type fileChunk struct {
	path    string
	added   int
	removed int
	diff    string
}


// LineComment is an annotation attached to a specific line in the diff.
type LineComment struct {
	File    string // relative file path
	Line    int    // 0-based index into the rendered diff content
	Marker  string // "+", "-", or " "
	Code    string // the line being commented on (trimmed)
	Comment string // user's comment text
}
type DiffPane struct {
	viewport viewport.Model
	width    int
	height   int

	files        []fileChunk
	totalAdded   int
	totalRemoved int
	fullDiff     string

	// selectedFile: -1 = all files, 0..N = specific file
	selectedFile int

	// sidebarWidth is computed from file names
	sidebarWidth int

	// Comment mode
	commentMode   bool
	commentCursor int                      // 0-based line index in current diff view
	comments      map[string][]LineComment // filePath → comments
}

func NewDiffPane() *DiffPane {
	return &DiffPane{
		viewport:     viewport.New(0, 0),
		selectedFile: 0,
	}
}

func (d *DiffPane) SetSize(width, height int) {
	d.width = width
	d.height = height
	d.computeSidebarWidth()
	d.updateViewportWidth()
	d.viewport.Height = height
	d.rebuildViewport()
}

func (d *DiffPane) updateViewportWidth() {
	diffWidth := d.width - d.sidebarWidth - 1
	if diffWidth < 10 {
		diffWidth = 10
	}
	d.viewport.Width = diffWidth
}

func (d *DiffPane) computeSidebarWidth() {
	// Compute the inner content width needed, then add border frame.
	borderFrame := filePanelBorderStyle.GetHorizontalFrameSize() // typically 2 for left+right border
	innerMin := 18
	innerMax := innerMin
	for _, f := range d.files {
		base := filepath.Base(f.path)
		statsW := len(fmt.Sprintf(" +%d -%d", f.added, f.removed))
		w := runewidth.StringWidth(base) + statsW + 4
		if w > innerMax {
			innerMax = w
		}
	}
	// Cap at 35% of total width (including border)
	limit := d.width*35/100 - borderFrame
	if limit < innerMin {
		limit = innerMin
	}
	if innerMax > limit {
		innerMax = limit
	}
	// sidebarWidth is the total outer width (content + border)
	d.sidebarWidth = innerMax + borderFrame
}

func (d *DiffPane) SetDiff(instance *session.Instance) {
	if instance == nil || !instance.Started() {
		d.files = nil
		d.fullDiff = ""
		return
	}

	stats := instance.GetDiffStats()
	if stats == nil || stats.Error != nil || stats.IsEmpty() {
		d.files = nil
		d.fullDiff = ""
		if stats != nil && stats.Error != nil {
			d.fullDiff = fmt.Sprintf("Error: %v", stats.Error)
		}
		return
	}

	d.totalAdded = stats.Added
	d.totalRemoved = stats.Removed
	d.files = parseFileChunks(stats.Content)
	d.fullDiff = colorizeDiff(stats.Content)

	if d.selectedFile >= len(d.files) {
		d.selectedFile = len(d.files) - 1
	}
	if d.selectedFile < -1 {
		d.selectedFile = -1
	}

	d.computeSidebarWidth()
	d.updateViewportWidth()
	d.rebuildViewport()
}

func (d *DiffPane) rebuildViewport() {
	if len(d.files) == 0 {
		return
	}
	var rawDiff string
	if d.selectedFile < 0 {
		rawDiff = d.fullDiff
	} else if d.selectedFile < len(d.files) {
		rawDiff = d.files[d.selectedFile].diff
	}
	diff := colorizeDiff(rawDiff)

	if !d.commentMode {
		d.viewport.SetContent(diff)
		return
	}

	// Comment mode: annotate with cursor indicator and comment annotations.
	cursorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#F0A868")).Bold(true)
	commentAnnotationStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#F0A868")).
		Italic(true)

	// Build a line→comment map for the current file.
	var currentFile string
	if d.selectedFile >= 0 && d.selectedFile < len(d.files) {
		currentFile = d.files[d.selectedFile].path
	}
	lineCommentMap := make(map[int]string)
	if d.comments != nil {
		for _, c := range d.comments[currentFile] {
			lineCommentMap[c.Line] = c.Comment
		}
	}

	coloredLines := strings.Split(diff, "\n")
	rawLines := strings.Split(rawDiff, "\n")
	var out strings.Builder
	for i, line := range coloredLines {
		if i == d.commentCursor {
			out.WriteString(cursorStyle.Render("▶ ") + line + "\n")
		} else {
			out.WriteString("  " + line + "\n")
		}
		// Show raw line for comment map lookup
		_ = rawLines
		if annotation, ok := lineCommentMap[i]; ok {
			out.WriteString(commentAnnotationStyle.Render("  │ ★ "+annotation) + "\n")
		}
	}
	d.viewport.SetContent(out.String())
}

func (d *DiffPane) String() string {
	if len(d.files) == 0 {
		msg := "No changes"
		if d.fullDiff != "" {
			msg = d.fullDiff
		}
		return lipgloss.Place(d.width, d.height, lipgloss.Center, lipgloss.Center, msg)
	}

	sidebar := d.renderSidebar()
	diffContent := d.viewport.View()

	// Join sidebar and diff horizontally, then truncate to exact height
	joined := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, " ", diffContent)
	lines := strings.Split(joined, "\n")
	if len(lines) > d.height {
		lines = lines[:d.height]
	}
	return strings.Join(lines, "\n")
}

// renderSidebar builds the left-hand file list panel.
func (d *DiffPane) renderSidebar() string {
	borderFrame := filePanelBorderStyle.GetHorizontalFrameSize()
	innerWidth := d.sidebarWidth - borderFrame

	var b strings.Builder

	// Header
	additions := AdditionStyle.Render(fmt.Sprintf("+%d", d.totalAdded))
	deletions := DeletionStyle.Render(fmt.Sprintf("-%d", d.totalRemoved))
	headerText := fmt.Sprintf("%s %s", additions, deletions)
	b.WriteString(headerText)
	b.WriteString("\n")

	// "All" entry
	allLabel := "\uf0ce All"
	if d.selectedFile == -1 {
		b.WriteString(fileItemSelectedStyle.Width(innerWidth).Render(" " + allLabel))
	} else {
		b.WriteString(fileItemStyle.Render(" " + allLabel))
	}
	b.WriteString("\n")

	// File entries
	for i, f := range d.files {
		base := filepath.Base(f.path)
		dir := filepath.Dir(f.path)
		isSelected := i == d.selectedFile

		// Stats suffix
		statsStr := ""
		if f.added > 0 {
			statsStr += fmt.Sprintf("+%d", f.added)
		}
		if f.removed > 0 {
			if statsStr != "" {
				statsStr += " "
			}
			statsStr += fmt.Sprintf("-%d", f.removed)
		}

		if isSelected {
			// Truncate filename to fit
			maxName := innerWidth - runewidth.StringWidth(statsStr) - 3
			name := base
			if maxName > 3 && runewidth.StringWidth(name) > maxName {
				name = runewidth.Truncate(name, maxName, "…")
			}
			line := fmt.Sprintf(" %s %s", name, statsStr)
			b.WriteString(fileItemSelectedStyle.Width(innerWidth).Render(line))
		} else {
			// Show dir/ dimmed, filename in accent color
			maxName := innerWidth - runewidth.StringWidth(statsStr) - 3
			var nameDisplay string
			if dir != "." {
				dirPrefix := dir + "/"
				remaining := maxName - runewidth.StringWidth(dirPrefix)
				if remaining < 4 {
					// Not enough room for dir, just show filename
					name := base
					if maxName > 3 && runewidth.StringWidth(name) > maxName {
						name = runewidth.Truncate(name, maxName, "…")
					}
					nameDisplay = fileItemStyle.Render(name)
				} else {
					name := base
					if runewidth.StringWidth(name) > remaining {
						name = runewidth.Truncate(name, remaining, "…")
					}
					nameDisplay = fileItemDimStyle.Render(dirPrefix) + fileItemStyle.Render(name)
				}
			} else {
				name := base
				if maxName > 3 && runewidth.StringWidth(name) > maxName {
					name = runewidth.Truncate(name, maxName, "…")
				}
				nameDisplay = fileItemStyle.Render(name)
			}

			// Colored stats
			coloredStats := ""
			if f.added > 0 {
				coloredStats += AdditionStyle.Render(fmt.Sprintf("+%d", f.added))
			}
			if f.removed > 0 {
				if coloredStats != "" {
					coloredStats += " "
				}
				coloredStats += DeletionStyle.Render(fmt.Sprintf("-%d", f.removed))
			}

			b.WriteString(" " + nameDisplay + " " + coloredStats)
		}
		b.WriteString("\n")
	}

	// Fill remaining height with empty lines so the border stretches
	lines := 2 + len(d.files)             // header + all entry + file entries
	for i := lines; i < d.height-3; i++ { // -3 for border + hint
		b.WriteString("\n")
	}

	// Hint at the bottom
	b.WriteString(diffHintStyle.Render("↑↓ files  J/K scroll"))

	content := b.String()
	vertFrame := filePanelBorderStyle.GetVerticalFrameSize()
	innerHeight := d.height - vertFrame
	if innerHeight < 1 {
		innerHeight = 1
	}
	return filePanelBorderStyle.Width(innerWidth).Height(innerHeight).Render(content)
}

func (d *DiffPane) FileUp() {
	if len(d.files) == 0 {
		return
	}
	d.selectedFile--
	if d.selectedFile < -1 {
		d.selectedFile = len(d.files) - 1
	}
	d.rebuildViewport()
	d.viewport.GotoTop()
}

func (d *DiffPane) FileDown() {
	if len(d.files) == 0 {
		return
	}
	d.selectedFile++
	if d.selectedFile >= len(d.files) {
		d.selectedFile = -1
	}
	d.rebuildViewport()
	d.viewport.GotoTop()
}

func (d *DiffPane) ScrollUp() {
	d.viewport.LineUp(3)
}

func (d *DiffPane) ScrollDown() {
	d.viewport.LineDown(3)
}

func (d *DiffPane) HasFiles() bool {
	return len(d.files) > 0
}

// GetSelectedFilePath returns the relative path of the currently selected file,
// or empty string if no specific file is selected (e.g. "All" is selected).
func (d *DiffPane) GetSelectedFilePath() string {
	if d.selectedFile < 0 || d.selectedFile >= len(d.files) {
		return ""
	}
	return d.files[d.selectedFile].path
}


// AddComment adds a comment annotation to the given file at the given line index.
func (d *DiffPane) AddComment(file string, line int, marker, code, comment string) {
	if d.comments == nil {
		d.comments = make(map[string][]LineComment)
	}
	d.comments[file] = append(d.comments[file], LineComment{
		File: file, Line: line, Marker: marker, Code: code, Comment: comment,
	})
}

// GetComments returns all stored comments, keyed by file path.
func (d *DiffPane) GetComments() map[string][]LineComment {
	if d.comments == nil {
		return map[string][]LineComment{}
	}
	return d.comments
}

// ClearComments removes all stored comments.
func (d *DiffPane) ClearComments() {
	d.comments = nil
}

// HasComments returns true if there is at least one comment stored.
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

// EnterCommentMode enters comment mode, resetting the cursor to line 0.
func (d *DiffPane) EnterCommentMode() {
	d.commentMode = true
	d.commentCursor = 0
}

// ExitCommentMode exits comment mode.
func (d *DiffPane) ExitCommentMode() {
	d.commentMode = false
}

// IsCommentMode returns true when comment mode is active.
func (d *DiffPane) IsCommentMode() bool {
	return d.commentMode
}

// CommentCursorDown moves the comment cursor down one line.
func (d *DiffPane) CommentCursorDown() {
	diff := d.currentRawDiff()
	lines := strings.Split(diff, "\n")
	if d.commentCursor < len(lines)-1 {
		d.commentCursor++
	}
	d.rebuildViewport()
}

// CommentCursorUp moves the comment cursor up one line.
func (d *DiffPane) CommentCursorUp() {
	if d.commentCursor > 0 {
		d.commentCursor--
	}
	d.rebuildViewport()
}

// currentRawDiff returns the raw (uncolorized) diff for the currently selected file/view.
func (d *DiffPane) currentRawDiff() string {
	if d.selectedFile < 0 {
		return d.fullDiff
	}
	if d.selectedFile < len(d.files) {
		return d.files[d.selectedFile].diff
	}
	return ""
}

// GetCursorLineInfo returns the file, marker, and code for the current cursor line.
// Returns empty strings if line is not a diff line (e.g. hunk header).
func (d *DiffPane) GetCursorLineInfo() (file, marker, code string, lineIdx int) {
	// Use the selected file path
	if d.selectedFile >= 0 && d.selectedFile < len(d.files) {
		file = d.files[d.selectedFile].path
	}
	diff := d.currentRawDiff()
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

// parseFileChunks splits a unified diff into per-file chunks with stats.
func parseFileChunks(content string) []fileChunk {
	var chunks []fileChunk
	var current *fileChunk
	var currentLines strings.Builder

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git ") {
			if current != nil {
				current.diff = currentLines.String()
				currentLines.Reset()
			}
			parts := strings.SplitN(line, " b/", 2)
			path := ""
			if len(parts) == 2 {
				path = parts[1]
			}
			chunks = append(chunks, fileChunk{path: path})
			current = &chunks[len(chunks)-1]
			currentLines.WriteString(line + "\n")
		} else if current != nil {
			currentLines.WriteString(line + "\n")
			if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
				current.added++
			} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
				current.removed++
			}
		}
	}
	if current != nil {
		current.diff = currentLines.String()
	}
	return chunks
}

func colorizeDiff(diff string) string {
	var coloredOutput strings.Builder
	lines := strings.Split(diff, "\n")
	for _, line := range lines {
		if len(line) > 0 {
			if strings.HasPrefix(line, "@@") {
				coloredOutput.WriteString(HunkStyle.Render(line) + "\n")
			} else if line[0] == '+' && (len(line) == 1 || line[1] != '+') {
				coloredOutput.WriteString(AdditionStyle.Render(line) + "\n")
			} else if line[0] == '-' && (len(line) == 1 || line[1] != '-') {
				coloredOutput.WriteString(DeletionStyle.Render(line) + "\n")
			} else {
				coloredOutput.WriteString(line + "\n")
			}
		} else {
			coloredOutput.WriteString("\n")
		}
	}
	return coloredOutput.String()
}
