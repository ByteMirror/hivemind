package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ByteMirror/hivemind/memory"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// datedFileRe matches YYYY-MM-DD.md filenames.
var datedFileRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}\.md$`)

// resolveMemoryScope returns the Manager to use for a write operation.
// scope may be "global", "repo", or "" (auto-detect from filename).
// repoMgr may be nil if no repo-specific manager is available.
func resolveMemoryScope(scope, file string, globalMgr, repoMgr *memory.Manager) *memory.Manager {
	switch strings.ToLower(scope) {
	case "global":
		return globalMgr
	case "repo":
		if repoMgr != nil {
			return repoMgr
		}
		return globalMgr
	default:
		// Auto-detect: dated files (YYYY-MM-DD.md or empty -> today's date) -> repo;
		// named files (global.md, MEMORY.md, etc.) -> global.
		if file == "" || datedFileRe.MatchString(filepath.Base(file)) {
			if repoMgr != nil {
				return repoMgr
			}
		}
		return globalMgr
	}
}

// handleMemoryWrite saves a fact to the IDE-wide memory store.
// scope="global" writes to globalMgr (defaulting to system/global.md); scope="repo" (or dated filenames) writes to repoMgr.
func handleMemoryWrite(globalMgr *memory.Manager, repoMgr *memory.Manager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		Log("tool call: memory_write")
		content := req.GetString("content", "")
		file := req.GetString("file", "")
		scope := req.GetString("scope", "")
		if content == "" {
			return gomcp.NewToolResultError("missing required parameter: content"), nil
		}

		// When scope is global and no explicit file, default to system/global.md
		// so it's always injected into agent context.
		mgr := resolveMemoryScope(scope, file, globalMgr, repoMgr)
		if file == "" && strings.ToLower(scope) == "global" {
			file = "system/global.md"
		}

		if err := mgr.Write(content, file); err != nil {
			Log("memory_write error: %v", err)
			return gomcp.NewToolResultError("failed to write memory: " + err.Error()), nil
		}
		Log("memory_write: saved %d chars scope=%q file=%q", len(content), scope, file)
		return gomcp.NewToolResultText("Memory saved."), nil
	}
}

// handleMemoryRead reads a full memory file body (frontmatter stripped).
func handleMemoryRead(mgr *memory.Manager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		Log("tool call: memory_read")
		relPath := req.GetString("path", "")
		if relPath == "" {
			return gomcp.NewToolResultError("missing required parameter: path"), nil
		}
		body, err := mgr.Read(relPath)
		if err != nil {
			return gomcp.NewToolResultError("failed to read memory file: " + err.Error()), nil
		}
		return gomcp.NewToolResultText(body), nil
	}
}

// handleMemorySearch searches both the global and (if non-nil) repo memory managers.
// Results from both are merged, deduplicated by Path+StartLine, sorted by Score, and
// trimmed to maxResults.
func handleMemorySearch(globalMgr *memory.Manager, repoMgr *memory.Manager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		Log("tool call: memory_search")
		query := req.GetString("query", "")
		if query == "" {
			return gomcp.NewToolResultError("missing required parameter: query"), nil
		}

		maxResults := 10
		if args := req.GetArguments(); args != nil {
			if v, ok := args["max_results"].(float64); ok {
				maxResults = int(v)
			}
		}

		// Search global manager.
		globalResults, err := globalMgr.Search(query, memory.SearchOpts{MaxResults: maxResults})
		if err != nil {
			Log("memory_search global error: %v", err)
			return gomcp.NewToolResultError("search failed: " + err.Error()), nil
		}

		combined := globalResults

		// Search repo manager if available.
		if repoMgr != nil {
			repoResults, err := repoMgr.Search(query, memory.SearchOpts{MaxResults: maxResults})
			if err != nil {
				Log("memory_search repo error: %v", err)
				// Non-fatal: return global results only.
			} else {
				// Merge, deduplicating by Path+StartLine.
				seen := make(map[string]struct{}, len(combined))
				for _, r := range combined {
					seen[fmt.Sprintf("%s\x00%d", r.Path, r.StartLine)] = struct{}{}
				}
				for _, r := range repoResults {
					key := fmt.Sprintf("%s\x00%d", r.Path, r.StartLine)
					if _, ok := seen[key]; !ok {
						seen[key] = struct{}{}
						combined = append(combined, r)
					}
				}
			}
		}

		// Sort merged results by Score descending.
		sort.Slice(combined, func(i, j int) bool {
			return combined[i].Score > combined[j].Score
		})
		if len(combined) > maxResults {
			combined = combined[:maxResults]
		}

		data, _ := json.MarshalIndent(combined, "", "  ")
		Log("memory_search: query=%q results=%d", query, len(combined))
		return gomcp.NewToolResultText(string(data)), nil
	}
}

// handleMemoryGet reads specific lines from a memory file.
// It first tries the global manager's dir; if the file is not found there, it tries the
// repo manager's dir. The path may optionally include a "repos/{slug}/" prefix.
func handleMemoryGet(globalMgr *memory.Manager, repoMgr *memory.Manager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		Log("tool call: memory_get")
		relPath := req.GetString("path", "")
		if relPath == "" {
			return gomcp.NewToolResultError("missing required parameter: path"), nil
		}

		var from, lines int
		if args := req.GetArguments(); args != nil {
			if v, ok := args["from"].(float64); ok {
				from = int(v)
			}
			if v, ok := args["lines"].(float64); ok {
				lines = int(v)
			}
		}

		// Try global manager first.
		text, err := globalMgr.Get(relPath, from, lines)
		if err == nil {
			return gomcp.NewToolResultText(text), nil
		}

		// If not found in global, try repo manager.
		if repoMgr != nil && errors.Is(err, os.ErrNotExist) {
			repoText, repoErr := repoMgr.Get(relPath, from, lines)
			if repoErr == nil {
				return gomcp.NewToolResultText(repoText), nil
			}
			// Return original error if repo also failed.
		}

		return gomcp.NewToolResultError("failed to read memory file: " + err.Error()), nil
	}
}

// handleMemoryList lists all memory files from the global manager and (if non-nil) the
// repo manager, returning a combined result.
func handleMemoryList(globalMgr *memory.Manager, repoMgr *memory.Manager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		Log("tool call: memory_list")
		files, err := globalMgr.List()
		if err != nil {
			return gomcp.NewToolResultError("failed to list memory: " + err.Error()), nil
		}

		if repoMgr != nil {
			repoFiles, repoErr := repoMgr.List()
			if repoErr != nil {
				Log("memory_list repo error: %v", repoErr)
				// Non-fatal: return global files only.
			} else {
				files = append(files, repoFiles...)
			}
		}

		data, _ := json.MarshalIndent(files, "", "  ")
		Log("memory_list: %d files", len(files))
		return gomcp.NewToolResultText(string(data)), nil
	}
}

// handleMemoryTree returns the memory file tree with descriptions.
func handleMemoryTree(mgr *memory.Manager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		Log("tool call: memory_tree")
		entries, err := mgr.Tree()
		if err != nil {
			return gomcp.NewToolResultError("failed to get memory tree: " + err.Error()), nil
		}
		data, _ := json.MarshalIndent(entries, "", "  ")
		Log("memory_tree: %d entries", len(entries))
		return gomcp.NewToolResultText(string(data)), nil
	}
}

// handleMemoryHistory returns git history for a memory file or all files.
func handleMemoryHistory(mgr *memory.Manager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		Log("tool call: memory_history")
		relPath := req.GetString("path", "")
		count := 10
		if args := req.GetArguments(); args != nil {
			if v, ok := args["count"].(float64); ok {
				count = int(v)
			}
		}
		entries, err := mgr.History(relPath, count)
		if err != nil {
			return gomcp.NewToolResultError("failed to get history: " + err.Error()), nil
		}
		if entries == nil {
			return gomcp.NewToolResultText("Git versioning not available for memory."), nil
		}
		data, _ := json.MarshalIndent(entries, "", "  ")
		Log("memory_history: path=%q entries=%d", relPath, len(entries))
		return gomcp.NewToolResultText(string(data)), nil
	}
}

// handleMemoryAppend appends content to a memory file.
func handleMemoryAppend(mgr *memory.Manager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		Log("tool call: memory_append")
		relPath := req.GetString("path", "")
		content := req.GetString("content", "")
		if relPath == "" {
			return gomcp.NewToolResultError("missing required parameter: path"), nil
		}
		if content == "" {
			return gomcp.NewToolResultError("missing required parameter: content"), nil
		}
		if err := mgr.Append(relPath, content); err != nil {
			Log("memory_append error: %v", err)
			return gomcp.NewToolResultError("failed to append: " + err.Error()), nil
		}
		return gomcp.NewToolResultText(fmt.Sprintf("Appended %d chars to %s.", len(content), relPath)), nil
	}
}

// handleMemoryMove renames/moves a memory file.
func handleMemoryMove(mgr *memory.Manager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		Log("tool call: memory_move")
		from := req.GetString("from", "")
		to := req.GetString("to", "")
		if from == "" || to == "" {
			return gomcp.NewToolResultError("missing required parameters: from, to"), nil
		}
		if err := mgr.Move(from, to); err != nil {
			return gomcp.NewToolResultError("failed to move: " + err.Error()), nil
		}
		return gomcp.NewToolResultText(fmt.Sprintf("Moved %s -> %s.", from, to)), nil
	}
}

// handleMemoryDelete removes a memory file.
func handleMemoryDelete(mgr *memory.Manager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		Log("tool call: memory_delete")
		relPath := req.GetString("path", "")
		if relPath == "" {
			return gomcp.NewToolResultError("missing required parameter: path"), nil
		}
		if err := mgr.Delete(relPath); err != nil {
			return gomcp.NewToolResultError("failed to delete: " + err.Error()), nil
		}
		return gomcp.NewToolResultText(fmt.Sprintf("Deleted %s.", relPath)), nil
	}
}

// handleMemoryPin moves a file to system/ (always-in-context).
func handleMemoryPin(mgr *memory.Manager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		Log("tool call: memory_pin")
		relPath := req.GetString("path", "")
		if relPath == "" {
			return gomcp.NewToolResultError("missing required parameter: path"), nil
		}
		if err := mgr.Pin(relPath); err != nil {
			return gomcp.NewToolResultError("failed to pin: " + err.Error()), nil
		}
		return gomcp.NewToolResultText(fmt.Sprintf("Pinned %s to system/.", relPath)), nil
	}
}

// handleMemoryUnpin moves a file out of system/.
func handleMemoryUnpin(mgr *memory.Manager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		Log("tool call: memory_unpin")
		relPath := req.GetString("path", "")
		if relPath == "" {
			return gomcp.NewToolResultError("missing required parameter: path"), nil
		}
		if err := mgr.Unpin(relPath); err != nil {
			return gomcp.NewToolResultError("failed to unpin: " + err.Error()), nil
		}
		return gomcp.NewToolResultText(fmt.Sprintf("Unpinned %s from system/.", relPath)), nil
	}
}
