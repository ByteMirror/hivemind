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

// InjectMemoryContext is the primary injection function called on agent start/resume.
// globalMgr must be non-nil; repoMgr may be nil if no repo memory dir exists yet.
// worktreePath is used to derive the repo name for the section heading and repo query.
func InjectMemoryContext(worktreePath string, globalMgr *memory.Manager, repoMgr *memory.Manager, count int) error {
	if globalMgr == nil {
		return nil
	}
	if count <= 0 {
		count = 5
	}

	// Gather tree.
	treeEntries, _ := globalMgr.Tree()

	// Gather system/ file contents.
	budget := getSystemBudget()
	systemFiles, _ := globalMgr.SystemFiles(budget)

	// Query global manager for cross-project / user-environment context.
	globalResults, err := globalMgr.Search(
		"global setup preferences environment hardware OS",
		memory.SearchOpts{MaxResults: count},
	)
	if err != nil {
		return fmt.Errorf("memory query (global) for CLAUDE.md: %w", err)
	}

	// Query repo manager for project-specific context (if available).
	var repoResults []memory.SearchResult
	if repoMgr != nil {
		repoSlug := filepath.Base(worktreePath)
		repoResults, err = repoMgr.Search(
			repoSlug+" project architecture decisions",
			memory.SearchOpts{MaxResults: count},
		)
		if err != nil {
			return fmt.Errorf("memory query (repo) for CLAUDE.md: %w", err)
		}
	}

	repoName := filepath.Base(worktreePath)
	section := buildMemorySection(treeEntries, systemFiles, globalResults, repoResults, repoName)

	claudeMDPath := filepath.Join(worktreePath, "CLAUDE.md")
	return upsertMemorySection(claudeMDPath, section)
}

// injectMemoryContext is the legacy unexported wrapper preserved for
// any internal callers that have not yet been migrated.
// Deprecated: use InjectMemoryContext directly.
func injectMemoryContext(worktreePath string, mgr *memory.Manager, count int) error {
	return InjectMemoryContext(worktreePath, mgr, nil, count)
}

// buildMemorySection formats the memory context into a Markdown section.
// It combines tree view, system files, and global/repo search results.
func buildMemorySection(tree []memory.TreeEntry, systemFiles map[string]string, globalResults []memory.SearchResult, repoResults []memory.SearchResult, repoName string) string {
	var b strings.Builder
	b.WriteString(memoryInjectHeader + "\n")
	b.WriteString("## Hivemind Memory\n\n")
	b.WriteString("Hivemind maintains an IDE-wide persistent memory store across all sessions and projects.\n\n")

	b.WriteString("### Rules\n\n")
	b.WriteString("- **Before answering** any question about the user's preferences, setup, past decisions, or active projects: call `memory_search` first.\n")
	b.WriteString("- **After completing any significant task** (implementing a feature, fixing a bug, making an architectural decision): call `memory_write` immediately. Do not wait to be asked.\n")
	b.WriteString("- **At the end of every working session**: call `memory_write` to record what was built, decisions made, and any user preferences observed. This is mandatory.\n")
	b.WriteString("- Write **stable facts** (hardware, OS, global preferences) to `global.md` using `scope=\"global\"`. Write **project decisions** with `scope=\"repo\"` (default for dated files).\n")
	b.WriteString("- **When asked to write memory**: Do it immediately without asking for confirmation. Call memory_write with a concise summary of: (1) what was built/changed, (2) key decisions made, (3) any user preferences expressed.\n\n")

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
	b.WriteString("| `memory_write(content, file?, scope?)` | scope=\"repo\" for this project's decisions; scope=\"global\" for user preferences/hardware |\n")
	b.WriteString("| `memory_get(path, from?, lines?)` | Read specific lines from a memory file |\n")
	b.WriteString("| `memory_list()` | Browse all memory files |\n\n")

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

	b.WriteString("### Global context\n\n")
	if len(globalResults) > 0 {
		for _, r := range globalResults {
			b.WriteString(fmt.Sprintf("**[%s L%d]** %s\n\n", r.Path, r.StartLine, r.Snippet))
		}
	} else {
		b.WriteString("*(no global memory yet)*\n\n")
	}

	b.WriteString(fmt.Sprintf("### Repo context (%s)\n\n", repoName))
	if len(repoResults) > 0 {
		for _, r := range repoResults {
			b.WriteString(fmt.Sprintf("**[%s L%d]** %s\n\n", r.Path, r.StartLine, r.Snippet))
		}
	} else {
		b.WriteString("*(no repo memory yet)*\n\n")
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
