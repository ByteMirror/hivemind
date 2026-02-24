package memory

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitGitRepo(t *testing.T) {
	dir := t.TempDir()
	repo, err := InitGitRepo(dir)
	require.NoError(t, err)
	assert.True(t, repo.IsInitialized())

	// .gitignore should exist and contain .index/
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	require.NoError(t, err)
	assert.Contains(t, string(data), ".index/")
}

func TestInitGitRepo_Idempotent(t *testing.T) {
	dir := t.TempDir()
	repo1, err := InitGitRepo(dir)
	require.NoError(t, err)

	repo2, err := InitGitRepo(dir)
	require.NoError(t, err)
	assert.True(t, repo1.IsInitialized())
	assert.True(t, repo2.IsInitialized())
}

func TestAutoCommit(t *testing.T) {
	dir := t.TempDir()
	repo, err := InitGitRepo(dir)
	require.NoError(t, err)

	// Create a file and commit.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.md"), []byte("hello"), 0600))
	require.NoError(t, repo.AutoCommit("initial commit"))

	// Log should show one entry.
	entries, err := repo.Log("", 10)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "initial commit", entries[0].Message)
}

func TestAutoCommit_NoChanges(t *testing.T) {
	dir := t.TempDir()
	repo, err := InitGitRepo(dir)
	require.NoError(t, err)

	// First commit the .gitignore.
	require.NoError(t, repo.AutoCommit("init"))

	// Second commit with no changes should return ErrNoChanges.
	err = repo.AutoCommit("should fail")
	assert.True(t, errors.Is(err, ErrNoChanges))
}

func TestLog_EmptyRepo(t *testing.T) {
	dir := t.TempDir()
	repo, err := InitGitRepo(dir)
	require.NoError(t, err)

	entries, err := repo.Log("", 10)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestLog_FilterByPath(t *testing.T) {
	dir := t.TempDir()
	repo, err := InitGitRepo(dir)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("aaa"), 0600))
	require.NoError(t, repo.AutoCommit("add a"))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.md"), []byte("bbb"), 0600))
	require.NoError(t, repo.AutoCommit("add b"))

	// Filter to only a.md
	entries, err := repo.Log("a.md", 10)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "add a", entries[0].Message)
}
