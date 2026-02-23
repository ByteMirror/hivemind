package memory

import (
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
