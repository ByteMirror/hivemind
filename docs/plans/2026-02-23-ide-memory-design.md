# IDE-Wide Memory System — Design

**Date**: 2026-02-23
**Status**: Approved
**Scope**: Global IDE-level persistent memory for all Hivemind agents

---

## Problem

Hivemind launches external coding agents (Claude Code, Aider, etc.) that have no memory across sessions or across projects. Each agent starts fresh, unaware of the user's setup, preferences, active projects, or decisions made in previous sessions. The goal is an IDE-wide memory system that all agents can read from and write to — a persistent knowledge base that grows smarter over time.

**Key challenge**: Unlike OpenClaw (which controls the full LLM inference pipeline), Hivemind spawns external processes. We can't inject tools into the agent runtime directly. Our two injection vectors are:

1. **MCP server** (`hivemind-mcp`) — already registered with Claude Code via `claude mcp add`. We add memory tools here.
2. **CLAUDE.md startup injection** — Hivemind writes a curated memory summary into the worktree's CLAUDE.md before launching each agent.

---

## Architecture

### Storage: `~/.hivemind/memory/`

Global IDE-wide memory, not scoped per project:

```
~/.hivemind/memory/
  global.md           # User preferences, computer setup, cross-project facts
  <yyyy-mm-dd>.md     # Agent-written dated notes (auto-named by agent)
  .index/
    memory.db         # SQLite: FTS5 index + vector BLOBs + embedding cache
```

Agents write Markdown files. The indexer watches for changes and keeps the database in sync.

### New Package: `memory/`

```
memory/
  manager.go     # MemoryManager — public API (Write, Search, Get, List)
  chunks.go      # Markdown chunking (by header / paragraph boundary)
  embeddings.go  # EmbeddingProvider interface + OpenAI + Ollama implementations
  fts.go         # FTS5 keyword search operations
  vectors.go     # Cosine similarity in pure Go (no CGO)
  schema.go      # SQLite schema creation and migrations
  watcher.go     # fsnotify-based file watcher (triggers re-index on change)
  types.go       # Public types (SearchResult, SearchOpts, ProviderStatus)
```

### SQLite Schema

```sql
CREATE TABLE files (
  id      INTEGER PRIMARY KEY,
  path    TEXT NOT NULL UNIQUE,   -- relative to memory dir
  mtime   INTEGER NOT NULL,       -- Unix ms
  hash    TEXT NOT NULL
);

CREATE TABLE chunks (
  id         INTEGER PRIMARY KEY,
  file_id    INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  start_line INTEGER NOT NULL,
  end_line   INTEGER NOT NULL,
  text       TEXT NOT NULL
);

CREATE VIRTUAL TABLE chunks_fts USING fts5(
  text,
  content='chunks',
  content_rowid='id'
);

CREATE TABLE chunks_vec (
  chunk_id  INTEGER NOT NULL REFERENCES chunks(id) ON DELETE CASCADE,
  embedding BLOB NOT NULL   -- float32[] serialized as little-endian bytes
);

CREATE TABLE embedding_cache (
  text_hash TEXT NOT NULL PRIMARY KEY,   -- SHA256 of chunk text
  embedding BLOB NOT NULL,
  provider  TEXT NOT NULL,
  model     TEXT NOT NULL,
  created   INTEGER NOT NULL
);
```

**No CGO dependency**: Using `modernc.org/sqlite` (pure Go SQLite port with FTS5 support). Vectors stored as BLOB, cosine similarity computed in Go — sufficient for IDE-scale memory (<10k chunks, <5ms search time).

---

## MCP Tools (added to `hivemind-mcp`)

### `memory_write`

Write a memory to the IDE-wide knowledge base.

| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `content` | string | yes | The fact/note to remember |
| `file` | string | no | Target file name in `~/.hivemind/memory/` (default: `YYYY-MM-DD.md`) |

After write: re-indexes the file (FTS + optional embedding).

---

### `memory_search`

Search the IDE-wide memory for relevant context.

| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `query` | string | yes | Natural language query |
| `max_results` | int | no | Default 10 |
| `min_score` | float | no | Default 0.3 |

Returns: `[{path, start_line, end_line, score, snippet}]`

Hybrid search: cosine similarity (if embeddings configured) + BM25 FTS, merged and ranked.

---

### `memory_get`

Read specific lines from a memory file.

| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `path` | string | yes | Relative path in memory dir |
| `from` | int | no | Start line |
| `lines` | int | no | Line count |

---

### `memory_list`

List all memory files with metadata.

Returns: `[{path, size_bytes, updated_at, chunk_count}]`

---

## Config (`~/.hivemind/config.json`)

```json
{
  "memory": {
    "enabled": true,
    "embedding_provider": "openai",
    "openai_api_key": "sk-...",
    "openai_model": "text-embedding-3-small",
    "ollama_url": "http://localhost:11434",
    "ollama_model": "nomic-embed-text",
    "startup_inject_count": 5
  }
}
```

Fallback order: `openai` → `ollama` → `none` (FTS-only, no embeddings).

---

## Agent Startup Injection

When Hivemind starts an agent instance, before launching the tmux session:

1. Query memory for context relevant to the current worktree/project
2. Append a `## Hivemind Memory` section to the worktree's CLAUDE.md

```markdown
## Hivemind Memory (auto-injected, do not edit)

This IDE maintains a persistent memory store. Use your MCP tools:
- `memory_write(content)` — save important facts you discover
- `memory_search(query)` — recall prior context before answering
- `memory_get(path, from, lines)` — read specific memory snippets

What's worth writing to memory:
- User's OS, hardware, terminal setup
- API keys / services configured
- Project tech stack preferences
- Decisions and their rationale
- Recurring patterns the user prefers or dislikes

Top relevant memories for this session:
[injected top-N memory snippets, each with path citation]
```

This section is **replaced on each instance restart** (idempotent). Agents see relevant context immediately without any tool call.

---

## Search Algorithm

### Step 1: Candidate Retrieval

```
IF embedding provider configured:
  embed(query) → cosine_similarity(query_vec, all_chunk_vecs) → top-N

ALWAYS:
  FTS5 BM25 match(query) → top-N keyword results
```

### Step 2: Score Merging

```
final_score = 0.6 * normalized_vector_score + 0.4 * normalized_bm25_score
```

If no embeddings: `final_score = bm25_score` only.

### Step 3: Temporal Decay

Newer memories rank higher:

```
age_days   = (now - file.mtime_ms) / 86_400_000
decay      = exp(-0.01 * age_days)   // half-life ≈ 69 days
final_score *= decay
```

### Step 4: Dedup + Sort

Remove overlapping chunks from the same file; sort descending by `final_score`.

### Embedding Cache

Before calling the API: hash the chunk text (SHA256). Check `embedding_cache` for existing vector. Only call API for new or changed chunks — dramatically reduces cost.

---

## Key Files Changed / Created

| File | Change |
|------|--------|
| `memory/` | New package (all files listed above) |
| `config/config.go` | Add `MemoryConfig` struct |
| `mcp/memory_tools.go` | New file: 4 memory tool handlers |
| `mcp/server.go` | Register memory tools (tier 1+) |
| `session/instance_lifecycle.go` | Call `injectMemoryContext()` at start |
| `session/memory_inject.go` | New file: CLAUDE.md injection logic |

---

## Implementation Notes

### Chunking Strategy

Memory files are Markdown. Chunk boundaries:
1. H1/H2/H3 header lines start a new chunk
2. Two consecutive blank lines start a new chunk
3. Max chunk size: 800 characters (split long paragraphs)

### File Watcher

Using `github.com/fsnotify/fsnotify` to watch `~/.hivemind/memory/` for `CREATE`, `WRITE`, `REMOVE` events. Debounce 500ms to avoid re-indexing on every keystroke.

### Thread Safety

`MemoryManager` uses a `sync.RWMutex` — multiple agents can search concurrently, writes serialize.

### Memory Tool Instructions

The MCP tool descriptions tell agents when to use them:
- `memory_search`: "Search IDE-wide memory before answering questions about prior work, user preferences, setup, or active projects."
- `memory_write`: "Write to IDE-wide memory when you discover something worth remembering across sessions."

---

## Out of Scope (Future)

- Session transcript auto-indexing (like OpenClaw's `sessions` source) — requires API for summarization
- Per-agent private memory (separate from global pool)
- Memory UI panel in the Hivemind TUI
- Memory export/import
