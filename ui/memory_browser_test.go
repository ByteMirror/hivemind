package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ByteMirror/hivemind/memory"
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
