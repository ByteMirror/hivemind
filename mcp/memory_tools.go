package mcp

import (
	"context"
	"encoding/json"

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
