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

	// Gather tree.
	treeEntries, _ := mgr.Tree()

	// Gather system/ file contents.
	budget := getSystemBudget()
	systemFiles, _ := mgr.SystemFiles(budget)

	// Query memory with a broad context query.
	query := filepath.Base(worktreePath) + " project setup preferences environment"
	results, err := mgr.Search(query, memory.SearchOpts{MaxResults: count})
	if err != nil {
		return fmt.Errorf("memory query for CLAUDE.md: %w", err)
	}

	section := buildMemorySection(treeEntries, systemFiles, results)

	claudeMDPath := filepath.Join(worktreePath, "CLAUDE.md")
	return upsertMemorySection(claudeMDPath, section)
}

// buildMemorySection formats the memory context into a Markdown section.
func buildMemorySection(tree []memory.TreeEntry, systemFiles map[string]string, results []memory.SearchResult) string {
	var b strings.Builder
	b.WriteString(memoryInjectHeader + "\n")
	b.WriteString("## Hivemind Memory\n\n")
	b.WriteString("Hivemind maintains an IDE-wide persistent memory store across all sessions and projects.\n\n")

	b.WriteString("### Rules\n\n")
	b.WriteString("- **Before answering** any question about the user's preferences, setup, past decisions, or active projects: call `memory_search` first.\n")
	b.WriteString("- **After every session** where you learn something durable: call `memory_write` to persist it.\n")
	b.WriteString("- Write **stable facts** (hardware, OS, global preferences) with `scope=\"global\"` (or to `global.md`). Write **project decisions** with `scope=\"repo\"` (dated files default to repo).\n")
	b.WriteString("- **When asked to write memory at session end**: Do it immediately. Call memory_write with a concise summary of: (1) what was built/changed, (2) key decisions made, (3) any user preferences expressed.\n\n")

	b.WriteString("### Tools\n\n")
	b.WriteString("| Tool | When to use |\n")
	b.WriteString("|------|-------------|\n")
	b.WriteString("| `memory_search(query)` | Start of session, before answering questions about prior context |\n")
	b.WriteString("| `memory_write(content, file?, scope?)` | scope=\"repo\" for this project's decisions; scope=\"global\" for user preferences/hardware |\n")
	b.WriteString("| `memory_read(path)` | Read full file body (frontmatter stripped) |\n")
	b.WriteString("| `memory_append(path, content)` | Append content to an existing memory file |\n")
	b.WriteString("| `memory_get(path, from?, lines?)` | Read specific lines from a memory file |\n")
	b.WriteString("| `memory_list()` | Browse all memory files |\n")
	b.WriteString("| `memory_tree()` | View memory file tree with descriptions |\n")
	b.WriteString("| `memory_move(from, to)` | Rename or reorganize a memory file |\n")
	b.WriteString("| `memory_delete(path)` | Remove a memory file |\n")
	b.WriteString("| `memory_pin(path)` | Move file to system/ (always-in-context) |\n")
	b.WriteString("| `memory_unpin(path)` | Move file out of system/ |\n")
	b.WriteString("| `memory_history(path?, count?)` | View git history of memory changes |\n\n")

	// Memory tree.
	if len(tree) > 0 {
		b.WriteString("### Memory Tree\n\n")
		b.WriteString("```\n")
		b.WriteString(memory.FormatTree(tree))
		b.WriteString("```\n\n")
	}

	// System context (full contents of system/ files).
	if len(systemFiles) > 0 {
		b.WriteString("### System Context\n\n")
		for path, body := range systemFiles {
			b.WriteString(fmt.Sprintf("**%s:**\n\n", path))
			b.WriteString(strings.TrimSpace(body))
			b.WriteString("\n\n")
		}
	}

	// Search result snippets.
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
