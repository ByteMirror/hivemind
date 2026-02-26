package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	mgr, err := NewManager(dir, nil) // nil = no embedding provider
	require.NoError(t, err)
	t.Cleanup(func() { mgr.Close() })
	return mgr
}

func TestManager_WriteCreatesFile(t *testing.T) {
	mgr := newTestManager(t)
	err := mgr.Write("The user prefers Go over Python.", "preferences.md")
	require.NoError(t, err)

	files, err := mgr.List()
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "preferences.md", files[0].Path)
	assert.Greater(t, files[0].SizeBytes, int64(0))
}

func TestManager_WriteAppendsToExistingFile(t *testing.T) {
	mgr := newTestManager(t)
	require.NoError(t, mgr.Write("Fact one.", "notes.md"))
	require.NoError(t, mgr.Write("Fact two.", "notes.md"))

	result, err := mgr.Get("notes.md", 0, 0)
	require.NoError(t, err)
	assert.Contains(t, result, "Fact one.")
	assert.Contains(t, result, "Fact two.")
}

func TestManager_GetReturnsLines(t *testing.T) {
	mgr := newTestManager(t)
	content := "line1\nline2\nline3\nline4\nline5"
	require.NoError(t, mgr.Write(content, "test.md"))

	// Get lines 2-3
	result, err := mgr.Get("test.md", 2, 2)
	require.NoError(t, err)
	assert.Contains(t, result, "line2")
	assert.Contains(t, result, "line3")
	assert.NotContains(t, result, "line1")
}

func TestManager_ListReturnsChunkCount(t *testing.T) {
	mgr := newTestManager(t)
	// Write enough content to produce multiple chunks
	content := "# Section A\n\nSome content.\n\n# Section B\n\nMore content."
	require.NoError(t, mgr.Write(content, "multi.md"))

	// Force index sync
	require.NoError(t, mgr.Sync("multi.md"))

	files, err := mgr.List()
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.GreaterOrEqual(t, files[0].ChunkCount, 2)
}

func TestManager_SearchFTSOnly(t *testing.T) {
	mgr := newTestManager(t)
	require.NoError(t, mgr.Write("The user prefers Go over Python.", "prefs.md"))
	require.NoError(t, mgr.Write("Project Hivemind uses Bubble Tea for TUI.", "projects.md"))

	results, err := mgr.Search("Go language preference", SearchOpts{MaxResults: 5})
	require.NoError(t, err)
	// At least one result should mention Go
	found := false
	for _, r := range results {
		if strings.Contains(r.Snippet, "Go") {
			found = true
		}
	}
	assert.True(t, found, "expected to find Go-related memory")
}

func TestManager_SearchReturnsSnippets(t *testing.T) {
	mgr := newTestManager(t)
	require.NoError(t, mgr.Write("# User Setup\n\nMacBook Pro M3, 32GB RAM, macOS Sequoia.", "setup.md"))

	results, err := mgr.Search("user computer hardware", SearchOpts{MaxResults: 5})
	require.NoError(t, err)
	if len(results) > 0 {
		assert.NotEmpty(t, results[0].Snippet)
		assert.LessOrEqual(t, len(results[0].Snippet), snippetMaxChars+10)
	}
}

func TestNewManagerWithOptions_GitDisabled(t *testing.T) {
	dir := t.TempDir()
	mgr, err := NewManagerWithOptions(dir, nil, ManagerOptions{GitEnabled: false})
	require.NoError(t, err)
	t.Cleanup(func() { mgr.Close() })

	require.NoError(t, mgr.Write("fact", "notes.md"))
	assert.False(t, mgr.GitEnabled())

	_, statErr := os.Stat(filepath.Join(dir, ".git"))
	assert.True(t, os.IsNotExist(statErr))

	history, histErr := mgr.History("", 10)
	require.NoError(t, histErr)
	assert.Nil(t, history)
}

func TestManager_WriteWithCommitMessage(t *testing.T) {
	dir := t.TempDir()
	mgr, err := NewManager(dir, nil)
	require.NoError(t, err)
	t.Cleanup(func() { mgr.Close() })

	require.NoError(t, mgr.WriteWithCommitMessage("fact", "notes.md", "custom memory commit"))
	history, err := mgr.History("", 10)
	require.NoError(t, err)
	require.NotEmpty(t, history)
	assert.Equal(t, "custom memory commit", history[0].Message)
}

func TestManager_GitBranches(t *testing.T) {
	dir := t.TempDir()
	mgr, err := NewManager(dir, nil)
	require.NoError(t, err)
	t.Cleanup(func() { mgr.Close() })

	require.NoError(t, mgr.Write("fact", "notes.md"))
	current, branches, err := mgr.GitBranches()
	require.NoError(t, err)
	assert.NotEmpty(t, current)
	assert.NotEmpty(t, branches)
}

func TestManager_BranchWriteAndReadAtRef(t *testing.T) {
	dir := t.TempDir()
	mgr, err := NewManager(dir, nil)
	require.NoError(t, err)
	t.Cleanup(func() { mgr.Close() })

	require.NoError(t, mgr.Write("root note", "notes.md"))
	require.NoError(t, mgr.CreateBranch("feature/memory", ""))
	require.NoError(t, mgr.WriteWithCommitMessageOnBranch("branch note", "notes.md", "branch write", "feature/memory"))

	// Default branch should remain unchanged before merge.
	body, err := mgr.Read("notes.md")
	require.NoError(t, err)
	assert.NotContains(t, body, "branch note")

	branchBody, err := mgr.ReadAtRef("notes.md", "feature/memory")
	require.NoError(t, err)
	assert.Contains(t, branchBody, "branch note")
}

func TestManager_MergeBranch_SyncsDefaultIndex(t *testing.T) {
	dir := t.TempDir()
	mgr, err := NewManager(dir, nil)
	require.NoError(t, err)
	t.Cleanup(func() { mgr.Close() })

	require.NoError(t, mgr.Write("root", "merge.md"))
	require.NoError(t, mgr.CreateBranch("feature/index", ""))
	require.NoError(t, mgr.WriteWithCommitMessageOnBranch("feature line", "merge.md", "feature update", "feature/index"))

	results, err := mgr.Search("feature line", SearchOpts{MaxResults: 5})
	require.NoError(t, err)
	assert.Empty(t, results, "feature branch content should not be indexed before merge")

	require.NoError(t, mgr.MergeBranch("feature/index", "", "ff-only"))

	results, err = mgr.Search("feature line", SearchOpts{MaxResults: 5})
	require.NoError(t, err)
	assert.NotEmpty(t, results, "merged content should be indexed on default branch")
}

func TestManager_ListAndTreeAtRef(t *testing.T) {
	dir := t.TempDir()
	mgr, err := NewManager(dir, nil)
	require.NoError(t, err)
	t.Cleanup(func() { mgr.Close() })

	require.NoError(t, mgr.Write("---\ndescription: Root file\n---\nroot", "root.md"))
	require.NoError(t, mgr.CreateBranch("feature/tree", ""))
	require.NoError(t, mgr.WriteWithCommitMessageOnBranch("---\ndescription: Branch file\n---\nbranch", "branch-only.md", "branch file", "feature/tree"))

	files, err := mgr.ListAtRef("feature/tree")
	require.NoError(t, err)
	found := false
	for _, f := range files {
		if f.Path == "branch-only.md" {
			found = true
			break
		}
	}
	assert.True(t, found)

	tree, err := mgr.TreeAtRef("feature/tree")
	require.NoError(t, err)
	found = false
	for _, e := range tree {
		if e.Path == "branch-only.md" && e.Description == "Branch file" {
			found = true
			break
		}
	}
	assert.True(t, found)
}
