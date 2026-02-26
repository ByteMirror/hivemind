package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ByteMirror/hivemind/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpsertMemorySection_AppendsWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/CLAUDE.md"
	require.NoError(t, os.WriteFile(path, []byte("# Existing\n\nContent here.\n"), 0600))

	require.NoError(t, upsertMemorySection(path, "<!-- hivemind-memory-start -->\n## New\n<!-- hivemind-memory-end -->\n"))

	data, _ := os.ReadFile(path)
	s := string(data)
	assert.Contains(t, s, "# Existing")
	assert.Contains(t, s, "## New")
}

func TestUpsertMemorySection_ReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/CLAUDE.md"
	initial := "# Top\n<!-- hivemind-memory-start -->\n## Old\n<!-- hivemind-memory-end -->\n# Bottom\n"
	require.NoError(t, os.WriteFile(path, []byte(initial), 0600))

	require.NoError(t, upsertMemorySection(path, "<!-- hivemind-memory-start -->\n## New\n<!-- hivemind-memory-end -->\n"))

	data, _ := os.ReadFile(path)
	s := string(data)
	assert.NotContains(t, s, "## Old")
	assert.Contains(t, s, "## New")
	assert.Contains(t, s, "# Top")
	assert.Contains(t, s, "# Bottom")
}

// newTestMemoryManager creates a real memory.Manager in a temp dir for testing.
func newTestMemoryManager(t *testing.T) (*memory.Manager, string) {
	t.Helper()
	dir := t.TempDir()
	mgr, err := memory.NewManager(dir, nil)
	require.NoError(t, err)
	t.Cleanup(func() { mgr.Close() })
	return mgr, dir
}

// TestInjectMemoryContext_GlobalOnly verifies that InjectMemoryContext with a nil
// repoMgr still produces a valid CLAUDE.md containing the global and repo sections.
// The repo section should show the "no repo memory yet" placeholder.
func TestInjectMemoryContext_GlobalOnly(t *testing.T) {
	globalMgr, _ := newTestMemoryManager(t)

	// Write content using terms that overlap with the global injection query.
	// The query is: "global setup preferences environment hardware OS"
	require.NoError(t, globalMgr.Write("global hardware OS preferences: MacBook Pro M3, macOS setup", "global.md"))

	// Create a fake worktree directory with a minimal CLAUDE.md.
	wtDir := t.TempDir()
	wtPath := filepath.Join(wtDir, "myrepo")
	require.NoError(t, os.MkdirAll(wtPath, 0700))
	claudeMD := filepath.Join(wtPath, "CLAUDE.md")
	require.NoError(t, os.WriteFile(claudeMD, []byte("# Project\n"), 0600))

	err := InjectMemoryContext(wtPath, globalMgr, nil, 5)
	require.NoError(t, err)

	data, err := os.ReadFile(claudeMD)
	require.NoError(t, err)
	s := string(data)

	// Structural checks — these must always be present.
	assert.Contains(t, s, memoryInjectHeader)
	assert.Contains(t, s, memoryInjectFooter)
	assert.Contains(t, s, "### Global context")
	assert.Contains(t, s, "### Repo context (myrepo)")

	// No repo manager was provided, so the repo section placeholder appears.
	assert.Contains(t, s, "*(no repo memory yet)*")

	// The global result snippet should be present (FTS5 matches on overlapping terms).
	assert.Contains(t, s, "MacBook Pro M3")
}

// TestInjectMemoryContext_BothSections verifies that when both a global and repo
// manager are provided, both sections appear in CLAUDE.md with their respective results.
func TestInjectMemoryContext_BothSections(t *testing.T) {
	globalMgr, _ := newTestMemoryManager(t)
	repoMgr, _ := newTestMemoryManager(t)

	// Write content with terms overlapping the global query:
	// "global setup preferences environment hardware OS"
	require.NoError(t, globalMgr.Write("global OS hardware setup: macOS 15, zsh, homebrew environment", "global.md"))

	// Write content with terms overlapping the repo query:
	// "{slug} project architecture decisions"
	// The slug here is "hivemind" (the basename of the worktree path).
	require.NoError(t, repoMgr.Write("hivemind project architecture decisions: Go 1.23, Bubble Tea TUI, no ORM", "2026-02-23.md"))

	// Create a fake worktree directory.
	wtDir := t.TempDir()
	wtPath := filepath.Join(wtDir, "hivemind")
	require.NoError(t, os.MkdirAll(wtPath, 0700))
	claudeMD := filepath.Join(wtPath, "CLAUDE.md")
	require.NoError(t, os.WriteFile(claudeMD, []byte("# Hivemind\n"), 0600))

	err := InjectMemoryContext(wtPath, globalMgr, repoMgr, 5)
	require.NoError(t, err)

	data, err := os.ReadFile(claudeMD)
	require.NoError(t, err)
	s := string(data)

	// Structural checks.
	assert.Contains(t, s, memoryInjectHeader)
	assert.Contains(t, s, memoryInjectFooter)
	assert.Contains(t, s, "### Global context")
	assert.Contains(t, s, "### Repo context (hivemind)")

	// Both result snippets should be present.
	assert.Contains(t, s, "macOS 15")
	assert.Contains(t, s, "Bubble Tea")

	// Neither placeholder should appear since both managers returned results.
	assert.NotContains(t, s, "*(no global memory yet)*")
	assert.NotContains(t, s, "*(no repo memory yet)*")
}

func TestInjectMemoryContextForRepo_MergesCanonicalAndLegacyRepoResults(t *testing.T) {
	globalMgr, _ := newTestMemoryManager(t)
	repoMgr, _ := newTestMemoryManager(t)
	legacyRepoMgr, _ := newTestMemoryManager(t)

	require.NoError(t, globalMgr.Write("global setup hardware: macOS", "global.md"))
	require.NoError(t, repoMgr.Write("hivemind project architecture decisions: canonical repo data", "2026-02-26.md"))
	require.NoError(t, legacyRepoMgr.Write("hivemind project architecture decisions: legacy repo data", "2026-02-25.md"))

	wtDir := t.TempDir()
	wtPath := filepath.Join(wtDir, "worktree-slug")
	require.NoError(t, os.MkdirAll(wtPath, 0700))
	claudeMD := filepath.Join(wtPath, "CLAUDE.md")
	require.NoError(t, os.WriteFile(claudeMD, []byte("# Repo\n"), 0600))

	err := InjectMemoryContextForRepo(wtPath, "hivemind", globalMgr, repoMgr, legacyRepoMgr, 5)
	require.NoError(t, err)

	data, err := os.ReadFile(claudeMD)
	require.NoError(t, err)
	s := string(data)

	assert.Contains(t, s, "canonical repo data")
	assert.Contains(t, s, "legacy repo data")
	assert.Contains(t, s, "### Repo context (hivemind)")
}
