package ui

import (
	"fmt"
	"path/filepath"
	"sort"
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
	historyDiffPreviewMaxLines = 32
)

// memoryFile combines tree metadata with list metadata for display.
type memoryFile struct {
	Path           string
	Description    string
	IsSystem       bool
	SizeBytes      int64
	UpdatedAt      int64 // Unix ms
	HistoryCommits int
	HistoryBranch  string
}

// MemoryBrowser is a full-screen split-pane memory file viewer and editor.
type MemoryBrowser struct {
	mgr              *memory.Manager
	repoMgrs         map[string]*memory.Manager
	files            []memoryFile
	selectedIdx      int
	content          string // file body (frontmatter stripped) for the selected file
	originalContent  string // content before edit started
	showHistory      bool
	history          []memory.GitLogEntry
	historyErr       string
	historyBranch    string
	historyDiff      string
	historyLimit     int
	gitCurrentBranch string
	gitBranches      []string

	branchMode      bool
	branchSelected  int
	branchStatusMsg string

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
		mgr:          mgr,
		repoMgrs:     make(map[string]*memory.Manager),
		textarea:     ta,
		viewport:     viewport.New(0, 0),
		focus:        focusList,
		historyLimit: 30,
	}

	b.refreshFileList()
	if len(b.files) > 0 {
		b.loadSelected()
	}
	return b, nil
}

// Close releases any cached repo-scoped managers opened by the browser.
func (b *MemoryBrowser) Close() {
	for slug, mgr := range b.repoMgrs {
		if mgr != nil {
			mgr.Close()
		}
		delete(b.repoMgrs, slug)
	}
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
	b.showHistory = false
	b.branchMode = false
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
	b.refreshViewportContent(false)
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
	b.loadSelected()
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

	if b.branchMode {
		switch msg.String() {
		case "esc", "b":
			b.branchMode = false
			b.branchStatusMsg = ""
			b.refreshViewportContent(true)
			return nil, false
		case "up", "k":
			if b.branchSelected > 0 {
				b.branchSelected--
				b.refreshViewportContent(true)
			}
			return nil, false
		case "down", "j":
			if _, options, _ := b.branchOptions(); b.branchSelected < len(options)-1 {
				b.branchSelected++
				b.refreshViewportContent(true)
			}
			return nil, false
		case "c":
			b.createBranchFromSelection()
			return nil, false
		case "x":
			b.deleteSelectedBranch()
			return nil, false
		case "m":
			b.mergeSelectedBranch()
			return nil, false
		default:
			return nil, false
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
	case "h":
		if b.SelectedFile() != "" && !b.confirmDelete {
			b.showHistory = !b.showHistory
			b.refreshViewportContent(true)
		}
	case "f":
		if b.showHistory && b.SelectedFile() != "" && !b.confirmDelete {
			b.cycleHistoryBranchFilter()
			b.refreshViewportContent(true)
		}
	case "b":
		if !b.confirmDelete && b.mgr.GitEnabled() {
			b.branchMode = !b.branchMode
			if !b.branchMode {
				b.branchStatusMsg = ""
			}
			b.refreshViewportContent(true)
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
		b.history = nil
		b.historyErr = ""
		b.historyDiff = ""
		b.viewport.SetContent("")
		return
	}
	path := b.files[b.selectedIdx].Path
	mgr, relPath, routeErr := b.managerForFile(path)
	if routeErr != nil {
		b.content = fmt.Sprintf("(error selecting file: %v)", routeErr)
		b.history = nil
		b.historyErr = routeErr.Error()
		b.historyDiff = ""
		b.refreshViewportContent(true)
		return
	}
	body, err := mgr.Read(relPath)
	if err != nil {
		b.content = fmt.Sprintf("(error reading file: %v)", err)
		b.history = nil
		b.historyErr = ""
		b.historyDiff = ""
		b.refreshViewportContent(true)
		return
	}
	b.content = body
	b.loadHistorySelected()
	b.refreshViewportContent(true)
}

func (b *MemoryBrowser) loadHistorySelected() {
	b.history = nil
	b.historyErr = ""
	b.historyDiff = ""
	path := b.SelectedFile()
	if path == "" {
		return
	}
	mgr, relPath, err := b.managerForFile(path)
	if err != nil {
		b.historyErr = err.Error()
		return
	}
	entries, err := mgr.HistoryWithBranch(relPath, b.historyLimit, b.historyBranch)
	if err != nil {
		b.historyErr = err.Error()
		return
	}
	b.history = entries
	if len(entries) > 0 && entries[0].ParentSHA != "" {
		diff, diffErr := mgr.DiffRefs(entries[0].ParentSHA, entries[0].SHA, relPath)
		if diffErr == nil {
			b.historyDiff = strings.TrimSpace(diff)
		}
	}
}

func (b *MemoryBrowser) refreshViewportContent(resetTop bool) {
	if b.branchMode {
		b.viewport.SetContent(b.renderBranches())
	} else if b.showHistory {
		b.viewport.SetContent(b.renderHistory())
	} else {
		b.viewport.SetContent(b.content)
	}
	if resetTop {
		b.viewport.GotoTop()
	}
}

func (b *MemoryBrowser) renderHistory() string {
	path := b.SelectedFile()
	mgr := b.mgr
	if path != "" {
		if scopedMgr, _, err := b.managerForFile(path); err == nil && scopedMgr != nil {
			mgr = scopedMgr
		}
	}
	if mgr == nil || !mgr.GitEnabled() {
		return "Git versioning is disabled for memory."
	}
	if b.historyErr != "" {
		return fmt.Sprintf("(error loading history: %s)", b.historyErr)
	}
	if len(b.history) == 0 {
		return "(no git history for this file yet)"
	}

	var sb strings.Builder
	if b.historyBranch != "" {
		sb.WriteString(fmt.Sprintf("branch filter: %s\n\n", b.historyBranch))
	}
	for _, entry := range b.history {
		sha := entry.SHA
		if len(sha) > 7 {
			sha = sha[:7]
		}
		ts := entry.Date
		if t, err := time.Parse(time.RFC3339, entry.Date); err == nil {
			ts = t.Format("2006-01-02 15:04")
		}
		extra := ""
		if entry.AuthorName != "" {
			extra += " by " + entry.AuthorName
		}
		if entry.Additions > 0 || entry.Deletions > 0 {
			extra += fmt.Sprintf(" (+%d/-%d)", entry.Additions, entry.Deletions)
		}
		if entry.Branch != "" {
			extra += " [" + entry.Branch + "]"
		}
		sb.WriteString(fmt.Sprintf("%s  %s  %s%s\n", ts, sha, entry.Message, extra))
	}
	if b.historyDiff != "" {
		sb.WriteString("\n---\n")
		sb.WriteString(browserDiffPreviewTitleStyle.Render("latest diff preview:\n"))
		sb.WriteString(renderStyledDiffPreview(b.historyDiff, historyDiffPreviewMaxLines))
	}
	return strings.TrimSpace(sb.String())
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
				Path:           t.Path,
				Description:    t.Description,
				IsSystem:       t.IsSystem,
				SizeBytes:      t.SizeBytes,
				UpdatedAt:      mtimeMap[t.Path],
				HistoryCommits: 0,
				HistoryBranch:  "",
			})
		}
	} else {
		// Fallback to flat list if tree fails.
		for _, f := range list {
			files = append(files, memoryFile{
				Path:           f.Path,
				SizeBytes:      f.SizeBytes,
				UpdatedAt:      f.UpdatedAt,
				HistoryCommits: 0,
				HistoryBranch:  "",
			})
		}
	}

	// Fill git branch info and per-file history counts when git versioning is enabled.
	b.gitCurrentBranch = ""
	b.gitBranches = nil
	if b.mgr.GitEnabled() {
		if info, err := b.mgr.GitBranchInfo(); err == nil {
			b.gitCurrentBranch = info.Current
			b.gitBranches = info.All
		}

		commitCountByPath := map[string]int{}
		branchHintByPath := map[string]string{}
		mergeHistory := func(prefix string, mgr *memory.Manager) {
			if mgr == nil {
				return
			}
			info, err := mgr.GitBranchInfo()
			if err != nil {
				return
			}
			defaultBranch := info.Default
			defaultEntries, err := mgr.HistoryWithBranch("", 2000, defaultBranch)
			if err != nil {
				return
			}
			defaultSHAs := map[string]struct{}{}
			for _, e := range defaultEntries {
				defaultSHAs[e.SHA] = struct{}{}
				seenInCommit := map[string]struct{}{}
				for _, p := range e.Files {
					if !strings.HasSuffix(strings.ToLower(p), ".md") {
						continue
					}
					if _, ok := seenInCommit[p]; ok {
						continue
					}
					seenInCommit[p] = struct{}{}
					key := filepath.ToSlash(filepath.Join(prefix, p))
					commitCountByPath[key]++
				}
			}
			for _, br := range info.All {
				if br == defaultBranch {
					continue
				}
				entries, historyErr := mgr.HistoryWithBranch("", 2000, br)
				if historyErr != nil {
					continue
				}
				for _, e := range entries {
					if _, ok := defaultSHAs[e.SHA]; ok {
						continue
					}
					seenInCommit := map[string]struct{}{}
					for _, p := range e.Files {
						if !strings.HasSuffix(strings.ToLower(p), ".md") {
							continue
						}
						if _, ok := seenInCommit[p]; ok {
							continue
						}
						seenInCommit[p] = struct{}{}
						key := filepath.ToSlash(filepath.Join(prefix, p))
						commitCountByPath[key]++
						if branchHintByPath[key] == "" {
							branchHintByPath[key] = br
						}
					}
				}
			}
		}

		mergeHistory("", b.mgr)

		repoSlugs := map[string]struct{}{}
		for _, f := range files {
			if slug, _, ok := parseRepoMemoryPath(f.Path); ok {
				repoSlugs[slug] = struct{}{}
			}
		}
		for slug := range repoSlugs {
			repoMgr, repoErr := b.repoManager(slug)
			if repoErr != nil {
				continue
			}
			mergeHistory(filepath.ToSlash(filepath.Join("repos", slug)), repoMgr)
		}

		for i := range files {
			pathKey := filepath.ToSlash(files[i].Path)
			files[i].HistoryCommits = commitCountByPath[pathKey]
			files[i].HistoryBranch = branchHintByPath[pathKey]
		}
	}
	b.files = files
}

func (b *MemoryBrowser) repoManager(slug string) (*memory.Manager, error) {
	if slug == "" {
		return nil, fmt.Errorf("empty repo slug")
	}
	if mgr, ok := b.repoMgrs[slug]; ok && mgr != nil {
		return mgr, nil
	}
	repoDir := filepath.Join(b.mgr.Dir(), "repos", slug)
	mgr, err := memory.NewManagerWithOptions(repoDir, nil, memory.ManagerOptions{
		GitEnabled: b.mgr.GitEnabled(),
	})
	if err != nil {
		return nil, err
	}
	b.repoMgrs[slug] = mgr
	return mgr, nil
}

func parseRepoMemoryPath(path string) (slug, rel string, ok bool) {
	clean := filepath.ToSlash(filepath.Clean(path))
	if !strings.HasPrefix(clean, "repos/") {
		return "", "", false
	}
	rest := strings.TrimPrefix(clean, "repos/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func (b *MemoryBrowser) managerForFile(path string) (*memory.Manager, string, error) {
	if slug, rel, ok := parseRepoMemoryPath(path); ok {
		repoMgr, err := b.repoManager(slug)
		if err != nil {
			return nil, "", err
		}
		return repoMgr, rel, nil
	}
	return b.mgr, path, nil
}

func (b *MemoryBrowser) cycleHistoryBranchFilter() {
	mgr := b.mgr
	if path := b.SelectedFile(); path != "" {
		if scopedMgr, _, err := b.managerForFile(path); err == nil && scopedMgr != nil {
			mgr = scopedMgr
		}
	}
	if mgr == nil || !mgr.GitEnabled() {
		return
	}
	info, err := mgr.GitBranchInfo()
	if err != nil || len(info.All) == 0 {
		return
	}

	options := []string{""}
	options = append(options, info.All...)
	idx := 0
	for i := range options {
		if options[i] == b.historyBranch {
			idx = i
			break
		}
	}
	idx = (idx + 1) % len(options)
	b.historyBranch = options[idx]
	b.loadHistorySelected()
}

func (b *MemoryBrowser) branchOptions() (*memory.Manager, []string, memory.GitBranchInfo) {
	path := b.SelectedFile()
	mgr := b.mgr
	if path != "" {
		if scopedMgr, _, err := b.managerForFile(path); err == nil && scopedMgr != nil {
			mgr = scopedMgr
		}
	}
	if mgr == nil || !mgr.GitEnabled() {
		return nil, nil, memory.GitBranchInfo{}
	}
	info, err := mgr.GitBranchInfo()
	if err != nil {
		return mgr, nil, memory.GitBranchInfo{}
	}
	options := make([]string, len(info.All))
	copy(options, info.All)
	sort.Strings(options)
	if b.branchSelected >= len(options) {
		b.branchSelected = browserMax(0, len(options)-1)
	}
	return mgr, options, info
}

func (b *MemoryBrowser) createBranchFromSelection() {
	mgr, options, info := b.branchOptions()
	if mgr == nil || len(options) == 0 {
		b.branchStatusMsg = "No branches available."
		b.refreshViewportContent(true)
		return
	}
	fromRef := info.Default
	if fromRef == "" {
		fromRef = options[b.branchSelected]
	}
	name := fmt.Sprintf("memory/%s", time.Now().Format("20060102-150405"))
	if err := mgr.CreateBranch(name, fromRef); err != nil {
		b.branchStatusMsg = "Create failed: " + err.Error()
	} else {
		b.branchStatusMsg = fmt.Sprintf("Created %s from %s.", name, fromRef)
		b.refreshFileList()
	}
	b.refreshViewportContent(true)
}

func (b *MemoryBrowser) deleteSelectedBranch() {
	mgr, options, info := b.branchOptions()
	if mgr == nil || len(options) == 0 {
		b.branchStatusMsg = "No branches available."
		b.refreshViewportContent(true)
		return
	}
	name := options[b.branchSelected]
	if name == info.Default {
		b.branchStatusMsg = "Refusing to delete default branch."
		b.refreshViewportContent(true)
		return
	}
	if name == info.Current {
		b.branchStatusMsg = "Refusing to delete current branch."
		b.refreshViewportContent(true)
		return
	}
	if err := mgr.DeleteBranch(name, false); err != nil {
		b.branchStatusMsg = "Delete failed: " + err.Error()
	} else {
		b.branchStatusMsg = fmt.Sprintf("Deleted branch %s.", name)
		b.refreshFileList()
	}
	b.refreshViewportContent(true)
}

func (b *MemoryBrowser) mergeSelectedBranch() {
	mgr, options, info := b.branchOptions()
	if mgr == nil || len(options) == 0 {
		b.branchStatusMsg = "No branches available."
		b.refreshViewportContent(true)
		return
	}
	source := options[b.branchSelected]
	target := info.Default
	if target == "" {
		target = info.Current
	}
	if source == target {
		b.branchStatusMsg = "Source equals target branch."
		b.refreshViewportContent(true)
		return
	}
	if err := mgr.MergeBranch(source, target, "ff-only"); err != nil {
		b.branchStatusMsg = "Merge failed: " + err.Error()
	} else {
		b.branchStatusMsg = fmt.Sprintf("Merged %s -> %s.", source, target)
		b.refreshFileList()
		b.loadSelected()
	}
	b.refreshViewportContent(true)
}

func (b *MemoryBrowser) renderBranches() string {
	mgr, options, info := b.branchOptions()
	if mgr == nil {
		return "Git versioning is disabled for memory."
	}
	if len(options) == 0 {
		return "(no branches)"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("default: %s\ncurrent: %s\n\n", info.Default, info.Current))
	for i, name := range options {
		prefix := "  "
		if i == b.branchSelected {
			prefix = "> "
		}
		meta := ""
		if name == info.Default {
			meta = " [default]"
		} else if name == info.Current {
			meta = " [current]"
		}
		sb.WriteString(prefix + name + meta + "\n")
	}
	if b.branchStatusMsg != "" {
		sb.WriteString("\n---\n")
		sb.WriteString(b.branchStatusMsg)
	}
	return strings.TrimSpace(sb.String())
}

func truncateLines(text string, maxLines int) string {
	if maxLines <= 0 {
		return ""
	}
	lines := strings.Split(text, "\n")
	if len(lines) <= maxLines {
		return text
	}
	return strings.Join(lines[:maxLines], "\n") + "\n…"
}

type diffLineClass int

const (
	diffLineNormal diffLineClass = iota
	diffLineHeader
	diffLineMeta
	diffLineHunk
	diffLineAdd
	diffLineDel
)

func classifyDiffLine(line string) diffLineClass {
	switch {
	case strings.HasPrefix(line, "diff --git "):
		return diffLineHeader
	case strings.HasPrefix(line, "index "), strings.HasPrefix(line, "--- "), strings.HasPrefix(line, "+++ "), strings.HasPrefix(line, "Binary files"):
		return diffLineMeta
	case strings.HasPrefix(line, "@@"):
		return diffLineHunk
	case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++ "):
		return diffLineAdd
	case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "--- "):
		return diffLineDel
	default:
		return diffLineNormal
	}
}

func styleDiffLine(line string) string {
	switch classifyDiffLine(line) {
	case diffLineHeader:
		return browserDiffHeaderStyle.Render(line)
	case diffLineMeta:
		return browserDiffMetaStyle.Render(line)
	case diffLineHunk:
		return browserDiffHunkStyle.Render(line)
	case diffLineAdd:
		return browserDiffAddStyle.Render(line)
	case diffLineDel:
		return browserDiffDelStyle.Render(line)
	default:
		return line
	}
}

func renderStyledDiffPreview(diff string, maxLines int) string {
	if strings.TrimSpace(diff) == "" {
		return ""
	}
	diff = truncateLines(diff, maxLines)
	lines := strings.Split(diff, "\n")
	var sb strings.Builder
	for i, line := range lines {
		sb.WriteString(styleDiffLine(line))
		if i != len(lines)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
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

	browserDiffPreviewTitleStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#8AB4F8")).
					Bold(true)

	browserDiffHeaderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#6FA8DC")).
				Bold(true)

	browserDiffMetaStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#AAB7C4"))

	browserDiffHunkStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#E6C07B")).
				Bold(true)

	browserDiffAddStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#89D185"))

	browserDiffDelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F28B82"))
)

func (b *MemoryBrowser) renderList(width int) string {
	innerW := width - 4 // border + padding
	if innerW < 4 {
		innerW = 4
	}

	var sb strings.Builder
	listTitle := "Memory Files"
	if b.mgr.GitEnabled() {
		if b.gitCurrentBranch != "" {
			if len(b.gitBranches) > 1 {
				listTitle = fmt.Sprintf("Memory Files (branch: %s, %d branches)", b.gitCurrentBranch, len(b.gitBranches))
			} else {
				listTitle = fmt.Sprintf("Memory Files (branch: %s)", b.gitCurrentBranch)
			}
		} else {
			listTitle = "Memory Files (git)"
		}
	}
	sb.WriteString(browserTitleStyle.Render(listTitle) + "\n\n")

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
		meta := mtime
		if b.mgr.GitEnabled() {
			if meta != "" {
				meta += " "
			}
			meta += fmt.Sprintf("c:%d", f.HistoryCommits)
			if f.HistoryBranch != "" {
				meta += " br:" + f.HistoryBranch
			}
		}

		// Truncate name to fit.
		maxName := innerW - len(prefix) - len(meta) - 2
		if maxName < 4 {
			maxName = 4
		}
		if len(name) > maxName {
			name = name[:maxName-1] + "…"
		}

		padding := innerW - len(prefix) - len(name) - len(meta)
		if padding < 1 {
			padding = 1
		}
		line := prefix + name + strings.Repeat(" ", padding) + meta

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
	if b.showHistory {
		title += " [history]"
	}
	if b.branchMode {
		title += " [branches]"
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
	if b.branchMode {
		return browserHintStyle.Render("  [up/down] select branch  [c] create  [m] merge->default  [x] delete  [b/esc] close branches")
	}
	if b.confirmDelete {
		return browserHintStyle.Render("  [y] confirm delete  [n] cancel")
	}
	sel := b.selectedFile()
	if sel != nil && sel.IsSystem {
		return browserHintStyle.Render("  [h] history/content  [f] cycle history branch  [b] branches  [e] edit  [u] unpin  [d] delete  [tab] switch pane  [esc] close")
	}
	return browserHintStyle.Render("  [h] history/content  [f] cycle history branch  [b] branches  [e] edit  [p] pin  [d] delete  [tab] switch pane  [esc] close")
}

func browserMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}
