package memory

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const snippetMaxChars = 700

// Manager is the primary interface for IDE-wide memory operations.
// It is safe for concurrent use from multiple goroutines (or processes via WAL).
type Manager struct {
	dir      string // ~/.hivemind/memory/
	db       *sql.DB
	provider EmbeddingProvider // nil == FTS-only
	reranker Reranker          // optional; nil == no reranking
	gitRepo  *GitRepo          // nil if git init failed (non-fatal)
	mu       sync.RWMutex
}

// ManagerOptions configures optional manager behaviors.
type ManagerOptions struct {
	// GitEnabled controls whether the memory store is git-versioned.
	// Defaults to true when NewManager is used.
	GitEnabled bool
}

// NewManager opens (or creates) a MemoryManager rooted at dir.
// provider may be nil for keyword-only search.
func NewManager(dir string, provider EmbeddingProvider) (*Manager, error) {
	return NewManagerWithOptions(dir, provider, ManagerOptions{GitEnabled: true})
}

// NewManagerWithOptions opens (or creates) a MemoryManager rooted at dir with
// explicit options.
func NewManagerWithOptions(dir string, provider EmbeddingProvider, opts ManagerOptions) (*Manager, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("mkdir memory dir: %w", err)
	}
	dbDir := filepath.Join(dir, ".index")
	db, err := openDB(filepath.Join(dbDir, "memory.db"))
	if err != nil {
		return nil, err
	}
	if provider == nil {
		provider = &noopProvider{}
	}

	mgr := &Manager{dir: dir, db: db, provider: provider}

	// Initialize git repo (non-fatal) when enabled.
	if opts.GitEnabled {
		if repo, gitErr := InitGitRepo(dir); gitErr == nil {
			mgr.gitRepo = repo
		}
	}

	// Bootstrap system/ directory.
	sysDir := filepath.Join(dir, "system")
	_ = os.MkdirAll(sysDir, 0700)

	// Migrate global.md → system/global.md if it exists at root but not in system/.
	rootGlobal := filepath.Join(dir, "global.md")
	sysGlobal := filepath.Join(sysDir, "global.md")
	if _, err := os.Stat(rootGlobal); err == nil {
		if _, err := os.Stat(sysGlobal); errors.Is(err, os.ErrNotExist) {
			if renameErr := os.Rename(rootGlobal, sysGlobal); renameErr == nil {
				// Update search index: remove old path, index new path.
				_ = mgr.Sync("global.md")
				_ = mgr.Sync(filepath.Join("system", "global.md"))
			}
		}
	}

	return mgr, nil
}

// Close releases resources held by the Manager.
func (m *Manager) Close() {
	m.db.Close()
}

// SetReranker configures an optional post-FTS5 reranker.
// Safe to call before the Manager is used for searches.
func (m *Manager) SetReranker(r Reranker) {
	m.reranker = r
}

// Dir returns the root directory of the memory store.
func (m *Manager) Dir() string { return m.dir }

// GitEnabled reports whether git versioning is active for this manager.
func (m *Manager) GitEnabled() bool { return m.gitRepo != nil }

// Write appends content to a named memory file and triggers re-indexing.
// If file is empty, the default is today's date (YYYY-MM-DD.md).
func (m *Manager) Write(content, file string) error {
	return m.WriteWithCommitMessageOnBranch(content, file, "", "")
}

// WriteWithCommitMessage appends content to a named memory file, re-indexes,
// and commits with the provided commit message when git is enabled.
// If commitMsg is empty, a default message is generated.
func (m *Manager) WriteWithCommitMessage(content, file, commitMsg string) error {
	return m.WriteWithCommitMessageOnBranch(content, file, commitMsg, "")
}

// WriteWithCommitMessageOnBranch appends content to a named memory file and commits
// to the selected branch when provided. When branch is empty, it writes to the
// repository default branch.
func (m *Manager) WriteWithCommitMessageOnBranch(content, file, commitMsg, branch string) error {
	if file == "" {
		file = time.Now().Format("2006-01-02") + ".md"
	}
	if commitMsg == "" {
		commitMsg = "memory: append to " + file
	}
	return m.withBranchMutation(branch, commitMsg, []string{file}, func() error {
		absPath := filepath.Join(m.dir, file)
		if err := os.MkdirAll(filepath.Dir(absPath), 0700); err != nil {
			return fmt.Errorf("mkdir: %w", err)
		}

		// Append a blank line separator if file already has content.
		f, err := os.OpenFile(absPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("open memory file: %w", err)
		}
		defer f.Close()
		stat, _ := f.Stat()
		if stat.Size() > 0 {
			if _, err := f.WriteString("\n\n"); err != nil {
				return err
			}
		}
		if _, err := f.WriteString(strings.TrimSpace(content) + "\n"); err != nil {
			return fmt.Errorf("write memory: %w", err)
		}
		return nil
	})
}

// Get reads lines from a memory file. from=0 means start; lines=0 means all.
func (m *Manager) Get(relPath string, from, lines int) (string, error) {
	absPath := filepath.Join(m.dir, relPath)
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("read memory file: %w", err)
	}
	allLines := strings.Split(string(data), "\n")
	if from > 0 && from <= len(allLines) {
		allLines = allLines[from-1:]
	}
	if lines > 0 && lines < len(allLines) {
		allLines = allLines[:lines]
	}
	return strings.Join(allLines, "\n"), nil
}

// GetAtRef reads lines from a memory file at a specific git ref.
// from=0 means start; lines=0 means all.
func (m *Manager) GetAtRef(relPath string, from, lines int, ref string) (string, error) {
	if strings.TrimSpace(ref) == "" {
		return m.Get(relPath, from, lines)
	}
	if m.gitRepo == nil {
		return "", fmt.Errorf("git versioning is disabled for memory")
	}
	data, err := m.gitRepo.ReadFileAtRef(ref, relPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%s: %w", relPath, ErrFileNotFound)
		}
		return "", err
	}
	allLines := strings.Split(data, "\n")
	if from > 0 && from <= len(allLines) {
		allLines = allLines[from-1:]
	}
	if lines > 0 && lines < len(allLines) {
		allLines = allLines[:lines]
	}
	return strings.Join(allLines, "\n"), nil
}

// List returns metadata for all memory files (recursive).
func (m *Manager) List() ([]FileInfo, error) {
	var files []FileInfo
	err := filepath.WalkDir(m.dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if name == ".git" || name == ".index" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".md") {
			return nil
		}
		rel, relErr := filepath.Rel(m.dir, path)
		if relErr != nil {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		files = append(files, FileInfo{
			Path:       rel,
			SizeBytes:  info.Size(),
			UpdatedAt:  info.ModTime().UnixMilli(),
			ChunkCount: m.countChunks(rel),
		})
		return nil
	})
	return files, err
}

func (m *Manager) countChunks(relPath string) int {
	var count int
	row := m.db.QueryRow(
		`SELECT COUNT(*) FROM chunks c JOIN files f ON c.file_id=f.id WHERE f.path=?`,
		relPath,
	)
	_ = row.Scan(&count)
	return count
}

// Sync re-indexes a specific file: chunks it, updates FTS and vectors.
func (m *Manager) Sync(relPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.syncFile(relPath)
}

func (m *Manager) syncFile(relPath string) error {
	absPath := filepath.Join(m.dir, relPath)
	stat, err := os.Stat(absPath)
	if err != nil {
		return m.deleteFileRecord(relPath)
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return err
	}
	content := string(data)
	hash := hashText(content)
	mtime := stat.ModTime().UnixMilli()

	// Check if already indexed with same hash.
	var storedHash string
	row := m.db.QueryRow("SELECT hash FROM files WHERE path=?", relPath)
	if scanErr := row.Scan(&storedHash); scanErr == nil && storedHash == hash {
		return nil // unchanged
	}

	// Before deleting, clean up FTS entries for this file's chunks.
	// FTS5 content tables require manual sync when the underlying rows are deleted.
	cleanupRows, queryErr := m.db.Query(
		"SELECT c.id, c.text FROM chunks c JOIN files f ON c.file_id=f.id WHERE f.path=?",
		relPath,
	)
	if queryErr == nil {
		for cleanupRows.Next() {
			var id int64
			var text string
			if scanErr := cleanupRows.Scan(&id, &text); scanErr == nil {
				_, _ = m.db.Exec(
					"INSERT INTO chunks_fts(chunks_fts, rowid, text) VALUES('delete', ?, ?)",
					id, text,
				)
			}
		}
		cleanupRows.Close()
	}

	// Delete old file record (cascade deletes chunks and chunks_vec rows).
	_, _ = m.db.Exec("DELETE FROM files WHERE path=?", relPath)

	// Insert file record.
	res, err := m.db.Exec(
		"INSERT INTO files (path, mtime, hash) VALUES (?, ?, ?)",
		relPath, mtime, hash,
	)
	if err != nil {
		return fmt.Errorf("insert file: %w", err)
	}
	fileID, _ := res.LastInsertId()

	chunks := chunkMarkdown(content)
	for _, chunk := range chunks {
		res, err := m.db.Exec(
			"INSERT INTO chunks (file_id, start_line, end_line, text) VALUES (?, ?, ?, ?)",
			fileID, chunk.StartLine, chunk.EndLine, chunk.Text,
		)
		if err != nil {
			return fmt.Errorf("insert chunk: %w", err)
		}
		chunkID, _ := res.LastInsertId()

		// Insert into FTS.
		if _, err := m.db.Exec(
			"INSERT INTO chunks_fts(rowid, text) VALUES (?, ?)",
			chunkID, chunk.Text,
		); err != nil {
			return fmt.Errorf("insert fts: %w", err)
		}

		// Embed and insert vector if provider is available.
		if m.provider.Dims() > 0 {
			if err := m.embedAndStore(chunkID, chunk.Text, chunk.Hash); err != nil {
				// Non-fatal: fall back to FTS-only for this chunk.
				continue
			}
		}
	}
	return nil
}

func (m *Manager) embedAndStore(chunkID int64, text, textHash string) error {
	// Check embedding cache first.
	var cachedBlob []byte
	row := m.db.QueryRow("SELECT embedding FROM embedding_cache WHERE text_hash=?", textHash)
	if err := row.Scan(&cachedBlob); err == nil {
		_, err = m.db.Exec("INSERT OR REPLACE INTO chunks_vec (chunk_id, embedding) VALUES (?, ?)", chunkID, cachedBlob)
		return err
	}

	vecs, err := m.provider.Embed([]string{text})
	if err != nil || len(vecs) == 0 {
		return err
	}
	blob := serializeVec(vecs[0])

	// Cache the embedding.
	_, _ = m.db.Exec(
		"INSERT OR REPLACE INTO embedding_cache (text_hash, embedding, provider, model, created) VALUES (?, ?, ?, ?, ?)",
		textHash, blob, m.provider.Name(), "", time.Now().UnixMilli(),
	)

	_, err = m.db.Exec("INSERT OR REPLACE INTO chunks_vec (chunk_id, embedding) VALUES (?, ?)", chunkID, blob)
	return err
}

func (m *Manager) deleteFileRecord(relPath string) error {
	_, err := m.db.Exec("DELETE FROM files WHERE path=?", relPath)
	return err
}

// Read returns the body of a memory file with frontmatter stripped.
func (m *Manager) Read(relPath string) (string, error) {
	abs, err := m.absPath(relPath)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%s: %w", relPath, ErrFileNotFound)
		}
		return "", err
	}
	_, body := ParseFrontmatter(string(data))
	return body, nil
}

// ReadAtRef returns the body of a memory file with frontmatter stripped at a specific ref.
func (m *Manager) ReadAtRef(relPath string, ref string) (string, error) {
	if strings.TrimSpace(ref) == "" {
		return m.Read(relPath)
	}
	if m.gitRepo == nil {
		return "", fmt.Errorf("git versioning is disabled for memory")
	}
	data, err := m.gitRepo.ReadFileAtRef(ref, relPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%s: %w", relPath, ErrFileNotFound)
		}
		return "", err
	}
	_, body := ParseFrontmatter(data)
	return body, nil
}

// WriteFile creates or overwrites a memory file, re-indexes, and auto-commits.
func (m *Manager) WriteFile(relPath, content, commitMsg string) error {
	return m.WriteFileOnBranch(relPath, content, commitMsg, "")
}

// WriteFileOnBranch creates or overwrites a memory file and commits it to the selected branch.
func (m *Manager) WriteFileOnBranch(relPath, content, commitMsg, branch string) error {
	if commitMsg == "" {
		commitMsg = "memory: write " + relPath
	}
	return m.withBranchMutation(branch, commitMsg, []string{relPath}, func() error {
		abs, err := m.absPath(relPath)
		if err != nil {
			return err
		}

		// Check read-only flag on existing file.
		if fm, fmErr := m.ReadFileFrontmatter(relPath); fmErr == nil && fm.ReadOnly {
			return fmt.Errorf("%s: %w", relPath, ErrReadOnly)
		}

		if err := os.MkdirAll(filepath.Dir(abs), 0700); err != nil {
			return fmt.Errorf("mkdir: %w", err)
		}
		if err := os.WriteFile(abs, []byte(content), 0600); err != nil {
			return fmt.Errorf("write: %w", err)
		}
		return nil
	})
}

// Append adds content to an existing memory file, re-indexes, and auto-commits.
func (m *Manager) Append(relPath, content string) error {
	return m.AppendOnBranch(relPath, content, "")
}

// AppendOnBranch adds content to an existing memory file and commits it to the selected branch.
func (m *Manager) AppendOnBranch(relPath, content, branch string) error {
	return m.withBranchMutation(branch, "memory: append to "+relPath, []string{relPath}, func() error {
		abs, err := m.absPath(relPath)
		if err != nil {
			return err
		}

		if fm, fmErr := m.ReadFileFrontmatter(relPath); fmErr == nil && fm.ReadOnly {
			return fmt.Errorf("%s: %w", relPath, ErrReadOnly)
		}

		if err := os.MkdirAll(filepath.Dir(abs), 0700); err != nil {
			return fmt.Errorf("mkdir: %w", err)
		}

		f, err := os.OpenFile(abs, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("open: %w", err)
		}
		defer f.Close()

		stat, _ := f.Stat()
		if stat.Size() > 0 {
			if _, err := f.WriteString("\n\n"); err != nil {
				return err
			}
		}
		if _, err := f.WriteString(strings.TrimSpace(content) + "\n"); err != nil {
			return err
		}
		return nil
	})
}

// Move renames a memory file, re-indexes both paths, and auto-commits.
func (m *Manager) Move(from, to string) error {
	return m.MoveOnBranch(from, to, "")
}

// MoveOnBranch renames/moves a memory file on the selected branch.
func (m *Manager) MoveOnBranch(from, to, branch string) error {
	return m.withBranchMutation(branch, fmt.Sprintf("memory: move %s → %s", from, to), []string{from, to}, func() error {
		absFrom, err := m.absPath(from)
		if err != nil {
			return err
		}
		absTo, err := m.absPath(to)
		if err != nil {
			return err
		}
		if _, err := os.Stat(absFrom); errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s: %w", from, ErrFileNotFound)
		}

		if err := os.MkdirAll(filepath.Dir(absTo), 0700); err != nil {
			return fmt.Errorf("mkdir: %w", err)
		}
		if err := os.Rename(absFrom, absTo); err != nil {
			return fmt.Errorf("rename: %w", err)
		}
		return nil
	})
}

// Delete removes a memory file, its index, and auto-commits.
func (m *Manager) Delete(relPath string) error {
	return m.DeleteOnBranch(relPath, "")
}

// DeleteOnBranch removes a memory file on the selected branch.
func (m *Manager) DeleteOnBranch(relPath, branch string) error {
	return m.withBranchMutation(branch, "memory: delete "+relPath, []string{relPath}, func() error {
		abs, err := m.absPath(relPath)
		if err != nil {
			return err
		}
		if _, err := os.Stat(abs); errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s: %w", relPath, ErrFileNotFound)
		}
		if err := os.Remove(abs); err != nil {
			return fmt.Errorf("remove: %w", err)
		}
		return nil
	})
}

// Pin moves a file into the system/ directory (always-in-context).
func (m *Manager) Pin(relPath string) error {
	return m.PinOnBranch(relPath, "")
}

// PinOnBranch moves a file into system/ on the selected branch.
func (m *Manager) PinOnBranch(relPath, branch string) error {
	if strings.HasPrefix(filepath.Clean(relPath), "system/") || strings.HasPrefix(filepath.Clean(relPath), "system\\") {
		return fmt.Errorf("%s: %w", relPath, ErrAlreadyPinned)
	}
	dest := filepath.Join("system", filepath.Base(relPath))
	return m.MoveOnBranch(relPath, dest, branch)
}

// Unpin moves a file out of the system/ directory back to root.
func (m *Manager) Unpin(relPath string) error {
	return m.UnpinOnBranch(relPath, "")
}

// UnpinOnBranch moves a file out of system/ on the selected branch.
func (m *Manager) UnpinOnBranch(relPath, branch string) error {
	clean := filepath.Clean(relPath)
	if !strings.HasPrefix(clean, "system/") && !strings.HasPrefix(clean, "system\\") {
		return fmt.Errorf("%s: %w", relPath, ErrNotPinned)
	}
	dest := filepath.Base(relPath)
	return m.MoveOnBranch(relPath, dest, branch)
}

// History returns git log entries for a file, or all files if relPath is empty.
// Returns nil if git is not available.
func (m *Manager) History(relPath string, count int) ([]GitLogEntry, error) {
	return m.HistoryWithBranch(relPath, count, "")
}

// HistoryWithBranch returns git log entries for a file on a specific branch/ref.
func (m *Manager) HistoryWithBranch(relPath string, count int, branch string) ([]GitLogEntry, error) {
	if m.gitRepo == nil {
		return nil, nil
	}
	return m.gitRepo.LogWithBranch(relPath, count, branch)
}

// GitBranches returns the current branch and all branch names for this memory
// git repo. When git versioning is disabled it returns zero values.
func (m *Manager) GitBranches() (current string, branches []string, err error) {
	if m.gitRepo == nil {
		return "", nil, nil
	}
	current, err = m.gitRepo.CurrentBranch()
	if err != nil {
		return "", nil, err
	}
	branches, err = m.gitRepo.Branches()
	if err != nil {
		return "", nil, err
	}
	return current, branches, nil
}

// GitBranchInfo returns current/default branch and all branches for this memory repo.
func (m *Manager) GitBranchInfo() (GitBranchInfo, error) {
	if m.gitRepo == nil {
		return GitBranchInfo{}, nil
	}
	return m.gitRepo.BranchInfo()
}

// CreateBranch creates a memory git branch from an optional source ref.
func (m *Manager) CreateBranch(name, fromRef string) error {
	if m.gitRepo == nil {
		return fmt.Errorf("git versioning is disabled for memory")
	}
	return m.gitRepo.CreateBranch(name, fromRef)
}

// DeleteBranch deletes a memory git branch.
func (m *Manager) DeleteBranch(name string, force bool) error {
	if m.gitRepo == nil {
		return fmt.Errorf("git versioning is disabled for memory")
	}
	return m.gitRepo.DeleteBranch(name, force)
}

// MergeBranch merges source into target branch and re-syncs changed markdown
// files when the target is the default branch.
func (m *Manager) MergeBranch(source, target, strategy string) error {
	if m.gitRepo == nil {
		return fmt.Errorf("git versioning is disabled for memory")
	}
	targetBranch := strings.TrimSpace(target)
	if targetBranch == "" {
		def, err := m.gitRepo.DefaultBranch()
		if err != nil {
			return err
		}
		targetBranch = def
	}

	changedFiles, err := m.gitRepo.MergeBranch(source, targetBranch, strategy)
	if err != nil {
		return err
	}

	def, err := m.gitRepo.DefaultBranch()
	if err != nil {
		return err
	}
	if targetBranch != def {
		return nil
	}

	for _, p := range changedFiles {
		if !strings.HasSuffix(strings.ToLower(p), ".md") {
			continue
		}
		if err := m.Sync(filepath.ToSlash(p)); err != nil {
			return err
		}
	}
	return nil
}

// DiffRefs returns the diff between two refs. Optional path limits output.
func (m *Manager) DiffRefs(baseRef, headRef, relPath string) (string, error) {
	if m.gitRepo == nil {
		return "", fmt.Errorf("git versioning is disabled for memory")
	}
	return m.gitRepo.DiffRefs(baseRef, headRef, relPath)
}

// SystemFiles reads all files from system/ up to maxChars total.
// Returns a map of relative path → body content (frontmatter stripped).
func (m *Manager) SystemFiles(maxChars int) (map[string]string, error) {
	sysDir := filepath.Join(m.dir, "system")
	if _, err := os.Stat(sysDir); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	result := make(map[string]string)
	totalChars := 0

	err := filepath.WalkDir(sysDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		rel, relErr := filepath.Rel(m.dir, path)
		if relErr != nil {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		_, body := ParseFrontmatter(string(data))
		if maxChars > 0 && totalChars+len(body) > maxChars {
			return nil // skip files that would exceed budget
		}
		result[rel] = body
		totalChars += len(body)
		return nil
	})
	return result, err
}

// autoCommit is a nil-safe helper that commits all changes in the memory git repo.
func (m *Manager) autoCommit(msg string) {
	if m.gitRepo == nil {
		return
	}
	err := m.gitRepo.AutoCommit(msg)
	if err != nil && !errors.Is(err, ErrNoChanges) {
		// Non-fatal: log would be ideal but we don't have a logger here.
		_ = err
	}
}

// ListAtRef returns metadata for all markdown files at a specific git ref.
func (m *Manager) ListAtRef(ref string) ([]FileInfo, error) {
	if strings.TrimSpace(ref) == "" {
		return m.List()
	}
	if m.gitRepo == nil {
		return nil, fmt.Errorf("git versioning is disabled for memory")
	}
	filesByPath, err := m.gitRepo.ListMarkdownFilesAtRef(ref)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(filesByPath))
	for path := range filesByPath {
		paths = append(paths, filepath.ToSlash(path))
	}
	sort.Strings(paths)

	out := make([]FileInfo, 0, len(paths))
	for _, p := range paths {
		content := filesByPath[p]
		out = append(out, FileInfo{
			Path:       p,
			SizeBytes:  int64(len(content)),
			UpdatedAt:  0,
			ChunkCount: 0,
		})
	}
	return out, nil
}

// TreeAtRef returns the markdown tree with descriptions at a specific git ref.
func (m *Manager) TreeAtRef(ref string) ([]TreeEntry, error) {
	if strings.TrimSpace(ref) == "" {
		return m.Tree()
	}
	if m.gitRepo == nil {
		return nil, fmt.Errorf("git versioning is disabled for memory")
	}
	filesByPath, err := m.gitRepo.ListMarkdownFilesAtRef(ref)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(filesByPath))
	for path := range filesByPath {
		paths = append(paths, filepath.ToSlash(path))
	}
	sort.Strings(paths)

	out := make([]TreeEntry, 0, len(paths))
	for _, p := range paths {
		content := filesByPath[p]
		fm, _ := ParseFrontmatter(content)
		out = append(out, TreeEntry{
			Path:        p,
			Description: fm.Description,
			SizeBytes:   int64(len(content)),
			IsSystem:    strings.HasPrefix(p, "system/"),
		})
	}
	return out, nil
}

func (m *Manager) withBranchMutation(branch, commitMsg string, syncPaths []string, mutate func() error) error {
	if strings.TrimSpace(commitMsg) == "" {
		return fmt.Errorf("commit message is required")
	}
	if m.gitRepo == nil {
		if strings.TrimSpace(branch) != "" {
			return fmt.Errorf("git versioning is disabled for memory")
		}
		if err := mutate(); err != nil {
			return err
		}
		for _, p := range syncPaths {
			if err := m.Sync(filepath.ToSlash(p)); err != nil {
				return err
			}
		}
		return nil
	}

	targetBranch := strings.TrimSpace(branch)
	defaultBranch, err := m.gitRepo.DefaultBranch()
	if err != nil {
		return err
	}
	if targetBranch == "" {
		targetBranch = defaultBranch
	}
	shouldSync := targetBranch == defaultBranch

	return m.gitRepo.withRepoLock("manager_branch_mutation", func() error {
		currentBranch, err := m.gitRepo.CurrentBranch()
		if err != nil {
			return err
		}
		if currentBranch != targetBranch {
			if _, err := m.gitRepo.gitExec("checkout", "--quiet", targetBranch); err != nil {
				return fmt.Errorf("checkout branch %q: %w", targetBranch, err)
			}
		}
		defer func() {
			if currentBranch != targetBranch {
				_, _ = m.gitRepo.gitExec("checkout", "--quiet", currentBranch)
			}
		}()

		if err := mutate(); err != nil {
			return err
		}
		if shouldSync {
			for _, p := range syncPaths {
				if err := m.Sync(filepath.ToSlash(p)); err != nil {
					return err
				}
			}
		}
		err = m.gitRepo.autoCommitUnlocked(commitMsg)
		if err != nil && !errors.Is(err, ErrNoChanges) {
			return err
		}
		return nil
	})
}
