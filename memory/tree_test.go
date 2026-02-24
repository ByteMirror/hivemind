package memory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTree_EmptyDir(t *testing.T) {
	mgr := newTestManager(t)
	entries, err := mgr.Tree()
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestTree_SystemDetection(t *testing.T) {
	mgr := newTestManager(t)

	// Create system/ and root files.
	require.NoError(t, os.MkdirAll(filepath.Join(mgr.dir, "system"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(mgr.dir, "system", "global.md"), []byte("# Global\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(mgr.dir, "notes.md"), []byte("# Notes\n"), 0600))

	entries, err := mgr.Tree()
	require.NoError(t, err)
	require.Len(t, entries, 2)

	entryMap := make(map[string]TreeEntry)
	for _, e := range entries {
		entryMap[e.Path] = e
	}

	sysEntry, ok := entryMap[filepath.Join("system", "global.md")]
	require.True(t, ok)
	assert.True(t, sysEntry.IsSystem)

	rootEntry, ok := entryMap["notes.md"]
	require.True(t, ok)
	assert.False(t, rootEntry.IsSystem)
}

func TestTree_SkipsHiddenDirs(t *testing.T) {
	mgr := newTestManager(t)

	// Create .git/ and .index/ dirs with files — should be skipped.
	for _, d := range []string{".git", ".index"} {
		dir := filepath.Join(mgr.dir, d)
		require.NoError(t, os.MkdirAll(dir, 0700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "should-skip.md"), []byte("skip"), 0600))
	}
	require.NoError(t, os.WriteFile(filepath.Join(mgr.dir, "visible.md"), []byte("keep"), 0600))

	entries, err := mgr.Tree()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "visible.md", entries[0].Path)
}

func TestTree_FrontmatterDescription(t *testing.T) {
	mgr := newTestManager(t)

	content := "---\ndescription: Hardware setup\n---\n# Setup\n"
	require.NoError(t, os.WriteFile(filepath.Join(mgr.dir, "setup.md"), []byte(content), 0600))

	entries, err := mgr.Tree()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "Hardware setup", entries[0].Description)
}

func TestFormatTree_Output(t *testing.T) {
	entries := []TreeEntry{
		{Path: "system/global.md", Description: "OS and hardware", SizeBytes: 1024, IsSystem: true},
		{Path: "notes.md", SizeBytes: 512},
	}

	out := FormatTree(entries)
	assert.Contains(t, out, "* system/global.md")
	assert.Contains(t, out, "OS and hardware")
	assert.Contains(t, out, "  notes.md")
}

func TestFormatTree_Empty(t *testing.T) {
	out := FormatTree(nil)
	assert.Equal(t, "(empty)\n", out)
}
