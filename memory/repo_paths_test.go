package memory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveRepoStorePaths_MigratesLegacyWhenCanonicalMissing(t *testing.T) {
	baseDir := t.TempDir()
	legacyDir := filepath.Join(baseDir, "repos", "wt-legacy")
	require.NoError(t, os.MkdirAll(legacyDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(legacyDir, "2026-02-26.md"), []byte("legacy note"), 0600))

	res, err := ResolveRepoStorePaths(baseDir, "/tmp/myrepo", "/tmp/wt-legacy")
	require.NoError(t, err)

	assert.Equal(t, "myrepo", res.CanonicalSlug)
	assert.Equal(t, filepath.Join(baseDir, "repos", "myrepo"), res.CanonicalPath)
	assert.Equal(t, "wt-legacy", res.LegacySlug)
	assert.Empty(t, res.LegacyPath)

	_, oldErr := os.Stat(legacyDir)
	assert.True(t, os.IsNotExist(oldErr))
	_, newErr := os.Stat(filepath.Join(baseDir, "repos", "myrepo", "2026-02-26.md"))
	assert.NoError(t, newErr)
}

func TestResolveRepoStorePaths_KeepsBothWhenCanonicalExists(t *testing.T) {
	baseDir := t.TempDir()
	canonicalDir := filepath.Join(baseDir, "repos", "myrepo")
	legacyDir := filepath.Join(baseDir, "repos", "wt-legacy")
	require.NoError(t, os.MkdirAll(canonicalDir, 0700))
	require.NoError(t, os.MkdirAll(legacyDir, 0700))

	res, err := ResolveRepoStorePaths(baseDir, "/tmp/myrepo", "/tmp/wt-legacy")
	require.NoError(t, err)

	assert.Equal(t, "myrepo", res.CanonicalSlug)
	assert.Equal(t, canonicalDir, res.CanonicalPath)
	assert.Equal(t, "wt-legacy", res.LegacySlug)
	assert.Equal(t, legacyDir, res.LegacyPath)
}
