package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/ByteMirror/hivemind/brain"
	"github.com/ByteMirror/hivemind/config"
	hivemindmcp "github.com/ByteMirror/hivemind/mcp"
	"github.com/ByteMirror/hivemind/memory"
)

func main() {
	hivemindDir := os.Getenv("HIVEMIND_DIR")
	if hivemindDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "hivemind-mcp: failed to get home directory: %v\n", err)
			os.Exit(1)
		}
		hivemindDir = filepath.Join(homeDir, ".hivemind")
	}

	// Set up file logging — stdout is the MCP protocol, stderr is captured by the client.
	if err := os.MkdirAll(hivemindDir, 0700); err == nil {
		logPath := filepath.Join(hivemindDir, "mcp-server.log")
		if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600); err == nil {
			logger := log.New(f, "[mcp] ", log.Ldate|log.Ltime|log.Lshortfile)
			hivemindmcp.SetLogger(logger)
			defer f.Close()
		}
	}

	instanceID := os.Getenv("HIVEMIND_INSTANCE_ID")
	repoPath := os.Getenv("HIVEMIND_REPO_PATH")

	// Default to Tier 2 (read + self-introspection). Tier 3 (write) is opt-in via env.
	tier := 2
	if os.Getenv("HIVEMIND_TIER") == "3" {
		tier = 3
	}

	// Try socket-based brain client first, fall back to file-based.
	socketPath := filepath.Join(hivemindDir, "hivemind.sock")
	var brainClient hivemindmcp.BrainClient
	socketClient := brain.NewClient(socketPath)
	if err := socketClient.Ping(); err == nil {
		hivemindmcp.Log("brain: using socket client (%s)", socketPath)
		brainClient = socketClient
	} else {
		hivemindmcp.Log("brain: socket unavailable (%v), using file fallback", err)
		brainClient = hivemindmcp.NewFileBrainClient(hivemindDir)
	}

	// Initialize memory manager from config (nil if memory is disabled).
	cfg := config.LoadConfig()
	gitEnabled := memory.GitEnabledFromConfig(cfg)
	var memMgr *memory.Manager
	memMgr, err := memory.NewManagerFromConfig(cfg)
	if err != nil {
		hivemindmcp.Log("memory init failed: %v", err)
		memMgr = nil
	} else if memMgr != nil {
		stop, stopErr := memMgr.StartWatcher()
		if stopErr != nil {
			hivemindmcp.Log("memory watcher failed: %v", stopErr)
		} else {
			defer stop()
		}
	}

	// Create repo-scoped memory manager when memory is enabled and a repo path is known.
	var repoMemMgr *memory.Manager
	var legacyRepoMemMgr *memory.Manager
	if memMgr != nil && repoPath != "" {
		worktreePath, _ := os.Getwd()
		resolution, resErr := memory.ResolveRepoStorePaths(memMgr.Dir(), repoPath, worktreePath)
		if resErr != nil {
			hivemindmcp.Log("repo memory path resolution failed: %v", resErr)
		} else {
			if resolution.CanonicalPath != "" {
				if rMgr, rErr := memory.NewManagerWithOptions(resolution.CanonicalPath, nil, memory.ManagerOptions{GitEnabled: gitEnabled}); rErr != nil {
					hivemindmcp.Log("repo memory init failed for %q: %v", resolution.CanonicalSlug, rErr)
				} else {
					repoMemMgr = rMgr
					defer repoMemMgr.Close()
				}
			}
			if resolution.LegacyPath != "" {
				if lMgr, lErr := memory.NewManagerWithOptions(resolution.LegacyPath, nil, memory.ManagerOptions{GitEnabled: gitEnabled}); lErr != nil {
					hivemindmcp.Log("legacy repo memory init failed for %q: %v", resolution.LegacySlug, lErr)
				} else {
					legacyRepoMemMgr = lMgr
					defer legacyRepoMemMgr.Close()
				}
			}
		}
	}

	hivemindmcp.Log("starting: hivemindDir=%s instanceID=%s repoPath=%s tier=%d", hivemindDir, instanceID, repoPath, tier)

	srv := hivemindmcp.NewHivemindMCPServer(brainClient, hivemindDir, instanceID, repoPath, tier, memMgr, repoMemMgr, legacyRepoMemMgr)
	if err := srv.Serve(); err != nil {
		hivemindmcp.Log("fatal: %v", err)
		fmt.Fprintf(os.Stderr, "hivemind-mcp: %v\n", err)
		os.Exit(1)
	}

	hivemindmcp.Log("shutdown cleanly")
}
