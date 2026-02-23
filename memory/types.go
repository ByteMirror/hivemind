package memory

// SearchResult is one match returned from a memory search.
type SearchResult struct {
	Path      string  // relative to memory dir, e.g. "global.md"
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
