package memory

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
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

func TestLog_MultipleCommits_ParsesAllEntriesAndFiles(t *testing.T) {
	dir := t.TempDir()
	repo, err := InitGitRepo(dir)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "one.md"), []byte("one"), 0600))
	require.NoError(t, repo.AutoCommit("commit one"))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "two.md"), []byte("two"), 0600))
	require.NoError(t, repo.AutoCommit("commit two"))

	entries, err := repo.Log("", 10)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(entries), 2)
	assert.Equal(t, "commit two", entries[0].Message)
	assert.NotEmpty(t, entries[0].Files)
	assert.Equal(t, "commit one", entries[1].Message)
	assert.NotEmpty(t, entries[1].Files)
}

func TestLog_IncludesMetadataAndStats(t *testing.T) {
	dir := t.TempDir()
	repo, err := InitGitRepo(dir)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "meta.md"), []byte("one"), 0600))
	require.NoError(t, repo.AutoCommit("meta commit one"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "meta.md"), []byte("one\ntwo\nthree\n"), 0600))
	require.NoError(t, repo.AutoCommit("meta commit two"))

	entries, err := repo.Log("", 5)
	require.NoError(t, err)
	require.NotEmpty(t, entries)
	assert.NotEmpty(t, entries[0].SHA)
	assert.NotEmpty(t, entries[0].Date)
	assert.NotEmpty(t, entries[0].AuthorName)
	assert.NotEmpty(t, entries[0].AuthorEmail)
	assert.GreaterOrEqual(t, entries[0].Additions, 0)
	assert.GreaterOrEqual(t, entries[0].Deletions, 0)
}

func TestBranchLifecycle_MergeAndDiff(t *testing.T) {
	dir := t.TempDir()
	repo, err := InitGitRepo(dir)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("root"), 0600))
	require.NoError(t, repo.AutoCommit("initial"))

	defBranch, err := repo.DefaultBranch()
	require.NoError(t, err)
	require.NotEmpty(t, defBranch)

	require.NoError(t, repo.CreateBranch("feature/memory", ""))
	_, err = repo.gitExec("checkout", "--quiet", "feature/memory")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("root\nfeature\n"), 0600))
	require.NoError(t, repo.AutoCommit("feature change"))
	_, err = repo.gitExec("checkout", "--quiet", defBranch)
	require.NoError(t, err)

	changed, err := repo.MergeBranch("feature/memory", defBranch, "ff-only")
	require.NoError(t, err)
	require.NotEmpty(t, changed)

	logEntries, err := repo.LogWithBranch("", 20, defBranch)
	require.NoError(t, err)
	require.NotEmpty(t, logEntries)
	assert.Equal(t, defBranch, logEntries[0].Branch)

	require.NoError(t, repo.DeleteBranch("feature/memory", false))

	diff, err := repo.DiffRefs(logEntries[len(logEntries)-1].SHA, logEntries[0].SHA, "notes.md")
	require.NoError(t, err)
	assert.NotEmpty(t, diff)
}

func TestAutoCommit_ConcurrentCalls_NoRepoBusyOrCorruption(t *testing.T) {
	dir := t.TempDir()
	repo, err := InitGitRepo(dir)
	require.NoError(t, err)

	var wg sync.WaitGroup
	errCh := make(chan error, 20)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := filepath.Join(dir, "c", "file-"+string(rune('a'+idx))+".md")
			_ = os.MkdirAll(filepath.Dir(name), 0700)
			_ = os.WriteFile(name, []byte("v"), 0600)
			if commitErr := repo.AutoCommit("parallel commit"); commitErr != nil && !errors.Is(commitErr, ErrNoChanges) {
				errCh <- commitErr
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for e := range errCh {
		require.NoError(t, e)
	}

	entries, err := repo.Log("", 50)
	require.NoError(t, err)
	assert.NotEmpty(t, entries)
}
