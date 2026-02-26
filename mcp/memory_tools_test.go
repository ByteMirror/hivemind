package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ByteMirror/hivemind/memory"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsRefLookupError_NotValidObjectName(t *testing.T) {
	assert.True(t, isRefLookupError(errors.New("fatal: Not a valid object name feature/missing")))
}

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

func TestHandleMemoryRead_BarePathFallsBackToRepo(t *testing.T) {
	globalMgr, err := memory.NewManager(t.TempDir(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { globalMgr.Close() })

	repoMgr, err := memory.NewManager(t.TempDir(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { repoMgr.Close() })

	require.NoError(t, repoMgr.Write("repo-only body", "smoke.md"))

	handler := handleMemoryRead(globalMgr, repoMgr, nil)
	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"path": "smoke.md"}

	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, resultText(t, result), "repo-only body")
}

func TestHandleMemoryAppend_BarePathFallsBackToRepoBranch(t *testing.T) {
	globalMgr, err := memory.NewManager(t.TempDir(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { globalMgr.Close() })

	repoMgr, err := memory.NewManager(t.TempDir(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { repoMgr.Close() })

	require.NoError(t, repoMgr.Write("base", "smoke.md"))
	require.NoError(t, repoMgr.CreateBranch("feature/smoke", ""))

	handler := handleMemoryAppend(globalMgr, repoMgr, nil)
	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"path":    "smoke.md",
		"content": "branch: appended line",
		"branch":  "feature/smoke",
	}

	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	branchBody, err := repoMgr.ReadAtRef("smoke.md", "feature/smoke")
	require.NoError(t, err)
	assert.Contains(t, branchBody, "branch: appended line")

	mainBody, err := repoMgr.Read("smoke.md")
	require.NoError(t, err)
	assert.NotContains(t, mainBody, "branch: appended line")
}

func TestHandleMemoryListAndTree_RefFallsThroughWhenGlobalRefMissing(t *testing.T) {
	globalMgr, err := memory.NewManager(t.TempDir(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { globalMgr.Close() })

	repoMgr, err := memory.NewManager(t.TempDir(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { repoMgr.Close() })

	require.NoError(t, repoMgr.Write("repo-only body", "smoke.md"))
	require.NoError(t, repoMgr.CreateBranch("feature/ref", ""))

	listHandler := handleMemoryList(globalMgr, repoMgr, nil)
	listReq := gomcp.CallToolRequest{}
	listReq.Params.Arguments = map[string]interface{}{"ref": "feature/ref"}

	listResult, err := listHandler(context.Background(), listReq)
	require.NoError(t, err)
	assert.False(t, listResult.IsError)
	assert.Contains(t, resultText(t, listResult), "smoke.md")

	treeHandler := handleMemoryTree(globalMgr, repoMgr, nil)
	treeReq := gomcp.CallToolRequest{}
	treeReq.Params.Arguments = map[string]interface{}{"ref": "feature/ref"}

	treeResult, err := treeHandler(context.Background(), treeReq)
	require.NoError(t, err)
	assert.False(t, treeResult.IsError)
	assert.Contains(t, resultText(t, treeResult), "smoke.md")
}

func TestHandleMemoryDiff_RespectsScopeForBarePath(t *testing.T) {
	globalMgr, err := memory.NewManager(t.TempDir(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { globalMgr.Close() })

	repoMgr, err := memory.NewManager(t.TempDir(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { repoMgr.Close() })

	require.NoError(t, repoMgr.Write("one", "smoke.md"))
	info, err := repoMgr.GitBranchInfo()
	require.NoError(t, err)
	require.NotEmpty(t, info.Default)

	const branchName = "feature/smoke"
	require.NoError(t, repoMgr.CreateBranch(branchName, info.Default))
	require.NoError(t, repoMgr.WriteWithCommitMessageOnBranch("two", "smoke.md", "branch edit", branchName))

	handler := handleMemoryDiff(globalMgr, repoMgr, nil)
	req := gomcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"base_ref": info.Default,
		"head_ref": branchName,
		"path":     "smoke.md",
		"scope":    "repo",
	}

	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, resultText(t, result), "diff --git")
	assert.Contains(t, resultText(t, result), "+two")
}
