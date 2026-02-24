package memory

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// Sentinel errors for memory operations.
var (
	ErrPathEscape    = errors.New("path escapes memory directory")
	ErrReadOnly      = errors.New("file is marked read-only")
	ErrFileNotFound  = errors.New("memory file not found")
	ErrAlreadyPinned = errors.New("file is already in system/")
	ErrNotPinned     = errors.New("file is not in system/")
)

// validateMemPath rejects paths that escape the memory directory or target
// internal directories (.git/, .index/).
func validateMemPath(relPath string) error {
	if relPath == "" {
		return fmt.Errorf("empty path: %w", ErrPathEscape)
	}
	if filepath.IsAbs(relPath) {
		return fmt.Errorf("absolute path not allowed: %w", ErrPathEscape)
	}
	clean := filepath.Clean(relPath)
	if strings.HasPrefix(clean, "..") {
		return fmt.Errorf("path escapes root: %w", ErrPathEscape)
	}
	if strings.HasPrefix(clean, ".git") || strings.HasPrefix(clean, ".index") {
		return fmt.Errorf("internal path not allowed: %w", ErrPathEscape)
	}
	return nil
}

// absPath validates relPath and returns the absolute path within the memory dir.
func (m *Manager) absPath(relPath string) (string, error) {
	if err := validateMemPath(relPath); err != nil {
		return "", err
	}
	return filepath.Join(m.dir, relPath), nil
}

// SearchResult is one match returned from a memory search.
type SearchResult struct {
	Path      string // relative to memory dir, e.g. "global.md"
	StartLine int
	EndLine   int
	Score     float32 // 0.0–1.0 combined score
	Snippet   string  // up to 700 chars of matched text
}

// FileInfo is metadata about one memory file, returned by List.
type FileInfo struct {
	Path       string // relative to memory dir
	SizeBytes  int64
	UpdatedAt  int64 // Unix ms
	ChunkCount int
}

// SearchOpts configures a Search call.
type SearchOpts struct {
	MaxResults int     // default 10
	MinScore   float32 // default 0.0 (no filter)
}

// EmbeddingProvider abstracts an embedding API.
type EmbeddingProvider interface {
	Embed(texts []string) ([][]float32, error)
	Dims() int
	Name() string
}

// Reranker re-orders search results by semantic relevance to a query.
// It is applied as a post-processing step after FTS5 keyword retrieval,
// without requiring any embedding storage.
type Reranker interface {
	Rerank(query string, results []SearchResult) ([]SearchResult, error)
	Name() string
}
