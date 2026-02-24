package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ByteMirror/hivemind/brain"
	"github.com/ByteMirror/hivemind/memory"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// handleMemoryInit spawns an agent to bootstrap memory from codebase analysis.
func handleMemoryInit(mgr *memory.Manager, client BrainClient, repoPath, instanceID string) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		Log("tool call: memory_init (instanceID=%s)", instanceID)

		title := fmt.Sprintf("memory-init-%d", time.Now().Unix())
		prompt := "Analyze this codebase and the user's environment. Use the memory tools to create organized memory files:\n" +
			"1. system/global.md — hardware, OS, shell, tools (use frontmatter: description: \"Hardware and OS info\")\n" +
			"2. system/conventions.md — code patterns, style, architecture decisions\n" +
			"3. A project-specific file with key decisions and tech stack\n\n" +
			"Use memory_tree first to see what already exists. Don't overwrite existing files — append or create new ones.\n" +
			"Use YAML frontmatter (---\\ndescription: ...\\n---) at the top of each file."

		skipPerms := true
		result, err := client.CreateInstance(repoPath, instanceID, brain.CreateInstanceParams{
			Title:           title,
			Prompt:          prompt,
			Role:            "architect",
			SkipPermissions: &skipPerms,
		})
		if err != nil {
			Log("memory_init error: %v", err)
			return gomcp.NewToolResultError("failed to spawn memory init agent: " + err.Error()), nil
		}

		data, _ := json.Marshal(result)
		Log("memory_init: spawned %s", title)
		return gomcp.NewToolResultText(fmt.Sprintf("Memory init agent spawned: %s\n%s", title, string(data))), nil
	}
}

// handleMemoryReflect spawns an agent to review recent memory changes and persist insights.
func handleMemoryReflect(mgr *memory.Manager, client BrainClient, repoPath, instanceID string) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		Log("tool call: memory_reflect (instanceID=%s)", instanceID)

		// Gather recent history to include in the prompt.
		var historySection string
		entries, err := mgr.History("", 20)
		if err == nil && len(entries) > 0 {
			var sb strings.Builder
			sb.WriteString("Recent memory changes:\n")
			for _, e := range entries {
				sb.WriteString(fmt.Sprintf("- [%s] %s (%s)\n", e.Date, e.Message, strings.Join(e.Files, ", ")))
			}
			historySection = sb.String()
		} else {
			historySection = "No recent history available."
		}

		title := fmt.Sprintf("memory-reflect-%d", time.Now().Unix())
		today := time.Now().Format("2006-01-02")
		prompt := fmt.Sprintf("Review the recent memory activity and write a reflection.\n\n%s\n\n"+
			"Instructions:\n"+
			"1. Use memory_tree and memory_search to understand the current state\n"+
			"2. Look for patterns, consolidate duplicates, identify gaps\n"+
			"3. Write a concise reflection to reflections/%s.md using memory_write\n"+
			"4. If you notice duplicate or contradictory information, use memory_move/memory_delete to clean up",
			historySection, today)

		skipPerms := true
		result, err := client.CreateInstance(repoPath, instanceID, brain.CreateInstanceParams{
			Title:           title,
			Prompt:          prompt,
			Role:            "reviewer",
			SkipPermissions: &skipPerms,
		})
		if err != nil {
			Log("memory_reflect error: %v", err)
			return gomcp.NewToolResultError("failed to spawn memory reflect agent: " + err.Error()), nil
		}

		data, _ := json.Marshal(result)
		Log("memory_reflect: spawned %s", title)
		return gomcp.NewToolResultText(fmt.Sprintf("Memory reflect agent spawned: %s\n%s", title, string(data))), nil
	}
}

// handleMemoryDefrag spawns an agent to reorganize aging memory files.
func handleMemoryDefrag(mgr *memory.Manager, client BrainClient, repoPath, instanceID string) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		Log("tool call: memory_defrag (instanceID=%s)", instanceID)

		// Gather file list to include in the prompt.
		var fileListSection string
		files, err := mgr.List()
		if err == nil && len(files) > 0 {
			var sb strings.Builder
			sb.WriteString("Current memory files:\n")
			for _, f := range files {
				sizeKB := float64(f.SizeBytes) / 1024.0
				sb.WriteString(fmt.Sprintf("- %s (%.1fK, %d chunks)\n", f.Path, sizeKB, f.ChunkCount))
			}
			fileListSection = sb.String()
		} else {
			fileListSection = "No memory files found."
		}

		title := fmt.Sprintf("memory-defrag-%d", time.Now().Unix())
		prompt := fmt.Sprintf("Reorganize the memory store for clarity and efficiency.\n\n%s\n\n"+
			"Instructions:\n"+
			"1. Use memory_tree to see the full structure with descriptions\n"+
			"2. Aim for 15-25 focused files organized into logical directories\n"+
			"3. Use memory_move to rename/reorganize files\n"+
			"4. Use memory_write to merge small related files\n"+
			"5. Use memory_delete to remove duplicates or obsolete content\n"+
			"6. Ensure every file has YAML frontmatter with a description\n"+
			"7. Pin important reference files to system/ using memory_pin\n"+
			"8. Do NOT delete system/ files unless creating better replacements",
			fileListSection)

		skipPerms := true
		result, err := client.CreateInstance(repoPath, instanceID, brain.CreateInstanceParams{
			Title:           title,
			Prompt:          prompt,
			Role:            "architect",
			SkipPermissions: &skipPerms,
		})
		if err != nil {
			Log("memory_defrag error: %v", err)
			return gomcp.NewToolResultError("failed to spawn memory defrag agent: " + err.Error()), nil
		}

		data, _ := json.Marshal(result)
		Log("memory_defrag: spawned %s", title)
		return gomcp.NewToolResultText(fmt.Sprintf("Memory defrag agent spawned: %s\n%s", title, string(data))), nil
	}
}
