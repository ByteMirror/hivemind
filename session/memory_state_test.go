package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ByteMirror/hivemind/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRepoManagersForPaths_UsesRepoSlugNotWorktreeSlug(t *testing.T) {
	globalMgr, _ := newTestMemoryManager(t)
	SetMemoryManager(globalMgr, 5, 0)
	SetMemoryFactory(func(dir string) (*memory.Manager, error) {
		return memory.NewManagerWithOptions(dir, nil, memory.ManagerOptions{GitEnabled: false})
	})
	defer func() {
		SetMemoryFactory(nil)
		SetMemoryManager(nil, 0, 0)
		CloseAllRepoManagers()
	}()

	repoMgr, legacyMgr, slug, err := GetRepoManagersForPaths("/repos/myrepo", "/tmp/worktrees/wt-session")
	require.NoError(t, err)
	require.NotNil(t, repoMgr)
	assert.Nil(t, legacyMgr)
	assert.Equal(t, "myrepo", slug)

	_, statErr := os.Stat(filepath.Join(globalMgr.Dir(), "repos", "myrepo"))
	assert.NoError(t, statErr)
}

func TestGetRepoManagersForPaths_MigratesLegacyWhenCanonicalMissing(t *testing.T) {
	globalMgr, dir := newTestMemoryManager(t)
	SetMemoryManager(globalMgr, 5, 0)
	defer func() {
		SetMemoryFactory(nil)
		SetMemoryManager(nil, 0, 0)
		CloseAllRepoManagers()
	}()

	legacyDir := filepath.Join(dir, "repos", "wt-slug")
	require.NoError(t, os.MkdirAll(legacyDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(legacyDir, "2026-02-26.md"), []byte("legacy"), 0600))

	repoMgr, legacyMgr, slug, err := GetRepoManagersForPaths("/repos/canonical", "/tmp/worktrees/wt-slug")
	require.NoError(t, err)
	require.NotNil(t, repoMgr)
	assert.Nil(t, legacyMgr)
	assert.Equal(t, "canonical", slug)

	_, oldErr := os.Stat(legacyDir)
	assert.True(t, os.IsNotExist(oldErr))
	_, newErr := repoMgr.Get("2026-02-26.md", 0, 0)
	assert.NoError(t, newErr)
}

func TestGetRepoManagersForPaths_ReturnsLegacyWhenBothDirsExist(t *testing.T) {
	globalMgr, dir := newTestMemoryManager(t)
	SetMemoryManager(globalMgr, 5, 0)
	defer func() {
		SetMemoryFactory(nil)
		SetMemoryManager(nil, 0, 0)
		CloseAllRepoManagers()
	}()

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "repos", "canonical"), 0700))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "repos", "wt-slug"), 0700))

	repoMgr, legacyMgr, slug, err := GetRepoManagersForPaths("/repos/canonical", "/tmp/worktrees/wt-slug")
	require.NoError(t, err)
	require.NotNil(t, repoMgr)
	require.NotNil(t, legacyMgr)
	assert.Equal(t, "canonical", slug)
}
