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
