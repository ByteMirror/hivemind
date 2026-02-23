package memory

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setFileMtime sets the mtime of a file in the Manager's directory and
// re-syncs it so the DB picks up the new mtime.
func setFileMtime(t *testing.T, mgr *Manager, name string, mtime time.Time) {
	t.Helper()
	absPath := filepath.Join(mgr.dir, name)
	require.NoError(t, os.Chtimes(absPath, mtime, mtime))
	// Force re-index: delete the file hash from the DB so syncFile re-inserts.
	_, err := mgr.db.Exec("DELETE FROM files WHERE path=?", name)
	require.NoError(t, err)
	require.NoError(t, mgr.Sync(name))
}

// TestApplyTemporalDecay_DatedFileDecays verifies that a file matching the
// YYYY-MM-DD.md pattern has its score reduced by temporal decay.
func TestApplyTemporalDecay_DatedFileDecays(t *testing.T) {
	mgr := newTestManager(t)

	// Write a dated file, then backdate its mtime to 5 years ago so decay is measurable.
	require.NoError(t, mgr.Write("Old journal entry.", "2020-01-01.md"))
	setFileMtime(t, mgr, "2020-01-01.md", time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))

	original := float32(1.0)
	results := []scoredResult{
		{SearchResult: SearchResult{Path: "2020-01-01.md", Score: original}},
	}

	decayed := applyTemporalDecay(results, mgr)
	require.Len(t, decayed, 1)
	assert.Less(t, decayed[0].Score, original,
		"dated file score should be reduced by temporal decay")
}

// TestApplyTemporalDecay_DatedFileWithDirPrefix verifies that a dated file
// stored under a subdirectory (e.g. "repos/project/2026-01-01.md") is still
// subject to temporal decay. This ensures filepath.Base correctly strips the
// directory component before the YYYY-MM-DD.md regex runs.
func TestApplyTemporalDecay_DatedFileWithDirPrefix(t *testing.T) {
	mgr := newTestManager(t)

	const relPath = "repos/project/2026-01-01.md"
	require.NoError(t, mgr.Write("Old project note.", relPath))
	setFileMtime(t, mgr, relPath, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	original := float32(1.0)
	results := []scoredResult{
		{SearchResult: SearchResult{Path: relPath, Score: original}},
	}

	decayed := applyTemporalDecay(results, mgr)
	require.Len(t, decayed, 1)
	assert.Less(t, decayed[0].Score, original,
		"dated file under a directory prefix should be reduced by temporal decay")
}

// TestApplyTemporalDecay_EvergreenFileExempt verifies that a non-dated
// evergreen file (e.g. global.md, MEMORY.md) is NOT affected by decay,
// even when the file is very old.
func TestApplyTemporalDecay_EvergreenFileExempt(t *testing.T) {
	mgr := newTestManager(t)

	// Write an evergreen file and backdate it so that decay *would* matter
	// if applied — ensuring we're truly testing the exemption, not just a
	// near-zero age coincidence.
	require.NoError(t, mgr.Write("Hardware: MacBook Pro M3.", "global.md"))
	setFileMtime(t, mgr, "global.md", time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))

	original := float32(1.0)
	results := []scoredResult{
		{SearchResult: SearchResult{Path: "global.md", Score: original}},
	}

	after := applyTemporalDecay(results, mgr)
	require.Len(t, after, 1)
	assert.Equal(t, original, after[0].Score,
		"evergreen file score must not be changed by temporal decay")
}

// TestApplyTemporalDecay_EvergreenFilesExempt_Multiple checks several
// non-dated filename patterns that should all be exempt.
func TestApplyTemporalDecay_EvergreenFilesExempt_Multiple(t *testing.T) {
	mgr := newTestManager(t)

	evergreenFiles := []string{
		"global.md",
		"MEMORY.md",
		"hivemind-project.md",
		"preferences.md",
		"setup.md",
	}

	oldTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, name := range evergreenFiles {
		require.NoError(t, mgr.Write("Stable fact.", name))
		setFileMtime(t, mgr, name, oldTime)
	}

	original := float32(0.75)
	var results []scoredResult
	for _, name := range evergreenFiles {
		results = append(results, scoredResult{
			SearchResult: SearchResult{Path: name, Score: original},
		})
	}

	after := applyTemporalDecay(results, mgr)
	require.Len(t, after, len(evergreenFiles))
	for _, r := range after {
		assert.Equal(t, original, r.Score,
			"evergreen file %q must not decay", r.Path)
	}
}

// TestDatedFileRe verifies the regex correctly classifies filenames.
func TestDatedFileRe(t *testing.T) {
	matched := []string{
		"2020-01-01.md",
		"2026-02-23.md",
		"2025-08-14.md",
		"1999-12-31.md",
	}
	notMatched := []string{
		"global.md",
		"MEMORY.md",
		"hivemind-project.md",
		"preferences.md",
		"2020-01-01.txt",
		"2020-1-1.md",
		"20200101.md",
		"notes-2020-01-01.md",
		"2020-01-01.md.bak",
	}

	for _, name := range matched {
		assert.True(t, datedFileRe.MatchString(name), "expected %q to match datedFileRe", name)
	}
	for _, name := range notMatched {
		assert.False(t, datedFileRe.MatchString(name), "expected %q NOT to match datedFileRe", name)
	}
}
