# IDE-Wide Memory System — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a global IDE-wide persistent memory system for all Hivemind agents using SQLite FTS5 + vector embeddings with dual injection via MCP tools and CLAUDE.md startup injection.

**Architecture:** Markdown files stored at `~/.hivemind/memory/` are indexed into a SQLite database (FTS5 for keyword search + BLOB vectors for semantic search). A `MemoryManager` type handles indexing, searching, and writing. MCP tools expose memory to agents at runtime; the TUI also injects top-K relevant memories into each worktree's CLAUDE.md on startup. Multiple processes (TUI + MCP server) share the SQLite file via WAL mode.

**Tech Stack:** `modernc.org/sqlite` (pure Go, no CGO, FTS5 support), `github.com/fsnotify/fsnotify` (file watcher), OpenAI/Ollama embedding API (pluggable, optional).

---

## Task 1: Add Dependencies

**Files:**
- Modify: `go.mod`

**Step 1: Add the two new dependencies**

```bash
cd /path/to/hivemind-repo
go get modernc.org/sqlite@latest
go get github.com/fsnotify/fsnotify@latest
```

**Step 2: Verify they appear in go.mod and go.sum**

```bash
grep -E "modernc.org/sqlite|fsnotify" go.mod
```
Expected: two lines with the package names and versions.

**Step 3: Verify the project still builds**

```bash
go build ./...
```
Expected: no errors.

**Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add modernc.org/sqlite and fsnotify for IDE memory"
```

---

## Task 2: Types

**Files:**
- Create: `memory/types.go`

**Step 1: Create types.go with public types**

```go
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
    UpdatedAt  int64  // Unix ms
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
```

**Step 2: Build to confirm no errors**

```bash
go build ./memory/...
```
Expected: builds cleanly (no other files yet, just types).

**Step 3: Commit**

```bash
git add memory/types.go
git commit -m "feat(memory): add public types"
```

---

## Task 3: SQLite Schema

**Files:**
- Create: `memory/schema.go`
- Create: `memory/schema_test.go`

**Step 1: Write the failing test**

```go
// memory/schema_test.go
package memory

import (
    "testing"

    "github.com/stretchr/testify/require"
)

func TestOpenDB_CreatesAllTables(t *testing.T) {
    db, err := openDB(t.TempDir() + "/test.db")
    require.NoError(t, err)
    defer db.Close()

    // Verify all tables exist
    tables := []string{"files", "chunks", "chunks_fts", "chunks_vec", "embedding_cache"}
    for _, table := range tables {
        var name string
        row := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table)
        err := row.Scan(&name)
        require.NoError(t, err, "table %q should exist", table)
    }
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./memory/... -run TestOpenDB_CreatesAllTables -v
```
Expected: FAIL with "openDB not defined" (or build error).

**Step 3: Implement schema.go**

```go
// memory/schema.go
package memory

import (
    "database/sql"
    "fmt"
    "os"
    "path/filepath"

    _ "modernc.org/sqlite"
)

const schema = `
PRAGMA journal_mode=WAL;
PRAGMA foreign_keys=ON;

CREATE TABLE IF NOT EXISTS files (
    id    INTEGER PRIMARY KEY,
    path  TEXT    NOT NULL UNIQUE,
    mtime INTEGER NOT NULL,
    hash  TEXT    NOT NULL
);

CREATE TABLE IF NOT EXISTS chunks (
    id         INTEGER PRIMARY KEY,
    file_id    INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    start_line INTEGER NOT NULL,
    end_line   INTEGER NOT NULL,
    text       TEXT    NOT NULL
);

CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
    text,
    content='chunks',
    content_rowid='id'
);

CREATE TABLE IF NOT EXISTS chunks_vec (
    chunk_id  INTEGER NOT NULL REFERENCES chunks(id) ON DELETE CASCADE,
    embedding BLOB    NOT NULL
);

CREATE TABLE IF NOT EXISTS embedding_cache (
    text_hash TEXT    NOT NULL PRIMARY KEY,
    embedding BLOB    NOT NULL,
    provider  TEXT    NOT NULL,
    model     TEXT    NOT NULL,
    created   INTEGER NOT NULL
);
`

func openDB(dbPath string) (*sql.DB, error) {
    if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
        return nil, fmt.Errorf("create db dir: %w", err)
    }
    db, err := sql.Open("sqlite", dbPath)
    if err != nil {
        return nil, fmt.Errorf("open sqlite: %w", err)
    }
    if _, err := db.Exec(schema); err != nil {
        db.Close()
        return nil, fmt.Errorf("apply schema: %w", err)
    }
    return db, nil
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./memory/... -run TestOpenDB_CreatesAllTables -v
```
Expected: PASS.

**Step 5: Commit**

```bash
git add memory/schema.go memory/schema_test.go
git commit -m "feat(memory): SQLite schema with FTS5 and vector blob tables"
```

---

## Task 4: Markdown Chunking

**Files:**
- Create: `memory/chunks.go`
- Create: `memory/chunks_test.go`

**Step 1: Write the failing tests**

```go
// memory/chunks_test.go
package memory

import (
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestChunkMarkdown_SplitsOnHeaders(t *testing.T) {
    md := "# Section A\n\nSome content here.\n\n## Section B\n\nMore content."
    chunks := chunkMarkdown(md)
    require.Len(t, chunks, 2)
    assert.Contains(t, chunks[0].Text, "Section A")
    assert.Contains(t, chunks[0].Text, "Some content here.")
    assert.Contains(t, chunks[1].Text, "Section B")
    assert.Contains(t, chunks[1].Text, "More content.")
}

func TestChunkMarkdown_SingleChunkNoHeaders(t *testing.T) {
    md := "Just a single paragraph with no headers."
    chunks := chunkMarkdown(md)
    require.Len(t, chunks, 1)
    assert.Equal(t, md, chunks[0].Text)
    assert.Equal(t, 1, chunks[0].StartLine)
    assert.Equal(t, 1, chunks[0].EndLine)
}

func TestChunkMarkdown_LongParagraphSplit(t *testing.T) {
    // 900 chars of content, no headers — should split into 2 chunks
    long := ""
    for i := 0; i < 90; i++ {
        long += "word_number_" + string(rune('a'+i%26)) + " "
    }
    chunks := chunkMarkdown(long)
    assert.Greater(t, len(chunks), 1, "long paragraph should be split")
    for _, c := range chunks {
        assert.LessOrEqual(t, len(c.Text), maxChunkChars+50)
    }
}

func TestChunkMarkdown_LineNumbers(t *testing.T) {
    md := "Line one\nLine two\n\n# Header\n\nAfter header"
    chunks := chunkMarkdown(md)
    require.Len(t, chunks, 2)
    assert.Equal(t, 1, chunks[0].StartLine)
    assert.Equal(t, 4, chunks[1].StartLine)
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./memory/... -run TestChunkMarkdown -v
```
Expected: FAIL (build error, chunkMarkdown not defined).

**Step 3: Implement chunks.go**

```go
// memory/chunks.go
package memory

import (
    "crypto/sha256"
    "fmt"
    "strings"
)

const maxChunkChars = 800

// Chunk is a text segment from a memory file.
type Chunk struct {
    StartLine int
    EndLine   int
    Text      string
    Hash      string
}

// chunkMarkdown splits Markdown text into chunks by headers and paragraph
// boundaries, with a hard limit of maxChunkChars per chunk.
func chunkMarkdown(text string) []Chunk {
    lines := strings.Split(text, "\n")
    var chunks []Chunk
    var buf []string
    startLine := 1

    flush := func(endLine int) {
        content := strings.TrimSpace(strings.Join(buf, "\n"))
        if content == "" {
            return
        }
        // If chunk is too long, split further.
        if len(content) > maxChunkChars {
            sub := splitLong(content, startLine)
            chunks = append(chunks, sub...)
        } else {
            chunks = append(chunks, makeChunk(content, startLine, endLine))
        }
        buf = nil
    }

    for i, line := range lines {
        lineNum := i + 1
        isHeader := strings.HasPrefix(line, "#")
        if isHeader && len(buf) > 0 {
            flush(lineNum - 1)
            startLine = lineNum
        }
        buf = append(buf, line)
    }
    flush(len(lines))

    return chunks
}

func splitLong(text string, startLine int) []Chunk {
    var chunks []Chunk
    for len(text) > maxChunkChars {
        // Split at last space before limit.
        idx := strings.LastIndex(text[:maxChunkChars], " ")
        if idx <= 0 {
            idx = maxChunkChars
        }
        chunks = append(chunks, makeChunk(strings.TrimSpace(text[:idx]), startLine, startLine))
        text = strings.TrimSpace(text[idx:])
    }
    if text != "" {
        chunks = append(chunks, makeChunk(text, startLine, startLine))
    }
    return chunks
}

func makeChunk(text string, start, end int) Chunk {
    return Chunk{
        StartLine: start,
        EndLine:   end,
        Text:      text,
        Hash:      hashText(text),
    }
}

func hashText(s string) string {
    h := sha256.Sum256([]byte(s))
    return fmt.Sprintf("%x", h[:8])
}
```

**Step 4: Run tests to verify they pass**

```bash
go test ./memory/... -run TestChunkMarkdown -v
```
Expected: PASS.

**Step 5: Commit**

```bash
git add memory/chunks.go memory/chunks_test.go
git commit -m "feat(memory): Markdown chunking with header and length splits"
```

---

## Task 5: Vector Math

**Files:**
- Create: `memory/vectors.go`
- Create: `memory/vectors_test.go`

**Step 1: Write the failing tests**

```go
// memory/vectors_test.go
package memory

import (
    "math"
    "testing"

    "github.com/stretchr/testify/assert"
)

func TestCosine_IdenticalVectors(t *testing.T) {
    v := []float32{1, 2, 3}
    score := cosine(v, v)
    assert.InDelta(t, 1.0, score, 0.001)
}

func TestCosine_OrthogonalVectors(t *testing.T) {
    a := []float32{1, 0, 0}
    b := []float32{0, 1, 0}
    score := cosine(a, b)
    assert.InDelta(t, 0.0, score, 0.001)
}

func TestCosine_KnownValue(t *testing.T) {
    a := []float32{1, 0}
    b := []float32{1, 1}
    // cos(45°) = 1/sqrt(2) ≈ 0.707
    score := cosine(a, b)
    assert.InDelta(t, 1.0/math.Sqrt2, float64(score), 0.001)
}

func TestSerializeRoundtrip(t *testing.T) {
    v := []float32{1.5, 2.5, -3.0, 0.0}
    blob := serializeVec(v)
    got := deserializeVec(blob)
    assert.Equal(t, v, got)
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./memory/... -run TestCosine -v
```
Expected: FAIL (build error).

**Step 3: Implement vectors.go**

```go
// memory/vectors.go
package memory

import (
    "encoding/binary"
    "math"
)

// cosine returns the cosine similarity between two float32 vectors (range 0–1).
// Returns 0 if either vector is zero-length or dimensions mismatch.
func cosine(a, b []float32) float32 {
    if len(a) != len(b) || len(a) == 0 {
        return 0
    }
    var dot, normA, normB float64
    for i := range a {
        dot += float64(a[i]) * float64(b[i])
        normA += float64(a[i]) * float64(a[i])
        normB += float64(b[i]) * float64(b[i])
    }
    if normA == 0 || normB == 0 {
        return 0
    }
    return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}

// serializeVec converts a float32 slice to little-endian bytes for SQLite BLOB.
func serializeVec(v []float32) []byte {
    buf := make([]byte, len(v)*4)
    for i, f := range v {
        binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
    }
    return buf
}

// deserializeVec converts little-endian bytes back to a float32 slice.
func deserializeVec(b []byte) []float32 {
    v := make([]float32, len(b)/4)
    for i := range v {
        v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
    }
    return v
}
```

**Step 4: Run tests to verify they pass**

```bash
go test ./memory/... -run "TestCosine|TestSerialize" -v
```
Expected: PASS.

**Step 5: Commit**

```bash
git add memory/vectors.go memory/vectors_test.go
git commit -m "feat(memory): cosine similarity and vector serialization"
```

---

## Task 6: Embedding Providers

**Files:**
- Create: `memory/embeddings.go`

No test here — HTTP providers are tested via integration tests later. For now just the interface and implementations.

**Step 1: Create embeddings.go**

```go
// memory/embeddings.go
package memory

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

// noopProvider returns nil embeddings (FTS-only mode).
type noopProvider struct{}

func (n *noopProvider) Embed(_ []string) ([][]float32, error) { return nil, nil }
func (n *noopProvider) Dims() int                             { return 0 }
func (n *noopProvider) Name() string                          { return "none" }

// OpenAIProvider calls the OpenAI embeddings API.
type OpenAIProvider struct {
    APIKey string
    Model  string // default "text-embedding-3-small"
    client *http.Client
}

func NewOpenAIProvider(apiKey, model string) *OpenAIProvider {
    if model == "" {
        model = "text-embedding-3-small"
    }
    return &OpenAIProvider{APIKey: apiKey, Model: model, client: &http.Client{Timeout: 30 * time.Second}}
}

func (p *OpenAIProvider) Dims() int    { return 1536 }
func (p *OpenAIProvider) Name() string { return "openai/" + p.Model }

func (p *OpenAIProvider) Embed(texts []string) ([][]float32, error) {
    body, _ := json.Marshal(map[string]any{
        "input": texts,
        "model": p.Model,
    })
    req, _ := http.NewRequest("POST", "https://api.openai.com/v1/embeddings", bytes.NewReader(body))
    req.Header.Set("Authorization", "Bearer "+p.APIKey)
    req.Header.Set("Content-Type", "application/json")

    resp, err := p.client.Do(req)
    if err != nil {
        return nil, fmt.Errorf("openai embed: %w", err)
    }
    defer resp.Body.Close()

    var result struct {
        Data []struct {
            Embedding []float32 `json:"embedding"`
        } `json:"data"`
        Error *struct {
            Message string `json:"message"`
        } `json:"error"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, fmt.Errorf("openai decode: %w", err)
    }
    if result.Error != nil {
        return nil, fmt.Errorf("openai api: %s", result.Error.Message)
    }
    vecs := make([][]float32, len(result.Data))
    for i, d := range result.Data {
        vecs[i] = d.Embedding
    }
    return vecs, nil
}

// OllamaProvider calls a local Ollama server for embeddings.
type OllamaProvider struct {
    URL   string // e.g. "http://localhost:11434"
    Model string // e.g. "nomic-embed-text"
    client *http.Client
}

func NewOllamaProvider(url, model string) *OllamaProvider {
    if url == "" {
        url = "http://localhost:11434"
    }
    if model == "" {
        model = "nomic-embed-text"
    }
    return &OllamaProvider{URL: url, Model: model, client: &http.Client{Timeout: 60 * time.Second}}
}

func (p *OllamaProvider) Dims() int    { return 768 }
func (p *OllamaProvider) Name() string { return "ollama/" + p.Model }

func (p *OllamaProvider) Embed(texts []string) ([][]float32, error) {
    vecs := make([][]float32, 0, len(texts))
    for _, text := range texts {
        body, _ := json.Marshal(map[string]any{
            "model":  p.Model,
            "prompt": text,
        })
        req, _ := http.NewRequest("POST", p.URL+"/api/embeddings", bytes.NewReader(body))
        req.Header.Set("Content-Type", "application/json")

        resp, err := p.client.Do(req)
        if err != nil {
            return nil, fmt.Errorf("ollama embed: %w", err)
        }
        defer resp.Body.Close()

        var result struct {
            Embedding []float32 `json:"embedding"`
        }
        if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
            return nil, fmt.Errorf("ollama decode: %w", err)
        }
        vecs = append(vecs, result.Embedding)
    }
    return vecs, nil
}
```

**Step 2: Build to verify no errors**

```bash
go build ./memory/...
```
Expected: builds cleanly.

**Step 3: Commit**

```bash
git add memory/embeddings.go
git commit -m "feat(memory): embedding provider interface, OpenAI and Ollama implementations"
```

---

## Task 7: MemoryConfig

**Files:**
- Modify: `config/config.go`
- Modify: `config/config_test.go`

**Step 1: Write the failing test**

```go
// In config/config_test.go, add:
func TestMemoryConfig_DefaultsToEnabled(t *testing.T) {
    cfg := DefaultConfig()
    require.NotNil(t, cfg.Memory)
    assert.False(t, cfg.Memory.Enabled) // off by default until user configures it
}

func TestMemoryConfig_JSONRoundtrip(t *testing.T) {
    cfg := &Config{
        Memory: &MemoryConfig{
            Enabled:             true,
            EmbeddingProvider:   "openai",
            OpenAIAPIKey:        "sk-test",
            StartupInjectCount:  5,
        },
    }
    data, err := json.Marshal(cfg)
    require.NoError(t, err)

    var got Config
    require.NoError(t, json.Unmarshal(data, &got))
    require.NotNil(t, got.Memory)
    assert.Equal(t, "openai", got.Memory.EmbeddingProvider)
    assert.Equal(t, "sk-test", got.Memory.OpenAIAPIKey)
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./config/... -run TestMemoryConfig -v
```
Expected: FAIL (MemoryConfig not defined).

**Step 3: Add MemoryConfig to config.go**

In `config/config.go`, add before the `Config` struct:

```go
// MemoryConfig configures the IDE-wide memory system.
type MemoryConfig struct {
    // Enabled turns the memory system on/off. Default false until configured.
    Enabled bool `json:"enabled"`
    // EmbeddingProvider selects the embedding backend: "openai", "ollama", or "none".
    // When "none" or unset, memory search falls back to keyword-only (FTS).
    EmbeddingProvider string `json:"embedding_provider,omitempty"`
    // OpenAIAPIKey is the API key for OpenAI embeddings.
    OpenAIAPIKey string `json:"openai_api_key,omitempty"`
    // OpenAIModel is the embedding model name. Default "text-embedding-3-small".
    OpenAIModel string `json:"openai_model,omitempty"`
    // OllamaURL is the Ollama server URL. Default "http://localhost:11434".
    OllamaURL string `json:"ollama_url,omitempty"`
    // OllamaModel is the Ollama model name. Default "nomic-embed-text".
    OllamaModel string `json:"ollama_model,omitempty"`
    // StartupInjectCount controls how many memory snippets are injected into
    // CLAUDE.md when starting an agent. Default 5.
    StartupInjectCount int `json:"startup_inject_count,omitempty"`
}
```

And add to the `Config` struct:

```go
// Memory configures the IDE-wide memory system.
Memory *MemoryConfig `json:"memory,omitempty"`
```

**Step 4: Run test to verify it passes**

```bash
go test ./config/... -run TestMemoryConfig -v
```
Expected: PASS.

**Step 5: Commit**

```bash
git add config/config.go config/config_test.go
git commit -m "feat(config): add MemoryConfig for IDE-wide memory settings"
```

---

## Task 8: MemoryManager Core (Write + List + Get)

**Files:**
- Create: `memory/manager.go`
- Create: `memory/manager_test.go`

**Step 1: Write failing tests**

```go
// memory/manager_test.go
package memory

import (
    "path/filepath"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func newTestManager(t *testing.T) *Manager {
    t.Helper()
    dir := t.TempDir()
    mgr, err := NewManager(dir, nil) // nil = no embedding provider
    require.NoError(t, err)
    t.Cleanup(func() { mgr.Close() })
    return mgr
}

func TestManager_WriteCreatesFile(t *testing.T) {
    mgr := newTestManager(t)
    err := mgr.Write("The user prefers Go over Python.", "preferences.md")
    require.NoError(t, err)

    files, err := mgr.List()
    require.NoError(t, err)
    require.Len(t, files, 1)
    assert.Equal(t, "preferences.md", files[0].Path)
    assert.Greater(t, files[0].SizeBytes, int64(0))
}

func TestManager_WriteAppendsToExistingFile(t *testing.T) {
    mgr := newTestManager(t)
    require.NoError(t, mgr.Write("Fact one.", "notes.md"))
    require.NoError(t, mgr.Write("Fact two.", "notes.md"))

    result, err := mgr.Get("notes.md", 0, 0)
    require.NoError(t, err)
    assert.Contains(t, result, "Fact one.")
    assert.Contains(t, result, "Fact two.")
}

func TestManager_GetReturnsLines(t *testing.T) {
    mgr := newTestManager(t)
    content := "line1\nline2\nline3\nline4\nline5"
    require.NoError(t, mgr.Write(content, "test.md"))

    // Get lines 2-3
    result, err := mgr.Get("test.md", 2, 2)
    require.NoError(t, err)
    assert.Contains(t, result, "line2")
    assert.Contains(t, result, "line3")
    assert.NotContains(t, result, "line1")
}

func TestManager_ListReturnsChunkCount(t *testing.T) {
    mgr := newTestManager(t)
    // Write enough content to produce multiple chunks
    content := "# Section A\n\nSome content.\n\n# Section B\n\nMore content."
    require.NoError(t, mgr.Write(content, "multi.md"))

    // Force index sync
    require.NoError(t, mgr.Sync("multi.md"))

    files, err := mgr.List()
    require.NoError(t, err)
    require.Len(t, files, 1)
    assert.GreaterOrEqual(t, files[0].ChunkCount, 2)
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./memory/... -run TestManager_ -v
```
Expected: FAIL (Manager not defined).

**Step 3: Implement manager.go**

```go
// memory/manager.go
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

    // Delete old chunks (cascade deletes FTS + vec rows too via triggers — see below).
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

        // Insert into FTS (content table trigger replacement).
        if _, err := m.db.Exec(
            "INSERT INTO chunks_fts(rowid, text) VALUES (?, ?)",
            chunkID, chunk.Text,
        ); err != nil {
            return fmt.Errorf("insert fts: %w", err)
        }

        // Embed and insert vector if provider is available.
        if m.provider.Dims() > 0 {
            if err := m.embedAndStore(chunkID, chunk.Text, hash); err != nil {
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
```

**Step 4: Run tests to verify they pass**

```bash
go test ./memory/... -run TestManager_ -v
```
Expected: PASS.

**Step 5: Commit**

```bash
git add memory/manager.go memory/manager_test.go
git commit -m "feat(memory): MemoryManager Write/List/Get and indexing"
```

---

## Task 9: Hybrid Search

**Files:**
- Create: `memory/search.go`
- Add tests to: `memory/manager_test.go`

**Step 1: Write failing tests (append to manager_test.go)**

```go
func TestManager_SearchFTSOnly(t *testing.T) {
    mgr := newTestManager(t)
    require.NoError(t, mgr.Write("The user prefers Go over Python.", "prefs.md"))
    require.NoError(t, mgr.Write("Project Hivemind uses Bubble Tea for TUI.", "projects.md"))

    results, err := mgr.Search("Go language preference", SearchOpts{MaxResults: 5})
    require.NoError(t, err)
    // At least one result should mention Go
    found := false
    for _, r := range results {
        if strings.Contains(r.Snippet, "Go") {
            found = true
        }
    }
    assert.True(t, found, "expected to find Go-related memory")
}

func TestManager_SearchReturnsSnippets(t *testing.T) {
    mgr := newTestManager(t)
    require.NoError(t, mgr.Write("# User Setup\n\nMacBook Pro M3, 32GB RAM, macOS Sequoia.", "setup.md"))

    results, err := mgr.Search("user computer hardware", SearchOpts{MaxResults: 5})
    require.NoError(t, err)
    if len(results) > 0 {
        assert.NotEmpty(t, results[0].Snippet)
        assert.LessOrEqual(t, len(results[0].Snippet), snippetMaxChars+10)
    }
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./memory/... -run TestManager_Search -v
```
Expected: FAIL (Manager.Search not defined).

**Step 3: Implement search.go**

```go
// memory/search.go
package memory

import (
    "math"
    "sort"
    "strings"
    "time"
)

type scoredResult struct {
    SearchResult
    vectorScore float32
    bm25Score   float32
}

// Search performs hybrid search: FTS5 BM25 + optional cosine similarity.
func (m *Manager) Search(query string, opts SearchOpts) ([]SearchResult, error) {
    if opts.MaxResults <= 0 {
        opts.MaxResults = 10
    }

    m.mu.RLock()
    defer m.mu.RUnlock()

    // 1. BM25 keyword search via FTS5
    bm25Results, err := m.bm25Search(query, opts.MaxResults*2)
    if err != nil {
        return nil, err
    }

    // 2. Vector search (if embeddings are configured)
    var vecResults []scoredResult
    if m.provider.Dims() > 0 {
        vecResults, err = m.vectorSearch(query, opts.MaxResults*2)
        if err != nil {
            // Non-fatal: fall back to keyword-only
            vecResults = nil
        }
    }

    // 3. Merge results
    merged := mergeResults(bm25Results, vecResults, opts.MaxResults)

    // 4. Apply temporal decay
    merged = applyTemporalDecay(merged, m)

    // 5. Apply min score filter and return
    var out []SearchResult
    for _, r := range merged {
        if r.Score >= opts.MinScore {
            out = append(out, r.SearchResult)
        }
    }
    return out, nil
}

func (m *Manager) bm25Search(query string, limit int) ([]scoredResult, error) {
    // FTS5 bm25() returns negative values (lower = better); negate for consistency.
    rows, err := m.db.Query(`
        SELECT c.id, f.path, c.start_line, c.end_line, c.text,
               -bm25(chunks_fts) as score
        FROM chunks_fts
        JOIN chunks c ON chunks_fts.rowid = c.id
        JOIN files f ON c.file_id = f.id
        WHERE chunks_fts MATCH ?
        ORDER BY score DESC
        LIMIT ?
    `, ftsQuery(query), limit)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var results []scoredResult
    for rows.Next() {
        var chunkID int64
        var r scoredResult
        if err := rows.Scan(&chunkID, &r.Path, &r.StartLine, &r.EndLine, &r.Snippet, &r.bm25Score); err != nil {
            continue
        }
        r.Snippet = truncate(r.Snippet, snippetMaxChars)
        results = append(results, r)
    }
    return results, rows.Err()
}

func (m *Manager) vectorSearch(query string, limit int) ([]scoredResult, error) {
    vecs, err := m.provider.Embed([]string{query})
    if err != nil || len(vecs) == 0 {
        return nil, err
    }
    queryVec := vecs[0]

    // Load all chunk vectors into memory.
    rows, err := m.db.Query(`
        SELECT cv.chunk_id, cv.embedding, c.start_line, c.end_line, c.text, f.path
        FROM chunks_vec cv
        JOIN chunks c ON cv.chunk_id = c.id
        JOIN files f ON c.file_id = f.id
    `)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var results []scoredResult
    for rows.Next() {
        var chunkID int64
        var blob []byte
        var r scoredResult
        if err := rows.Scan(&chunkID, &blob, &r.StartLine, &r.EndLine, &r.Snippet, &r.Path); err != nil {
            continue
        }
        chunkVec := deserializeVec(blob)
        r.vectorScore = cosine(queryVec, chunkVec)
        r.Snippet = truncate(r.Snippet, snippetMaxChars)
        results = append(results, r)
    }
    if err := rows.Err(); err != nil {
        return nil, err
    }

    // Sort by vector score and take top-limit.
    sort.Slice(results, func(i, j int) bool {
        return results[i].vectorScore > results[j].vectorScore
    })
    if len(results) > limit {
        results = results[:limit]
    }
    return results, nil
}

func mergeResults(bm25, vec []scoredResult, limit int) []scoredResult {
    seen := map[string]int{} // path:startLine -> index in merged
    var merged []scoredResult

    // Normalize BM25 scores to 0-1 range
    bm25Results := normalizeScores(bm25, func(r scoredResult) float32 { return r.bm25Score })
    vecResults := normalizeScores(vec, func(r scoredResult) float32 { return r.vectorScore })

    for i, r := range bm25Results {
        key := r.Path + ":" + string(rune(r.StartLine))
        r.Score = 0.4 * bm25Results[i].bm25Score
        seen[key] = len(merged)
        merged = append(merged, r)
    }
    for _, r := range vecResults {
        key := r.Path + ":" + string(rune(r.StartLine))
        if idx, ok := seen[key]; ok {
            merged[idx].Score += 0.6 * r.vectorScore
        } else {
            r.Score = 0.6 * r.vectorScore
            merged = append(merged, r)
        }
    }

    sort.Slice(merged, func(i, j int) bool { return merged[i].Score > merged[j].Score })
    if len(merged) > limit {
        merged = merged[:limit]
    }
    return merged
}

func normalizeScores(results []scoredResult, getScore func(scoredResult) float32) []scoredResult {
    if len(results) == 0 {
        return results
    }
    var max float32
    for _, r := range results {
        if s := getScore(r); s > max {
            max = s
        }
    }
    if max == 0 {
        return results
    }
    out := make([]scoredResult, len(results))
    for i, r := range results {
        out[i] = r
        s := getScore(r) / max
        if getScore(r) == r.bm25Score {
            out[i].bm25Score = s
        } else {
            out[i].vectorScore = s
        }
    }
    return out
}

func applyTemporalDecay(results []scoredResult, m *Manager) []scoredResult {
    now := float64(time.Now().UnixMilli())
    for i := range results {
        var mtime int64
        row := m.db.QueryRow("SELECT mtime FROM files WHERE path=?", results[i].Path)
        if err := row.Scan(&mtime); err != nil {
            continue
        }
        ageDays := (now - float64(mtime)) / (86_400_000)
        decay := math.Exp(-0.01 * ageDays)
        results[i].Score *= float32(decay)
    }
    // Re-sort after decay.
    sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
    return results
}

// ftsQuery sanitizes a query string for FTS5 MATCH syntax.
func ftsQuery(q string) string {
    // Wrap each token in double quotes for phrase safety, join with OR.
    tokens := strings.Fields(q)
    if len(tokens) == 0 {
        return q
    }
    quoted := make([]string, len(tokens))
    for i, t := range tokens {
        quoted[i] = `"` + strings.ReplaceAll(t, `"`, "") + `"`
    }
    return strings.Join(quoted, " OR ")
}

func truncate(s string, max int) string {
    if len(s) <= max {
        return s
    }
    return s[:max] + "…"
}
```

**Step 4: Run tests to verify they pass**

```bash
go test ./memory/... -run TestManager_Search -v
```
Expected: PASS.

**Step 5: Commit**

```bash
git add memory/search.go memory/manager_test.go
git commit -m "feat(memory): hybrid search (FTS5 BM25 + cosine) with temporal decay"
```

---

## Task 10: File Watcher

**Files:**
- Create: `memory/watcher.go`

**Step 1: Create watcher.go** (no isolated unit test — integration covered by manager tests)

```go
// memory/watcher.go
package memory

import (
    "path/filepath"
    "strings"
    "time"

    "github.com/fsnotify/fsnotify"
)

// StartWatcher watches the memory directory for file changes and triggers
// re-indexing. Returns a stop function. Errors are logged but non-fatal.
func (m *Manager) StartWatcher() (stop func(), err error) {
    watcher, err := fsnotify.NewWatcher()
    if err != nil {
        return func() {}, err
    }
    if err := watcher.Add(m.dir); err != nil {
        watcher.Close()
        return func() {}, err
    }

    done := make(chan struct{})
    go func() {
        defer watcher.Close()
        // Debounce: collect events and process after 500ms quiet period.
        pending := map[string]struct{}{}
        timer := time.NewTimer(24 * time.Hour) // initially idle
        timer.Stop()

        for {
            select {
            case event, ok := <-watcher.Events:
                if !ok {
                    return
                }
                if !strings.HasSuffix(event.Name, ".md") {
                    continue
                }
                rel, err := filepath.Rel(m.dir, event.Name)
                if err != nil {
                    continue
                }
                pending[rel] = struct{}{}
                timer.Reset(500 * time.Millisecond)

            case <-timer.C:
                for rel := range pending {
                    _ = m.Sync(rel)
                }
                pending = map[string]struct{}{}

            case <-done:
                return
            }
        }
    }()

    return func() { close(done) }, nil
}
```

**Step 2: Build to verify no errors**

```bash
go build ./memory/...
```
Expected: builds cleanly.

**Step 3: Commit**

```bash
git add memory/watcher.go
git commit -m "feat(memory): fsnotify file watcher with 500ms debounce"
```

---

## Task 11: NewManager from Config

**Files:**
- Create: `memory/from_config.go`

**Step 1: Create from_config.go**

This wires together the config struct with MemoryManager construction — used by both the TUI and the MCP server binary.

```go
// memory/from_config.go
package memory

import (
    "fmt"
    "os"
    "path/filepath"

    "github.com/ByteMirror/hivemind/config"
)

// NewManagerFromConfig creates a MemoryManager from the application config.
// Returns (nil, nil) if memory is disabled or not configured.
func NewManagerFromConfig(cfg *config.Config) (*Manager, error) {
    if cfg.Memory == nil || !cfg.Memory.Enabled {
        return nil, nil
    }

    homeDir, err := os.UserHomeDir()
    if err != nil {
        return nil, fmt.Errorf("memory: get home dir: %w", err)
    }
    memDir := filepath.Join(homeDir, ".hivemind", "memory")

    var provider EmbeddingProvider
    switch cfg.Memory.EmbeddingProvider {
    case "openai":
        if cfg.Memory.OpenAIAPIKey == "" {
            return nil, fmt.Errorf("memory: openai provider requires openai_api_key in config")
        }
        provider = NewOpenAIProvider(cfg.Memory.OpenAIAPIKey, cfg.Memory.OpenAIModel)
    case "ollama":
        provider = NewOllamaProvider(cfg.Memory.OllamaURL, cfg.Memory.OllamaModel)
    default:
        provider = &noopProvider{} // FTS-only
    }

    return NewManager(memDir, provider)
}
```

**Step 2: Build to confirm**

```bash
go build ./memory/...
```
Expected: builds cleanly.

**Step 3: Commit**

```bash
git add memory/from_config.go
git commit -m "feat(memory): NewManagerFromConfig wires config to MemoryManager"
```

---

## Task 12: MCP Memory Tools

**Files:**
- Create: `mcp/memory_tools.go`

**Step 1: Create memory_tools.go**

```go
// mcp/memory_tools.go
package mcp

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/ByteMirror/hivemind/memory"
    gomcp "github.com/mark3labs/mcp-go/mcp"
    mcpserver "github.com/mark3labs/mcp-go/server"
)

// handleMemoryWrite saves a fact to the IDE-wide memory store.
func handleMemoryWrite(mgr *memory.Manager) mcpserver.ToolHandlerFunc {
    return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
        Log("tool call: memory_write")
        content := req.GetString("content", "")
        file := req.GetString("file", "")
        if content == "" {
            return gomcp.NewToolResultError("missing required parameter: content"), nil
        }
        if err := mgr.Write(content, file); err != nil {
            Log("memory_write error: %v", err)
            return gomcp.NewToolResultError("failed to write memory: " + err.Error()), nil
        }
        Log("memory_write: saved %d chars", len(content))
        return gomcp.NewToolResultText("Memory saved."), nil
    }
}

// handleMemorySearch searches the IDE-wide memory store.
func handleMemorySearch(mgr *memory.Manager) mcpserver.ToolHandlerFunc {
    return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
        Log("tool call: memory_search")
        query := req.GetString("query", "")
        if query == "" {
            return gomcp.NewToolResultError("missing required parameter: query"), nil
        }

        maxResults := 10
        if v := req.GetString("max_results", ""); v != "" {
            fmt.Sscanf(v, "%d", &maxResults)
        }

        results, err := mgr.Search(query, memory.SearchOpts{MaxResults: maxResults})
        if err != nil {
            Log("memory_search error: %v", err)
            return gomcp.NewToolResultError("search failed: " + err.Error()), nil
        }

        data, _ := json.MarshalIndent(results, "", "  ")
        Log("memory_search: query=%q results=%d", query, len(results))
        return gomcp.NewToolResultText(string(data)), nil
    }
}

// handleMemoryGet reads specific lines from a memory file.
func handleMemoryGet(mgr *memory.Manager) mcpserver.ToolHandlerFunc {
    return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
        Log("tool call: memory_get")
        relPath := req.GetString("path", "")
        if relPath == "" {
            return gomcp.NewToolResultError("missing required parameter: path"), nil
        }
        var from, lines int
        fmt.Sscanf(req.GetString("from", "0"), "%d", &from)
        fmt.Sscanf(req.GetString("lines", "0"), "%d", &lines)

        text, err := mgr.Get(relPath, from, lines)
        if err != nil {
            return gomcp.NewToolResultError("failed to read memory file: " + err.Error()), nil
        }
        return gomcp.NewToolResultText(text), nil
    }
}

// handleMemoryList lists all memory files.
func handleMemoryList(mgr *memory.Manager) mcpserver.ToolHandlerFunc {
    return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
        Log("tool call: memory_list")
        files, err := mgr.List()
        if err != nil {
            return gomcp.NewToolResultError("failed to list memory: " + err.Error()), nil
        }
        data, _ := json.MarshalIndent(files, "", "  ")
        Log("memory_list: %d files", len(files))
        return gomcp.NewToolResultText(string(data)), nil
    }
}
```

**Step 2: Build to verify**

```bash
go build ./mcp/...
```
Expected: builds cleanly.

**Step 3: Commit**

```bash
git add mcp/memory_tools.go
git commit -m "feat(mcp): add memory_write, memory_search, memory_get, memory_list tool handlers"
```

---

## Task 13: Register Memory Tools in MCP Server

**Files:**
- Modify: `mcp/server.go`

**Step 1: Add `memoryMgr` field to HivemindMCPServer**

In `mcp/server.go`, add `memoryMgr *memory.Manager` field to `HivemindMCPServer` struct:

```go
// In the HivemindMCPServer struct:
memoryMgr *memory.Manager // nil when memory is disabled
```

Update `NewHivemindMCPServer` signature to accept a memory manager:

```go
func NewHivemindMCPServer(brainClient BrainClient, hivemindDir, instanceID, repoPath string, tier int, memMgr *memory.Manager) *HivemindMCPServer {
```

Set `h.memoryMgr = memMgr` inside the constructor, then add a call to `h.registerMemoryTools()` if `memMgr != nil`:

```go
if memMgr != nil {
    h.registerMemoryTools()
}
```

**Step 2: Add registerMemoryTools method**

```go
// registerMemoryTools registers the IDE-wide memory tools (all tiers).
func (h *HivemindMCPServer) registerMemoryTools() {
    mgr := h.memoryMgr

    memWrite := gomcp.NewTool("memory_write",
        gomcp.WithDescription(
            "Write to IDE-wide memory. "+
                "Use this whenever you discover something worth remembering across sessions: "+
                "user preferences, project facts, environment setup, API keys configured, "+
                "decisions made and their rationale.",
        ),
        gomcp.WithString("content",
            gomcp.Required(),
            gomcp.Description("The fact or note to save. Plain text or Markdown."),
        ),
        gomcp.WithString("file",
            gomcp.Description("Target filename in ~/.hivemind/memory/ (default: YYYY-MM-DD.md)."),
        ),
    )
    h.server.AddTool(memWrite, handleMemoryWrite(mgr))

    memSearch := gomcp.NewTool("memory_search",
        gomcp.WithDescription(
            "Search IDE-wide memory before answering questions about prior work, "+
                "user preferences, project setups, or past decisions. "+
                "Returns ranked snippets with file path and line numbers.",
        ),
        gomcp.WithString("query",
            gomcp.Required(),
            gomcp.Description("Natural language search query."),
        ),
        gomcp.WithNumber("max_results",
            gomcp.Description("Maximum results to return (default 10)."),
        ),
    )
    h.server.AddTool(memSearch, handleMemorySearch(mgr))

    memGet := gomcp.NewTool("memory_get",
        gomcp.WithDescription(
            "Read specific lines from a memory file. "+
                "Use after memory_search to pull only the relevant lines.",
        ),
        gomcp.WithString("path",
            gomcp.Required(),
            gomcp.Description("Relative path within ~/.hivemind/memory/ (from memory_search results)."),
        ),
        gomcp.WithNumber("from",
            gomcp.Description("Start line number (1-indexed, default 1)."),
        ),
        gomcp.WithNumber("lines",
            gomcp.Description("Number of lines to read (default: entire file)."),
        ),
    )
    h.server.AddTool(memGet, handleMemoryGet(mgr))

    memList := gomcp.NewTool("memory_list",
        gomcp.WithDescription("List all IDE-wide memory files with metadata."),
        gomcp.WithReadOnlyHintAnnotation(true),
    )
    h.server.AddTool(memList, handleMemoryList(mgr))
}
```

**Step 3: Update cmd/mcp-server/main.go to pass memory manager**

In `cmd/mcp-server/main.go`, after loading config, create the memory manager:

```go
cfg := config.LoadConfig()
memMgr, _ := memory.NewManagerFromConfig(cfg)
if memMgr != nil {
    stop, _ := memMgr.StartWatcher()
    defer stop()
}
// Pass memMgr to NewHivemindMCPServer
```

**Step 4: Build the full project**

```bash
go build ./...
```
Expected: builds cleanly.

**Step 5: Commit**

```bash
git add mcp/server.go cmd/mcp-server/main.go
git commit -m "feat(mcp): register memory tools in MCP server, wire memory manager"
```

---

## Task 14: CLAUDE.md Startup Injection

**Files:**
- Create: `session/memory_inject.go`
- Modify: `session/instance_lifecycle.go`

**Step 1: Create session/memory_inject.go**

```go
// session/memory_inject.go
package session

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"

    "github.com/ByteMirror/hivemind/memory"
)

const (
    memoryInjectHeader = "<!-- hivemind-memory-start -->"
    memoryInjectFooter = "<!-- hivemind-memory-end -->"
)

// injectMemoryContext queries the IDE memory and appends/replaces the
// Hivemind Memory section in the worktree's CLAUDE.md.
// This is called on every instance start so context is always fresh.
func injectMemoryContext(worktreePath string, mgr *memory.Manager, count int) error {
    if mgr == nil {
        return nil
    }
    if count <= 0 {
        count = 5
    }

    // Query memory with a broad context query.
    query := filepath.Base(worktreePath) + " project setup preferences environment"
    results, err := mgr.Search(query, memory.SearchOpts{MaxResults: count})
    if err != nil {
        return fmt.Errorf("memory query for CLAUDE.md: %w", err)
    }

    section := buildMemorySection(results)

    claudeMDPath := filepath.Join(worktreePath, "CLAUDE.md")
    return upsertMemorySection(claudeMDPath, section)
}

// buildMemorySection formats search results into a Markdown section.
func buildMemorySection(results []memory.SearchResult) string {
    var b strings.Builder
    b.WriteString(memoryInjectHeader + "\n")
    b.WriteString("## Hivemind Memory\n\n")
    b.WriteString("This IDE maintains a persistent knowledge base. Use your MCP tools:\n")
    b.WriteString("- `memory_write(content)` — save important facts you discover\n")
    b.WriteString("- `memory_search(query)` — recall prior context before answering\n")
    b.WriteString("- `memory_get(path, from, lines)` — read specific snippets\n")
    b.WriteString("- `memory_list()` — list all memory files\n\n")
    b.WriteString("**Always call `memory_search` before answering questions about the user's** ")
    b.WriteString("preferences, setup, past decisions, or active projects.\n\n")

    if len(results) > 0 {
        b.WriteString("### Top Relevant Memories\n\n")
        for _, r := range results {
            b.WriteString(fmt.Sprintf("**[%s L%d]** %s\n\n", r.Path, r.StartLine, r.Snippet))
        }
    }

    b.WriteString(memoryInjectFooter + "\n")
    return b.String()
}

// upsertMemorySection writes the memory section into CLAUDE.md,
// replacing any existing hivemind-memory block or appending if absent.
func upsertMemorySection(claudeMDPath, section string) error {
    var existing string
    if data, err := os.ReadFile(claudeMDPath); err == nil {
        existing = string(data)
    }

    startIdx := strings.Index(existing, memoryInjectHeader)
    endIdx := strings.Index(existing, memoryInjectFooter)

    var updated string
    if startIdx >= 0 && endIdx >= 0 {
        // Replace existing block.
        updated = existing[:startIdx] + section + existing[endIdx+len(memoryInjectFooter)+1:]
    } else {
        // Append at end.
        if existing != "" && !strings.HasSuffix(existing, "\n") {
            existing += "\n"
        }
        updated = existing + "\n" + section
    }

    return os.WriteFile(claudeMDPath, []byte(updated), 0600)
}
```

**Step 2: Write failing test for upsertMemorySection**

```go
// session/memory_inject_test.go
package session

import (
    "os"
    "testing"

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
```

**Step 3: Run tests to verify they fail, then pass after implementation**

```bash
go test ./session/... -run TestUpsertMemorySection -v
```
Expected after implementation: PASS.

**Step 4: Hook into instance_lifecycle.go**

In `session/instance_lifecycle.go`, in the `Start()` method, add after the MCP server registration (around line 93):

```go
// Inject IDE memory context into CLAUDE.md before starting the agent.
if memMgr := getMemoryManager(); memMgr != nil {
    worktreePath := i.gitWorktree.GetWorktreePath()
    count := getMemoryInjectCount()
    go func() {
        if err := injectMemoryContext(worktreePath, memMgr, count); err != nil {
            log.WarningLog.Printf("memory inject: %v", err)
        }
    }()
}
```

> **Note:** `getMemoryManager()` and `getMemoryInjectCount()` are package-level functions set by the TUI. Add these to a new file `session/memory_state.go`:

```go
// session/memory_state.go
package session

import (
    "sync"
    "github.com/ByteMirror/hivemind/memory"
)

var (
    globalMemMgr   *memory.Manager
    globalMemCount int
    memMu          sync.RWMutex
)

// SetMemoryManager configures the memory manager used for startup injection.
// Called once from app.go when the TUI starts.
func SetMemoryManager(mgr *memory.Manager, count int) {
    memMu.Lock()
    defer memMu.Unlock()
    globalMemMgr = mgr
    globalMemCount = count
}

func getMemoryManager() *memory.Manager {
    memMu.RLock()
    defer memMu.RUnlock()
    return globalMemMgr
}

func getMemoryInjectCount() int {
    memMu.RLock()
    defer memMu.RUnlock()
    if globalMemCount <= 0 {
        return 5
    }
    return globalMemCount
}
```

**Step 5: Initialize memory manager in app.go**

In `app/app.go`, after loading config:

```go
cfg := config.LoadConfig()
memMgr, err := memory.NewManagerFromConfig(cfg)
if err != nil {
    log.WarningLog.Printf("memory init: %v", err)
} else if memMgr != nil {
    injectCount := 5
    if cfg.Memory != nil && cfg.Memory.StartupInjectCount > 0 {
        injectCount = cfg.Memory.StartupInjectCount
    }
    session.SetMemoryManager(memMgr, injectCount)
    stop, _ := memMgr.StartWatcher()
    // Store stop func so it can be called on app shutdown.
    _ = stop
}
```

**Step 6: Build the whole project**

```bash
go build ./...
```
Expected: builds cleanly.

**Step 7: Run all tests**

```bash
go test ./...
```
Expected: all pass.

**Step 8: Commit**

```bash
git add session/memory_inject.go session/memory_inject_test.go \
        session/memory_state.go session/instance_lifecycle.go app/app.go
git commit -m "feat(session): inject IDE memory context into CLAUDE.md at agent startup"
```

---

## Task 15: Update MCP Server Instructions

**Files:**
- Modify: `mcp/server.go`

**Step 1: Add memory instructions to serverInstructions constant**

Append to the `serverInstructions` constant:

```go
const serverInstructions = "You are running inside Hivemind, a multi-agent orchestration system. " +
    // ... existing text ...
    "gitStatus: This is the git status at the start of the conversation. ..." +
    // ADD:
    "\n\nThis IDE has a persistent memory store. " +
    "ALWAYS call memory_search before answering questions about the user's preferences, " +
    "environment setup, active projects, or past decisions. " +
    "Call memory_write whenever you discover something worth remembering across sessions."
```

**Step 2: Build and test**

```bash
go build ./...
go test ./...
```
Expected: builds and all tests pass.

**Step 3: Final commit**

```bash
git add mcp/server.go
git commit -m "feat(mcp): add memory instructions to MCP server system prompt"
```

---

## Verification

After all tasks are complete, run the full test suite:

```bash
go build ./...
go test ./...
go vet ./...
```

All should pass. To test end-to-end manually:
1. Add `"memory": {"enabled": true}` to `~/.hivemind/config.json`
2. Run `hivemind` and create an agent
3. In the agent, call `memory_write` with a test fact
4. Restart the agent and call `memory_search` — the fact should appear
5. Open the worktree's CLAUDE.md and verify the Hivemind Memory section was injected
