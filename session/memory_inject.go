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
	b.WriteString("Hivemind maintains an IDE-wide persistent memory store across all sessions and projects.\n\n")

	b.WriteString("### Rules\n\n")
	b.WriteString("- **Before answering** any question about the user's preferences, setup, past decisions, or active projects: call `memory_search` first.\n")
	b.WriteString("- **After every session** where you learn something durable: call `memory_write` to persist it.\n")
	b.WriteString("- Write **stable facts** (hardware, OS, global preferences) to `global.md`. Write **dated notes** to the default `YYYY-MM-DD.md`.\n\n")

	b.WriteString("### What is worth writing to memory\n\n")
	b.WriteString("- User's OS, hardware, terminal and editor setup\n")
	b.WriteString("- API keys, services, and credentials configured\n")
	b.WriteString("- Project tech stack decisions and the reasoning behind them\n")
	b.WriteString("- Recurring patterns the user likes or dislikes\n")
	b.WriteString("- Anything you had to look up or figure out that the user will likely ask again\n\n")

	b.WriteString("### Tools\n\n")
	b.WriteString("| Tool | When to use |\n")
	b.WriteString("|------|-------------|\n")
	b.WriteString("| `memory_search(query)` | Start of session, before answering questions about prior context |\n")
	b.WriteString("| `memory_write(content, file?)` | When you discover something worth remembering |\n")
	b.WriteString("| `memory_get(path, from?, lines?)` | Read specific lines from a memory file |\n")
	b.WriteString("| `memory_list()` | Browse all memory files |\n\n")

	if len(results) > 0 {
		b.WriteString("### Relevant context for this session\n\n")
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
	if startIdx >= 0 && endIdx > startIdx {
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
