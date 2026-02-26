package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ByteMirror/hivemind/memory"
	tea "github.com/charmbracelet/bubbletea"
)

func TestMemoryBrowser_Navigation(t *testing.T) {
	dir := t.TempDir()
	// Write two memory files (avoid global.md which gets migrated to system/).
	os.WriteFile(filepath.Join(dir, "prefs.md"), []byte("# Prefs\nSetup info here."), 0600)
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# Notes\nSome notes."), 0600)

	mgr, err := memory.NewManager(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	// Sync files into index
	_ = mgr.Sync("prefs.md")
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
	body := "This is the setup memory file."
	os.WriteFile(filepath.Join(dir, "setup.md"), []byte(body), 0600)

	mgr, err := memory.NewManager(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	_ = mgr.Sync("setup.md")

	b, err := NewMemoryBrowser(mgr)
	if err != nil {
		t.Fatal(err)
	}

	got := b.Content()
	if got != body {
		t.Errorf("expected content %q, got %q", body, got)
	}
}

func TestMemoryBrowser_LoadContent_StripsFrontmatter(t *testing.T) {
	dir := t.TempDir()
	raw := "---\ndescription: Test file\n---\nActual body content."
	os.WriteFile(filepath.Join(dir, "test.md"), []byte(raw), 0600)

	mgr, err := memory.NewManager(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	_ = mgr.Sync("test.md")

	b, err := NewMemoryBrowser(mgr)
	if err != nil {
		t.Fatal(err)
	}

	got := b.Content()
	if got != "Actual body content." {
		t.Errorf("expected frontmatter stripped, got %q", got)
	}
}

func TestMemoryBrowser_EditAndSave(t *testing.T) {
	dir := t.TempDir()
	// Use notes.md (not global.md) to avoid migration to system/.
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("original"), 0600)

	mgr, err := memory.NewManager(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	_ = mgr.Sync("notes.md")

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
	saved, _ := os.ReadFile(filepath.Join(dir, "notes.md"))
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

func TestMemoryBrowser_PinUnpin(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "prefs.md"), []byte("my prefs"), 0600)

	mgr, err := memory.NewManager(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	_ = mgr.Sync("prefs.md")

	b, err := NewMemoryBrowser(mgr)
	if err != nil {
		t.Fatal(err)
	}

	// Pin: should move to system/prefs.md
	if err := b.PinSelected(); err != nil {
		t.Fatalf("pin failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "system", "prefs.md")); err != nil {
		t.Fatal("expected system/prefs.md to exist after pin")
	}
	if _, err := os.Stat(filepath.Join(dir, "prefs.md")); !os.IsNotExist(err) {
		t.Error("expected root prefs.md to be gone after pin")
	}

	// Find the pinned file and select it.
	for i, f := range b.files {
		if f.Path == filepath.Join("system", "prefs.md") {
			b.selectedIdx = i
			break
		}
	}

	// Unpin: should move back to root
	if err := b.UnpinSelected(); err != nil {
		t.Fatalf("unpin failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "prefs.md")); err != nil {
		t.Fatal("expected prefs.md at root after unpin")
	}
}

func TestMemoryBrowser_SystemFileIndicator(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "system"), 0700)
	os.WriteFile(filepath.Join(dir, "system", "conventions.md"), []byte("---\ndescription: Code conventions\n---\nUse goimports."), 0600)
	os.WriteFile(filepath.Join(dir, "daily.md"), []byte("some notes"), 0600)

	mgr, err := memory.NewManager(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	_ = mgr.Sync("system/conventions.md")
	_ = mgr.Sync("daily.md")

	b, err := NewMemoryBrowser(mgr)
	if err != nil {
		t.Fatal(err)
	}

	// Find the system file and verify it's marked as system.
	found := false
	for _, f := range b.files {
		if f.Path == filepath.Join("system", "conventions.md") {
			found = true
			if !f.IsSystem {
				t.Error("expected system/conventions.md to have IsSystem=true")
			}
			if f.Description != "Code conventions" {
				t.Errorf("expected description 'Code conventions', got %q", f.Description)
			}
		}
		if f.Path == "daily.md" {
			if f.IsSystem {
				t.Error("expected daily.md to have IsSystem=false")
			}
		}
	}
	if !found {
		t.Error("system/conventions.md not found in file list")
	}
}

func TestMemoryBrowser_GitMetadataAndHistoryToggle(t *testing.T) {
	dir := t.TempDir()
	mgr, err := memory.NewManager(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	if err := mgr.Write("first memory", "notes.md"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Write("second memory", "notes.md"); err != nil {
		t.Fatal(err)
	}

	b, err := NewMemoryBrowser(mgr)
	if err != nil {
		t.Fatal(err)
	}

	if b.gitCurrentBranch == "" {
		t.Fatal("expected git branch metadata to be loaded")
	}
	if len(b.gitBranches) == 0 {
		t.Fatal("expected git branches to be loaded")
	}

	found := false
	for _, f := range b.files {
		if f.Path == "notes.md" {
			found = true
			if f.HistoryCommits < 1 {
				t.Fatalf("expected notes.md to have history commits, got %d", f.HistoryCommits)
			}
		}
	}
	if !found {
		t.Fatal("notes.md not found in browser file list")
	}

	// Toggle history mode.
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}}
	_, _ = b.HandleKeyPress(msg)
	if !b.showHistory {
		t.Fatal("expected history mode to be enabled")
	}
	if len(b.history) == 0 {
		t.Fatal("expected selected file history entries")
	}
}

func TestMemoryBrowser_HistoryToggleWithGitDisabled(t *testing.T) {
	dir := t.TempDir()
	mgr, err := memory.NewManagerWithOptions(dir, nil, memory.ManagerOptions{GitEnabled: false})
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	if err := mgr.Write("note", "notes.md"); err != nil {
		t.Fatal(err)
	}

	b, err := NewMemoryBrowser(mgr)
	if err != nil {
		t.Fatal(err)
	}
	if b.mgr.GitEnabled() {
		t.Fatal("expected git to be disabled")
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}}
	_, _ = b.HandleKeyPress(msg)
	if !b.showHistory {
		t.Fatal("expected history mode to toggle on")
	}
	if got := b.renderHistory(); got == "" {
		t.Fatal("expected history placeholder text when git is disabled")
	}
}

func TestMemoryBrowser_HistoryFromRepoScopedMemoryStore(t *testing.T) {
	dir := t.TempDir()

	globalMgr, err := memory.NewManager(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer globalMgr.Close()

	repoDir := filepath.Join(dir, "repos", "hivemind")
	repoMgr, err := memory.NewManager(repoDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer repoMgr.Close()

	if err := repoMgr.Write("first section", "smoke.md"); err != nil {
		t.Fatal(err)
	}
	if err := repoMgr.Write("second section", "smoke.md"); err != nil {
		t.Fatal(err)
	}

	b, err := NewMemoryBrowser(globalMgr)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	target := filepath.ToSlash(filepath.Join("repos", "hivemind", "smoke.md"))
	found := false
	for i, f := range b.files {
		if filepath.ToSlash(f.Path) == target {
			b.selectedIdx = i
			found = true
			if f.HistoryCommits < 1 {
				t.Fatalf("expected history commits for %s, got %d", target, f.HistoryCommits)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected %s in memory browser list", target)
	}

	// Reload selected file and toggle history.
	b.loadSelected()
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}}
	_, _ = b.HandleKeyPress(msg)
	if !b.showHistory {
		t.Fatal("expected history mode to be enabled")
	}
	if len(b.history) == 0 {
		t.Fatal("expected repo-scoped history entries for selected file")
	}
	if got := b.renderHistory(); got == "(no git history for this file yet)" {
		t.Fatalf("expected visible history for %s, got no-history placeholder", target)
	}
}

func TestMemoryBrowser_HistoryBranchFilterCycle(t *testing.T) {
	dir := t.TempDir()
	mgr, err := memory.NewManager(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	if err := mgr.Write("root", "notes.md"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.CreateBranch("feature/ui", ""); err != nil {
		t.Fatal(err)
	}
	if err := mgr.WriteWithCommitMessageOnBranch("feature line", "notes.md", "feature commit", "feature/ui"); err != nil {
		t.Fatal(err)
	}

	b, err := NewMemoryBrowser(mgr)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}}
	_, _ = b.HandleKeyPress(msg)
	if !b.showHistory {
		t.Fatal("expected history mode enabled")
	}

	filterBefore := b.historyBranch
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}}
	_, _ = b.HandleKeyPress(msg)
	if b.historyBranch == filterBefore {
		t.Fatal("expected history branch filter to cycle")
	}
}

func TestMemoryBrowser_BranchModeToggle(t *testing.T) {
	dir := t.TempDir()
	mgr, err := memory.NewManager(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	if err := mgr.Write("root", "notes.md"); err != nil {
		t.Fatal(err)
	}

	b, err := NewMemoryBrowser(mgr)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}}
	_, _ = b.HandleKeyPress(msg)
	if !b.branchMode {
		t.Fatal("expected branch mode to be enabled")
	}
	if got := b.renderBranches(); got == "" {
		t.Fatal("expected branch view output")
	}
}
