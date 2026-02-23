# Memory System — Test Guide

Manual end-to-end tests for the IDE-wide memory system (`memory/` package, MCP tools, CLAUDE.md injection).

---

## Setup

Add to `~/.hivemind/config.json`:

```json
{
  "memory": {
    "enabled": true
  }
}
```

`embedding_provider` defaults to `"none"` — FTS keyword search only, no API key required. Sufficient for all tests except Test 8 (Claude re-ranking).

Rebuild after any config or code change:

```bash
go build ./...
```

---

## Test 1 — CLAUDE.md injection at agent startup

**Goal:** Verify that a `## Hivemind Memory` block is written into the agent's CLAUDE.md when it starts.

**Steps:**

1. Create a seed file so there is content to inject:

   ```bash
   mkdir -p ~/.hivemind/memory
   cat > ~/.hivemind/memory/global.md << 'EOF'
   # My Setup

   MacBook Pro M3 Max. macOS Sequoia. Terminal: Ghostty. Shell: zsh.
   Language preference: Go for backend, TypeScript for tooling.
   EOF
   ```

2. Start Hivemind and create an agent instance (any project).

3. Inspect the agent's CLAUDE.md:

   ```bash
   grep -A 40 "hivemind-memory-start" <worktree-path>/CLAUDE.md
   ```

**Expected:** A `<!-- hivemind-memory-start --> ... <!-- hivemind-memory-end -->` block is present containing the Rules section, Tools table, and a `### Relevant context for this session` section quoting the `global.md` content.

**Failure modes:**
- Block absent → memory not enabled, or `injectMemoryContext` errored (check `~/.hivemind/logs/`)
- Block present but empty snippets → `global.md` content did not match the search query — try adding more specific keywords to the file

---

## Test 2 — `memory_write` tool

**Goal:** Verify that an agent can write a new memory entry and it appears in the correct file.

**Steps:**

1. Start an agent. Prompt it:

   > "Please write to memory: I always use conventional commits format with lowercase subject lines. Save it to global.md."

2. Watch the agent call `memory_write` with `file="global.md"`.

3. Verify the file was written:

   ```bash
   cat ~/.hivemind/memory/global.md
   ```

**Expected:** The commit style preference appears at the end of `global.md`, separated from existing content by a blank line.

**Variations:**
- Prompt without specifying a file → agent writes to today's `YYYY-MM-DD.md`
- Prompt with a custom filename → file is created at `~/.hivemind/memory/<name>.md`

---

## Test 3 — `memory_search` tool

**Goal:** Verify that keyword search finds relevant content from memory files.

**Steps:**

1. Ensure `global.md` contains at least two different facts (e.g. hardware setup and commit style from Test 2).

2. Start an agent. Prompt it:

   > "Use memory_search to find anything I've written about commit messages."

3. Watch the tool call and its result.

**Expected:** Agent calls `memory_search("commit messages")` (or similar) and returns the snippet about conventional commits with a file path and line number citation (e.g. `[global.md L12]`).

**Failure modes:**
- No results → FTS5 index may not have been built; check that `~/.hivemind/memory/.index/memory.db` exists
- Wrong results → verify `global.md` content matches the search terms (FTS5 is keyword-based, not semantic)

---

## Test 4 — `memory_get` tool

**Goal:** Verify that an agent can read specific lines from a memory file.

**Steps:**

1. Start an agent. Prompt it:

   > "Use memory_get to read lines 1–5 of global.md."

**Expected:** Agent calls `memory_get(path="global.md", from=1, lines=5)` and returns the first 5 lines of the file verbatim.

**Variations:**
- Omit `lines` parameter → entire file is returned
- Use a line number from a `memory_search` result → agent reads context around a snippet

---

## Test 5 — `memory_list` tool

**Goal:** Verify that the tool returns metadata for all memory files.

**Steps:**

1. Ensure at least two files exist in `~/.hivemind/memory/` (e.g. `global.md` and a dated file).

2. Start an agent. Prompt it:

   > "Use memory_list to show me all files in memory."

**Expected:** Agent calls `memory_list()` and returns a list with each file's path, size, last updated timestamp, and chunk count.

---

## Test 6 — Cross-agent write and read

**Goal:** Verify that a fact written by Agent A is visible to Agent B in a completely separate worktree.

**Steps:**

1. Start **Agent A** in any worktree. Prompt it:

   > "Remember that I always want git commit messages in lowercase conventional commits format. Write it to memory."

2. Watch Agent A call `memory_write`. Note the file it writes to.

3. Kill Agent A. Start **Agent B** in a *different* worktree (different project, different branch).

4. Prompt Agent B:

   > "What commit message style do I prefer?"

**Expected:** Agent B calls `memory_search("commit message style")` and answers with the lowercase conventional commits preference — without being told anything in this session.

**Failure modes:**
- Agent B doesn't search → memory instructions not injected into CLAUDE.md; check Test 1
- Search returns no results → check `~/.hivemind/memory/*.md` for the written content

---

## Test 7 — Idempotent re-injection

**Goal:** Verify that restarting an agent updates the memory block in CLAUDE.md rather than appending a duplicate.

**Steps:**

1. Start an agent. Confirm CLAUDE.md contains exactly one `<!-- hivemind-memory-start --> ... <!-- hivemind-memory-end -->` block.

2. Write a new memory fact:

   > "Remember that I prefer Go 1.24 or newer. Write it to global.md."

3. Restart the same agent (Stop → Start in Hivemind).

4. Check CLAUDE.md:

   ```bash
   grep -c "hivemind-memory-start" <worktree-path>/CLAUDE.md
   ```

**Expected:** Count is `1` (still exactly one block). The block now includes the Go version preference in the injected snippets.

**Failure modes:**
- Count is `2` → `upsertMemorySection` failed to detect the existing block

---

## Test 8 — Claude re-ranking (semantic search)

**Goal:** Verify that semantically related content is found even without exact keyword overlap.

**Requires:** Claude binary in `PATH` with a valid session (API key or Max subscription).

**Config:**

```json
{
  "memory": {
    "enabled": true,
    "embedding_provider": "claude"
  }
}
```

**Steps:**

1. Write a fact using words you will *not* search with:

   ```bash
   cat >> ~/.hivemind/memory/global.md << 'EOF'

   The user dislikes verbosity. They prefer terse, direct implementations.
   Minimal abstractions. No over-engineering.
   EOF
   ```

2. Start an agent. Prompt it:

   > "Use memory_search to find anything about my coding style preferences."

   (Query uses "coding style" — the memory text uses "verbosity", "terse", "minimal".)

**Expected:** Agent finds and returns the snippet. With `embedding_provider: "none"` (FTS only) this would return nothing; with `"claude"` re-ranking, the Claude Haiku model re-orders the FTS candidates by semantic relevance so the snippet surfaces.

**Verify re-ranker is active:**

```bash
# The re-ranker spawns a claude process; you can confirm with:
ps aux | grep claude
# Should show a short-lived claude -p --model claude-haiku-4-5-20251001 process during search
```

**Failure modes:**
- Nothing found with `"claude"` provider → check that `claude` is in PATH and authenticated (`claude --version`)
- Falls back to FTS order → expected graceful behaviour when claude is unavailable

---

## Test 9 — File watcher (live re-index)

**Goal:** Verify that manually editing a memory file triggers re-indexing without restarting Hivemind.

**Steps:**

1. Start Hivemind with an agent running.

2. Manually append a new fact to a memory file:

   ```bash
   echo "\n\nI use Ghostty as my terminal emulator." >> ~/.hivemind/memory/global.md
   ```

3. Wait ~1 second (debounce delay), then from the agent:

   > "Use memory_search to find anything about my terminal."

**Expected:** Agent finds "Ghostty" — the file watcher detected the change and re-indexed the file without any restart.

**Failure modes:**
- Not found → watcher may not be running; check that the TUI process started the watcher via `app.go`

---

## Test 10 — `global.md` as permanent knowledge base

**Goal:** Verify that `global.md` content is available to every agent across all projects.

**Steps:**

1. Write permanent facts to `~/.hivemind/memory/global.md`:

   ```markdown
   # User Profile

   Name: Fabian. Based in Europe (CET timezone).
   Primary language: German, but prefers English in code and commits.
   Current main project: Hivemind (Go, Bubble Tea TUI).
   ```

2. Start 3 different agents across different projects/worktrees.

3. Ask each one: "What's my name and main project?"

**Expected:** All three answer correctly from memory — demonstrating that `global.md` provides universal context across all sessions without any manual re-telling.

---

## Automated smoke test (no running Hivemind required)

Exercises the `memory` package directly — safe to run in CI or locally without any config.

```bash
# From repo root:
go test ./memory/... -v -run "TestManager_"
```

For a more realistic integration test, create and run this script:

```bash
cat > /tmp/memory_smoke_test.go << 'EOF'
package main

import (
	"fmt"
	"os"
	"github.com/ByteMirror/hivemind/memory"
)

func main() {
	dir, _ := os.MkdirTemp("", "memory-test")
	defer os.RemoveAll(dir)

	mgr, err := memory.NewManager(dir, nil)
	if err != nil { panic(err) }
	defer mgr.Close()

	// Test Write
	mgr.Write("The user prefers Go over Python for backend services.", "prefs.md")
	mgr.Write("MacBook Pro M3, 64GB RAM, macOS Sequoia.", "setup.md")

	// Test Search
	results, err := mgr.Search("programming language preference", memory.SearchOpts{MaxResults: 5})
	if err != nil { panic(err) }
	fmt.Printf("Search returned %d result(s)\n", len(results))
	for _, r := range results {
		fmt.Printf("  [%s L%d] score=%.3f: %.80s\n", r.Path, r.StartLine, r.Score, r.Snippet)
	}

	// Test List
	files, err := mgr.List()
	if err != nil { panic(err) }
	fmt.Printf("List returned %d file(s)\n", len(files))

	// Test Get
	content, err := mgr.Get("prefs.md", 1, 0)
	if err != nil { panic(err) }
	fmt.Printf("Get returned: %s\n", content)
}
EOF

cd /tmp && go run memory_smoke_test.go
```

**Expected output:**
```
Search returned 1 result(s)
  [prefs.md L1] score=X.XXX: The user prefers Go over Python for backend services.
List returned 2 file(s)
Get returned: The user prefers Go over Python for backend services.
```
