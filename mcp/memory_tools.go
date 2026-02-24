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
// Backward compatible: no path = append to YYYY-MM-DD.md. With path = overwrite.
func handleMemoryWrite(mgr *memory.Manager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		Log("tool call: memory_write")
		content := req.GetString("content", "")
		if content == "" {
			return gomcp.NewToolResultError("missing required parameter: content"), nil
		}

		path := req.GetString("file", "")
		scope := req.GetString("scope", "")
		commitMsg := req.GetString("commit_message", "")

		// Resolve scope when no explicit file path is given.
		if path == "" && scope == "global" {
			path = "system/global.md"
		}

		if path != "" {
			// Create/overwrite specific file.
			if err := mgr.WriteFile(path, content, commitMsg); err != nil {
				Log("memory_write error: %v", err)
				return gomcp.NewToolResultError("failed to write memory: " + err.Error()), nil
			}
		} else {
			// Default: append to daily file (YYYY-MM-DD.md).
			if err := mgr.Write(content, ""); err != nil {
				Log("memory_write error: %v", err)
				return gomcp.NewToolResultError("failed to write memory: " + err.Error()), nil
			}
		}
		Log("memory_write: saved %d chars to %s", len(content), path)
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

// handleMemorySearch searches the IDE-wide memory store.
func handleMemorySearch(mgr *memory.Manager) mcpserver.ToolHandlerFunc {
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
		if args := req.GetArguments(); args != nil {
			if v, ok := args["from"].(float64); ok {
				from = int(v)
			}
			if v, ok := args["lines"].(float64); ok {
				lines = int(v)
			}
		}

		text, err := mgr.Get(relPath, from, lines)
		if err != nil {
			return gomcp.NewToolResultError("failed to read memory file: " + err.Error()), nil
		}
		return gomcp.NewToolResultText(text), nil
	}
}

// handleMemoryList lists all memory files (recursive).
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
		return gomcp.NewToolResultText(fmt.Sprintf("Moved %s → %s.", from, to)), nil
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
