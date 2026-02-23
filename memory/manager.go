package memory

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const snippetMaxChars = 700

// Manager is the primary interface for IDE-wide memory operations.
// It is safe for concurrent use from multiple goroutines (or processes via WAL).
type Manager struct {
	dir      string           // ~/.hivemind/memory/
	db       *sql.DB
	provider EmbeddingProvider // nil == FTS-only
	reranker Reranker          // optional; nil == no reranking
	mu       sync.RWMutex
}

// NewManager opens (or creates) a MemoryManager rooted at dir.
// provider may be nil for keyword-only search.
func NewManager(dir string, provider EmbeddingProvider) (*Manager, error) {
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
	return &Manager{dir: dir, db: db, provider: provider}, nil
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

// Write appends content to a named memory file and triggers re-indexing.
// If file is empty, the default is today's date (YYYY-MM-DD.md).
func (m *Manager) Write(content, file string) error {
	if file == "" {
		file = time.Now().Format("2006-01-02") + ".md"
	}
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

	return m.Sync(file)
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

// List returns metadata for all memory files.
func (m *Manager) List() ([]FileInfo, error) {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return nil, err
	}
	var files []FileInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		relPath := entry.Name()
		chunkCount := m.countChunks(relPath)
		files = append(files, FileInfo{
			Path:       relPath,
			SizeBytes:  info.Size(),
			UpdatedAt:  info.ModTime().UnixMilli(),
			ChunkCount: chunkCount,
		})
	}
	return files, nil
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
