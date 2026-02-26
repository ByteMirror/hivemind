package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ByteMirror/hivemind/memory"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleMemoryWrite_UsesCommitMessage(t *testing.T) {
	mgr, err := memory.NewManager(t.TempDir(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { mgr.Close() })

	handler := handleMemoryWrite(mgr, nil)
	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"content":        "remember this",
		"file":           "notes.md",
		"scope":          "global",
		"commit_message": "custom commit message",
	}

	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, resultText(t, result), "Memory saved.")

	entries, err := mgr.History("", 10)
	require.NoError(t, err)
	require.NotEmpty(t, entries)
	assert.Equal(t, "custom commit message", entries[0].Message)
}

func TestHandleMemoryHistory_ReturnsNotAvailableWhenGitDisabled(t *testing.T) {
	mgr, err := memory.NewManagerWithOptions(t.TempDir(), nil, memory.ManagerOptions{GitEnabled: false})
	require.NoError(t, err)
	t.Cleanup(func() { mgr.Close() })

	handler := handleMemoryHistory(mgr, nil, nil)
	req := gomcp.CallToolRequest{}

	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, resultText(t, result), "Git versioning not available for memory.")
}

func TestHandleMemoryHistory_ScopeFilteringAndScopeField(t *testing.T) {
	globalMgr, err := memory.NewManager(t.TempDir(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { globalMgr.Close() })
	repoMgr, err := memory.NewManager(t.TempDir(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { repoMgr.Close() })
	legacyRepoMgr, err := memory.NewManager(t.TempDir(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { legacyRepoMgr.Close() })

	require.NoError(t, globalMgr.Write("global note", "system/global.md"))
	require.NoError(t, repoMgr.Write("repo note", "2026-02-26.md"))
	require.NoError(t, legacyRepoMgr.Write("legacy repo note", "2026-02-25.md"))

	handler := handleMemoryHistory(globalMgr, repoMgr, legacyRepoMgr)

	allReq := gomcp.CallToolRequest{}
	allReq.Params.Arguments = map[string]interface{}{"scope": "all", "count": 20.0}
	allResult, err := handler(context.Background(), allReq)
	require.NoError(t, err)
	assert.False(t, allResult.IsError)

	var allEntries []scopedGitLogEntry
	require.NoError(t, json.Unmarshal([]byte(resultText(t, allResult)), &allEntries))
	require.NotEmpty(t, allEntries)
	hasGlobal := false
	hasRepo := false
	for _, e := range allEntries {
		if e.Scope == "global" {
			hasGlobal = true
		}
		if e.Scope == "repo" {
			hasRepo = true
		}
	}
	assert.True(t, hasGlobal)
	assert.True(t, hasRepo)

	repoReq := gomcp.CallToolRequest{}
	repoReq.Params.Arguments = map[string]interface{}{"scope": "repo", "count": 20.0}
	repoResult, err := handler(context.Background(), repoReq)
	require.NoError(t, err)
	var repoEntries []scopedGitLogEntry
	require.NoError(t, json.Unmarshal([]byte(resultText(t, repoResult)), &repoEntries))
	require.NotEmpty(t, repoEntries)
	for _, e := range repoEntries {
		assert.Equal(t, "repo", e.Scope)
	}

	globalReq := gomcp.CallToolRequest{}
	globalReq.Params.Arguments = map[string]interface{}{"scope": "global", "count": 20.0}
	globalResult, err := handler(context.Background(), globalReq)
	require.NoError(t, err)
	var globalEntries []scopedGitLogEntry
	require.NoError(t, json.Unmarshal([]byte(resultText(t, globalResult)), &globalEntries))
	require.NotEmpty(t, globalEntries)
	for _, e := range globalEntries {
		assert.Equal(t, "global", e.Scope)
	}
}

func TestHandleMemoryWrite_WithBranchDoesNotTouchDefault(t *testing.T) {
	mgr, err := memory.NewManager(t.TempDir(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { mgr.Close() })
	require.NoError(t, mgr.WriteWithCommitMessageOnBranch("default note", "notes.md", "default", ""))
	require.NoError(t, mgr.CreateBranch("feature/mcp", ""))

	handler := handleMemoryWrite(mgr, nil)
	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"content": "branch-only note",
		"file":    "notes.md",
		"branch":  "feature/mcp",
		"scope":   "global",
	}

	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	defaultBody, err := mgr.Read("notes.md")
	require.NoError(t, err)
	assert.NotContains(t, defaultBody, "branch-only note")

	branchBody, err := mgr.ReadAtRef("notes.md", "feature/mcp")
	require.NoError(t, err)
	assert.Contains(t, branchBody, "branch-only note")
}

func TestHandleMemoryBranches_ListsBranchInfo(t *testing.T) {
	mgr, err := memory.NewManager(t.TempDir(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { mgr.Close() })
	require.NoError(t, mgr.Write("x", "notes.md"))
	require.NoError(t, mgr.CreateBranch("feature/one", ""))

	handler := handleMemoryBranches(mgr, nil, nil)
	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"scope": "global"}

	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	var rows []memoryBranchState
	require.NoError(t, json.Unmarshal([]byte(resultText(t, result)), &rows))
	require.NotEmpty(t, rows)
	assert.True(t, rows[0].GitAvailable)
	assert.Contains(t, rows[0].Branches, "feature/one")
}

func TestHandleMemoryDiff_ReturnsDiff(t *testing.T) {
	mgr, err := memory.NewManager(t.TempDir(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { mgr.Close() })

	require.NoError(t, mgr.Write("one", "diff.md"))
	entries, err := mgr.History("", 10)
	require.NoError(t, err)
	require.NotEmpty(t, entries)
	base := entries[0].SHA

	require.NoError(t, mgr.Write("two", "diff.md"))
	entries, err = mgr.History("", 10)
	require.NoError(t, err)
	require.NotEmpty(t, entries)
	head := entries[0].SHA

	handler := handleMemoryDiff(mgr, nil, nil)
	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"base_ref": base,
		"head_ref": head,
		"path":     "diff.md",
		"scope":    "global",
	}

	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, resultText(t, result), "diff --git")
}
