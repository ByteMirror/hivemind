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
