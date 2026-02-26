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
	"time"

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

func resolveScopedManager(scope string, globalMgr, repoMgr *memory.Manager) *memory.Manager {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "global":
		return globalMgr
	case "repo":
		if repoMgr != nil {
			return repoMgr
		}
		return globalMgr
	default:
		if repoMgr != nil {
			return repoMgr
		}
		return globalMgr
	}
}

func repoSlugFromManager(mgr *memory.Manager) string {
	if mgr == nil {
		return ""
	}
	return filepath.Base(filepath.Clean(mgr.Dir()))
}

func parseRepoMemoryPath(path string) (slug, rel string, ok bool) {
	clean := filepath.ToSlash(filepath.Clean(path))
	if !strings.HasPrefix(clean, "repos/") {
		return "", "", false
	}
	rest := strings.TrimPrefix(clean, "repos/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func routePathToManager(path string, globalMgr, repoMgr, legacyRepoMgr *memory.Manager) (mgr *memory.Manager, relPath string) {
	slug, rel, ok := parseRepoMemoryPath(path)
	if !ok {
		return globalMgr, path
	}
	if repoMgr != nil && slug == repoSlugFromManager(repoMgr) {
		return repoMgr, rel
	}
	if legacyRepoMgr != nil && slug == repoSlugFromManager(legacyRepoMgr) {
		return legacyRepoMgr, rel
	}
	return globalMgr, path
}

func isRefLookupError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "unknown revision") ||
		strings.Contains(s, "bad revision") ||
		strings.Contains(s, "not in the working tree") ||
		strings.Contains(s, "ambiguous argument")
}

func parseOptionalIntArg(req gomcp.CallToolRequest, key string, fallback int) int {
	if args := req.GetArguments(); args != nil {
		if v, ok := args[key].(float64); ok {
			return int(v)
		}
	}
	return fallback
}

func parseOptionalBoolArg(req gomcp.CallToolRequest, key string, fallback bool) bool {
	if args := req.GetArguments(); args != nil {
		if v, ok := args[key].(bool); ok {
			return v
		}
	}
	return fallback
}

// handleMemoryWrite saves a fact to the IDE-wide memory store.
// scope="global" writes to globalMgr (defaulting to system/global.md); scope="repo" (or dated filenames) writes to repoMgr.
func handleMemoryWrite(globalMgr *memory.Manager, repoMgr *memory.Manager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		Log("tool call: memory_write")
		content := req.GetString("content", "")
		file := req.GetString("file", "")
		scope := req.GetString("scope", "")
		commitMessage := req.GetString("commit_message", "")
		branch := req.GetString("branch", "")
		if content == "" {
			return gomcp.NewToolResultError("missing required parameter: content"), nil
		}

		mgr := resolveMemoryScope(scope, file, globalMgr, repoMgr)
		if file == "" && strings.ToLower(scope) == "global" {
			file = "system/global.md"
		}

		if err := mgr.WriteWithCommitMessageOnBranch(content, file, commitMessage, branch); err != nil {
			Log("memory_write error: %v", err)
			return gomcp.NewToolResultError("failed to write memory: " + err.Error()), nil
		}
		Log("memory_write: saved %d chars scope=%q file=%q branch=%q", len(content), scope, file, branch)
		return gomcp.NewToolResultText("Memory saved."), nil
	}
}

// handleMemoryRead reads a full memory file body (frontmatter stripped).
func handleMemoryRead(globalMgr *memory.Manager, repoMgr *memory.Manager, legacyRepoMgr *memory.Manager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		Log("tool call: memory_read")
		relPath := req.GetString("path", "")
		if relPath == "" {
			return gomcp.NewToolResultError("missing required parameter: path"), nil
		}
		ref := req.GetString("ref", "")
		mgr, scopedPath := routePathToManager(relPath, globalMgr, repoMgr, legacyRepoMgr)

		var (
			body string
			err  error
		)
		if ref == "" {
			body, err = mgr.Read(scopedPath)
		} else {
			body, err = mgr.ReadAtRef(scopedPath, ref)
		}
		if err != nil {
			return gomcp.NewToolResultError("failed to read memory file: " + err.Error()), nil
		}
		return gomcp.NewToolResultText(body), nil
	}
}

// handleMemorySearch searches global and repo memory managers (canonical + legacy).
// Results are merged, deduplicated by scope+path+start_line, sorted by Score, and
// trimmed to maxResults.
func handleMemorySearch(globalMgr *memory.Manager, repoMgr *memory.Manager, legacyRepoMgr *memory.Manager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		Log("tool call: memory_search")
		query := req.GetString("query", "")
		if query == "" {
			return gomcp.NewToolResultError("missing required parameter: query"), nil
		}

		maxResults := parseOptionalIntArg(req, "max_results", 10)

		globalResults, err := globalMgr.Search(query, memory.SearchOpts{MaxResults: maxResults})
		if err != nil {
			Log("memory_search global error: %v", err)
			return gomcp.NewToolResultError("search failed: " + err.Error()), nil
		}

		var combined []memory.SearchResult
		seen := make(map[string]struct{}, len(globalResults))

		addResults := func(scope string, results []memory.SearchResult) {
			for _, r := range results {
				key := fmt.Sprintf("%s\x00%s\x00%d", scope, r.Path, r.StartLine)
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				combined = append(combined, r)
			}
		}

		addResults("global", globalResults)

		if repoMgr != nil {
			repoResults, repoErr := repoMgr.Search(query, memory.SearchOpts{MaxResults: maxResults})
			if repoErr != nil {
				Log("memory_search repo error: %v", repoErr)
			} else {
				addResults("repo", repoResults)
			}
		}

		if legacyRepoMgr != nil {
			legacyResults, legacyErr := legacyRepoMgr.Search(query, memory.SearchOpts{MaxResults: maxResults})
			if legacyErr != nil {
				Log("memory_search legacy repo error: %v", legacyErr)
			} else {
				addResults("repo", legacyResults)
			}
		}

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
func handleMemoryGet(globalMgr *memory.Manager, repoMgr *memory.Manager, legacyRepoMgr *memory.Manager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		Log("tool call: memory_get")
		relPath := req.GetString("path", "")
		if relPath == "" {
			return gomcp.NewToolResultError("missing required parameter: path"), nil
		}
		from := parseOptionalIntArg(req, "from", 0)
		lines := parseOptionalIntArg(req, "lines", 0)
		ref := req.GetString("ref", "")

		mgr, scopedPath := routePathToManager(relPath, globalMgr, repoMgr, legacyRepoMgr)
		var (
			text string
			err  error
		)
		if ref == "" {
			text, err = mgr.Get(scopedPath, from, lines)
		} else {
			text, err = mgr.GetAtRef(scopedPath, from, lines, ref)
		}
		if err == nil {
			return gomcp.NewToolResultText(text), nil
		}

		if mgr == globalMgr && errors.Is(err, os.ErrNotExist) {
			if repoMgr != nil {
				if ref == "" {
					text, err = repoMgr.Get(relPath, from, lines)
				} else {
					text, err = repoMgr.GetAtRef(relPath, from, lines, ref)
				}
				if err == nil {
					return gomcp.NewToolResultText(text), nil
				}
			}
			if legacyRepoMgr != nil {
				if ref == "" {
					text, err = legacyRepoMgr.Get(relPath, from, lines)
				} else {
					text, err = legacyRepoMgr.GetAtRef(relPath, from, lines, ref)
				}
				if err == nil {
					return gomcp.NewToolResultText(text), nil
				}
			}
		}

		return gomcp.NewToolResultError("failed to read memory file: " + err.Error()), nil
	}
}

// handleMemoryList lists all memory files from the global manager and (if non-nil) the
// canonical/legacy repo managers, returning a combined deduplicated result.
func handleMemoryList(globalMgr *memory.Manager, repoMgr *memory.Manager, legacyRepoMgr *memory.Manager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		Log("tool call: memory_list")
		ref := req.GetString("ref", "")

		listFromMgr := func(mgr *memory.Manager) ([]memory.FileInfo, error) {
			if mgr == nil {
				return nil, nil
			}
			if ref == "" {
				return mgr.List()
			}
			return mgr.ListAtRef(ref)
		}

		files, err := listFromMgr(globalMgr)
		if err != nil && !(ref != "" && isRefLookupError(err)) {
			return gomcp.NewToolResultError("failed to list memory: " + err.Error()), nil
		}
		if err != nil {
			files = nil
		}

		combined := make([]memory.FileInfo, 0, len(files))
		seen := map[string]struct{}{}
		addFiles := func(scope string, entries []memory.FileInfo) {
			for _, f := range entries {
				key := scope + "\x00" + f.Path
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				combined = append(combined, f)
			}
		}
		addFiles("global", files)

		if repoMgr != nil {
			repoFiles, repoErr := listFromMgr(repoMgr)
			if repoErr != nil {
				if !(ref != "" && isRefLookupError(repoErr)) {
					Log("memory_list repo error: %v", repoErr)
				}
			} else {
				addFiles("repo", repoFiles)
			}
		}

		if legacyRepoMgr != nil {
			legacyFiles, legacyErr := listFromMgr(legacyRepoMgr)
			if legacyErr != nil {
				if !(ref != "" && isRefLookupError(legacyErr)) {
					Log("memory_list legacy repo error: %v", legacyErr)
				}
			} else {
				addFiles("repo", legacyFiles)
			}
		}

		data, _ := json.MarshalIndent(combined, "", "  ")
		Log("memory_list: %d files", len(combined))
		return gomcp.NewToolResultText(string(data)), nil
	}
}

// handleMemoryTree returns the memory file tree with descriptions.
func handleMemoryTree(globalMgr *memory.Manager, repoMgr *memory.Manager, legacyRepoMgr *memory.Manager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		Log("tool call: memory_tree")
		ref := req.GetString("ref", "")

		treeFromMgr := func(mgr *memory.Manager) ([]memory.TreeEntry, error) {
			if mgr == nil {
				return nil, nil
			}
			if ref == "" {
				return mgr.Tree()
			}
			return mgr.TreeAtRef(ref)
		}

		entries, err := treeFromMgr(globalMgr)
		if err != nil && !(ref != "" && isRefLookupError(err)) {
			return gomcp.NewToolResultError("failed to get memory tree: " + err.Error()), nil
		}
		if err != nil {
			entries = nil
		}

		combined := make([]memory.TreeEntry, 0, len(entries))
		seen := map[string]struct{}{}
		addEntries := func(scope string, rows []memory.TreeEntry) {
			for _, e := range rows {
				key := scope + "\x00" + e.Path
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				combined = append(combined, e)
			}
		}
		addEntries("global", entries)

		if repoMgr != nil {
			repoEntries, repoErr := treeFromMgr(repoMgr)
			if repoErr == nil {
				addEntries("repo", repoEntries)
			} else if !(ref != "" && isRefLookupError(repoErr)) {
				Log("memory_tree repo error: %v", repoErr)
			}
		}
		if legacyRepoMgr != nil {
			legacyEntries, legacyErr := treeFromMgr(legacyRepoMgr)
			if legacyErr == nil {
				addEntries("repo", legacyEntries)
			} else if !(ref != "" && isRefLookupError(legacyErr)) {
				Log("memory_tree legacy repo error: %v", legacyErr)
			}
		}

		data, _ := json.MarshalIndent(combined, "", "  ")
		Log("memory_tree: %d entries", len(combined))
		return gomcp.NewToolResultText(string(data)), nil
	}
}

type scopedGitLogEntry struct {
	Scope       string   `json:"scope"`
	SHA         string   `json:"sha"`
	ParentSHA   string   `json:"parent_sha,omitempty"`
	Message     string   `json:"message"`
	Date        string   `json:"date"`
	AuthorName  string   `json:"author_name,omitempty"`
	AuthorEmail string   `json:"author_email,omitempty"`
	Additions   int      `json:"additions,omitempty"`
	Deletions   int      `json:"deletions,omitempty"`
	Branch      string   `json:"branch,omitempty"`
	Files       []string `json:"files,omitempty"`
}

// handleMemoryHistory returns git history for a memory file or all files.
func handleMemoryHistory(globalMgr *memory.Manager, repoMgr *memory.Manager, legacyRepoMgr *memory.Manager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		Log("tool call: memory_history")
		relPath := req.GetString("path", "")
		scope := strings.ToLower(req.GetString("scope", "all"))
		if scope == "" {
			scope = "all"
		}
		branch := req.GetString("branch", "")
		count := parseOptionalIntArg(req, "count", 10)

		if scope != "all" && scope != "global" && scope != "repo" {
			return gomcp.NewToolResultError("invalid scope; expected one of: all, global, repo"), nil
		}

		type source struct {
			scope string
			mgr   *memory.Manager
		}
		var sources []source
		if scope == "all" || scope == "global" {
			sources = append(sources, source{scope: "global", mgr: globalMgr})
		}
		if scope == "all" || scope == "repo" {
			sources = append(sources, source{scope: "repo", mgr: repoMgr})
			if legacyRepoMgr != nil {
				sources = append(sources, source{scope: "repo", mgr: legacyRepoMgr})
			}
		}

		var merged []scopedGitLogEntry
		seen := map[string]struct{}{}
		anyGitEnabled := false
		for _, src := range sources {
			if src.mgr == nil {
				continue
			}
			if src.mgr.GitEnabled() {
				anyGitEnabled = true
			}

			pathForMgr := relPath
			if slug, rel, ok := parseRepoMemoryPath(relPath); ok {
				if src.scope == "repo" && slug == repoSlugFromManager(src.mgr) {
					pathForMgr = rel
				} else if src.scope == "repo" {
					continue
				}
			}

			entries, err := src.mgr.HistoryWithBranch(pathForMgr, count, branch)
			if err != nil {
				if branch != "" && isRefLookupError(err) {
					continue
				}
				return gomcp.NewToolResultError("failed to get history: " + err.Error()), nil
			}
			for _, e := range entries {
				key := fmt.Sprintf("%s\x00%s", src.scope, e.SHA)
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				merged = append(merged, scopedGitLogEntry{
					Scope:       src.scope,
					SHA:         e.SHA,
					ParentSHA:   e.ParentSHA,
					Message:     e.Message,
					Date:        e.Date,
					AuthorName:  e.AuthorName,
					AuthorEmail: e.AuthorEmail,
					Additions:   e.Additions,
					Deletions:   e.Deletions,
					Branch:      e.Branch,
					Files:       e.Files,
				})
			}
		}

		if !anyGitEnabled {
			return gomcp.NewToolResultText("Git versioning not available for memory."), nil
		}

		sort.Slice(merged, func(i, j int) bool {
			ti, ei := time.Parse(time.RFC3339, merged[i].Date)
			tj, ej := time.Parse(time.RFC3339, merged[j].Date)
			switch {
			case ei == nil && ej == nil:
				if ti.Equal(tj) {
					return merged[i].SHA > merged[j].SHA
				}
				return ti.After(tj)
			case ei == nil:
				return true
			case ej == nil:
				return false
			default:
				if merged[i].Date == merged[j].Date {
					return merged[i].SHA > merged[j].SHA
				}
				return merged[i].Date > merged[j].Date
			}
		})
		if len(merged) > count {
			merged = merged[:count]
		}

		data, _ := json.MarshalIndent(merged, "", "  ")
		Log("memory_history: scope=%q path=%q branch=%q entries=%d", scope, relPath, branch, len(merged))
		return gomcp.NewToolResultText(string(data)), nil
	}
}

// handleMemoryAppend appends content to a memory file.
func handleMemoryAppend(globalMgr *memory.Manager, repoMgr *memory.Manager, legacyRepoMgr *memory.Manager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		Log("tool call: memory_append")
		relPath := req.GetString("path", "")
		content := req.GetString("content", "")
		branch := req.GetString("branch", "")
		if relPath == "" {
			return gomcp.NewToolResultError("missing required parameter: path"), nil
		}
		if content == "" {
			return gomcp.NewToolResultError("missing required parameter: content"), nil
		}
		mgr, scopedPath := routePathToManager(relPath, globalMgr, repoMgr, legacyRepoMgr)
		if err := mgr.AppendOnBranch(scopedPath, content, branch); err != nil {
			Log("memory_append error: %v", err)
			return gomcp.NewToolResultError("failed to append: " + err.Error()), nil
		}
		return gomcp.NewToolResultText(fmt.Sprintf("Appended %d chars to %s.", len(content), relPath)), nil
	}
}

// handleMemoryMove renames/moves a memory file.
func handleMemoryMove(globalMgr *memory.Manager, repoMgr *memory.Manager, legacyRepoMgr *memory.Manager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		Log("tool call: memory_move")
		from := req.GetString("from", "")
		to := req.GetString("to", "")
		branch := req.GetString("branch", "")
		if from == "" || to == "" {
			return gomcp.NewToolResultError("missing required parameters: from, to"), nil
		}
		fromMgr, fromRel := routePathToManager(from, globalMgr, repoMgr, legacyRepoMgr)
		toMgr, toRel := routePathToManager(to, globalMgr, repoMgr, legacyRepoMgr)
		if fromMgr != toMgr {
			return gomcp.NewToolResultError("cross-scope move is not supported"), nil
		}
		if err := fromMgr.MoveOnBranch(fromRel, toRel, branch); err != nil {
			return gomcp.NewToolResultError("failed to move: " + err.Error()), nil
		}
		return gomcp.NewToolResultText(fmt.Sprintf("Moved %s -> %s.", from, to)), nil
	}
}

// handleMemoryDelete removes a memory file.
func handleMemoryDelete(globalMgr *memory.Manager, repoMgr *memory.Manager, legacyRepoMgr *memory.Manager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		Log("tool call: memory_delete")
		relPath := req.GetString("path", "")
		branch := req.GetString("branch", "")
		if relPath == "" {
			return gomcp.NewToolResultError("missing required parameter: path"), nil
		}
		mgr, scopedPath := routePathToManager(relPath, globalMgr, repoMgr, legacyRepoMgr)
		if err := mgr.DeleteOnBranch(scopedPath, branch); err != nil {
			return gomcp.NewToolResultError("failed to delete: " + err.Error()), nil
		}
		return gomcp.NewToolResultText(fmt.Sprintf("Deleted %s.", relPath)), nil
	}
}

// handleMemoryPin moves a file to system/ (always-in-context).
func handleMemoryPin(globalMgr *memory.Manager, repoMgr *memory.Manager, legacyRepoMgr *memory.Manager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		Log("tool call: memory_pin")
		relPath := req.GetString("path", "")
		branch := req.GetString("branch", "")
		if relPath == "" {
			return gomcp.NewToolResultError("missing required parameter: path"), nil
		}
		mgr, scopedPath := routePathToManager(relPath, globalMgr, repoMgr, legacyRepoMgr)
		if err := mgr.PinOnBranch(scopedPath, branch); err != nil {
			return gomcp.NewToolResultError("failed to pin: " + err.Error()), nil
		}
		return gomcp.NewToolResultText(fmt.Sprintf("Pinned %s to system/.", relPath)), nil
	}
}

// handleMemoryUnpin moves a file out of system/.
func handleMemoryUnpin(globalMgr *memory.Manager, repoMgr *memory.Manager, legacyRepoMgr *memory.Manager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		Log("tool call: memory_unpin")
		relPath := req.GetString("path", "")
		branch := req.GetString("branch", "")
		if relPath == "" {
			return gomcp.NewToolResultError("missing required parameter: path"), nil
		}
		mgr, scopedPath := routePathToManager(relPath, globalMgr, repoMgr, legacyRepoMgr)
		if err := mgr.UnpinOnBranch(scopedPath, branch); err != nil {
			return gomcp.NewToolResultError("failed to unpin: " + err.Error()), nil
		}
		return gomcp.NewToolResultText(fmt.Sprintf("Unpinned %s from system/.", relPath)), nil
	}
}

type memoryBranchState struct {
	Scope        string   `json:"scope"`
	StoreSlug    string   `json:"store_slug,omitempty"`
	Current      string   `json:"current"`
	Default      string   `json:"default"`
	Branches     []string `json:"branches"`
	GitAvailable bool     `json:"git_available"`
}

func managerScopeSlug(scope string, mgr *memory.Manager) string {
	if scope == "global" || mgr == nil {
		return ""
	}
	return filepath.Base(filepath.Clean(mgr.Dir()))
}

func handleMemoryBranches(globalMgr *memory.Manager, repoMgr *memory.Manager, legacyRepoMgr *memory.Manager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		Log("tool call: memory_branches")
		scope := strings.ToLower(req.GetString("scope", "repo"))
		if scope == "" {
			scope = "repo"
		}
		if scope != "repo" && scope != "global" && scope != "all" {
			return gomcp.NewToolResultError("invalid scope; expected one of: all, global, repo"), nil
		}

		type source struct {
			scope string
			mgr   *memory.Manager
		}
		var sources []source
		switch scope {
		case "global":
			sources = append(sources, source{scope: "global", mgr: globalMgr})
		case "repo":
			sources = append(sources, source{scope: "repo", mgr: repoMgr})
			if legacyRepoMgr != nil {
				sources = append(sources, source{scope: "repo", mgr: legacyRepoMgr})
			}
		case "all":
			sources = append(sources, source{scope: "global", mgr: globalMgr})
			sources = append(sources, source{scope: "repo", mgr: repoMgr})
			if legacyRepoMgr != nil {
				sources = append(sources, source{scope: "repo", mgr: legacyRepoMgr})
			}
		}

		var out []memoryBranchState
		for _, src := range sources {
			if src.mgr == nil {
				continue
			}
			if !src.mgr.GitEnabled() {
				out = append(out, memoryBranchState{
					Scope:        src.scope,
					StoreSlug:    managerScopeSlug(src.scope, src.mgr),
					GitAvailable: false,
				})
				continue
			}
			info, err := src.mgr.GitBranchInfo()
			if err != nil {
				return gomcp.NewToolResultError("failed to list branches: " + err.Error()), nil
			}
			out = append(out, memoryBranchState{
				Scope:        src.scope,
				StoreSlug:    managerScopeSlug(src.scope, src.mgr),
				Current:      info.Current,
				Default:      info.Default,
				Branches:     info.All,
				GitAvailable: true,
			})
		}

		if len(out) == 0 {
			return gomcp.NewToolResultText("Git versioning not available for memory."), nil
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		return gomcp.NewToolResultText(string(data)), nil
	}
}

func handleMemoryBranchCreate(globalMgr *memory.Manager, repoMgr *memory.Manager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		Log("tool call: memory_branch_create")
		name := req.GetString("name", "")
		fromRef := req.GetString("from_ref", "")
		scope := req.GetString("scope", "")
		if strings.TrimSpace(name) == "" {
			return gomcp.NewToolResultError("missing required parameter: name"), nil
		}
		mgr := resolveScopedManager(scope, globalMgr, repoMgr)
		if err := mgr.CreateBranch(name, fromRef); err != nil {
			return gomcp.NewToolResultError("failed to create branch: " + err.Error()), nil
		}
		return gomcp.NewToolResultText(fmt.Sprintf("Created branch %s.", name)), nil
	}
}

func handleMemoryBranchDelete(globalMgr *memory.Manager, repoMgr *memory.Manager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		Log("tool call: memory_branch_delete")
		name := req.GetString("name", "")
		scope := req.GetString("scope", "")
		force := parseOptionalBoolArg(req, "force", false)
		if strings.TrimSpace(name) == "" {
			return gomcp.NewToolResultError("missing required parameter: name"), nil
		}
		mgr := resolveScopedManager(scope, globalMgr, repoMgr)
		if err := mgr.DeleteBranch(name, force); err != nil {
			return gomcp.NewToolResultError("failed to delete branch: " + err.Error()), nil
		}
		return gomcp.NewToolResultText(fmt.Sprintf("Deleted branch %s.", name)), nil
	}
}

func handleMemoryBranchMerge(globalMgr *memory.Manager, repoMgr *memory.Manager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		Log("tool call: memory_branch_merge")
		source := req.GetString("source", "")
		target := req.GetString("target", "")
		strategy := req.GetString("strategy", "")
		scope := req.GetString("scope", "")
		if strings.TrimSpace(source) == "" {
			return gomcp.NewToolResultError("missing required parameter: source"), nil
		}
		mgr := resolveScopedManager(scope, globalMgr, repoMgr)
		if err := mgr.MergeBranch(source, target, strategy); err != nil {
			return gomcp.NewToolResultError("failed to merge branch: " + err.Error()), nil
		}
		if strings.TrimSpace(target) == "" {
			target = "(default)"
		}
		return gomcp.NewToolResultText(fmt.Sprintf("Merged %s into %s.", source, target)), nil
	}
}

func handleMemoryDiff(globalMgr *memory.Manager, repoMgr *memory.Manager, legacyRepoMgr *memory.Manager) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		Log("tool call: memory_diff")
		baseRef := req.GetString("base_ref", "")
		headRef := req.GetString("head_ref", "")
		path := req.GetString("path", "")
		scope := req.GetString("scope", "")
		if strings.TrimSpace(baseRef) == "" || strings.TrimSpace(headRef) == "" {
			return gomcp.NewToolResultError("missing required parameters: base_ref, head_ref"), nil
		}

		mgr := resolveScopedManager(scope, globalMgr, repoMgr)
		relPath := path
		if path != "" {
			routedMgr, routedPath := routePathToManager(path, globalMgr, repoMgr, legacyRepoMgr)
			mgr = routedMgr
			relPath = routedPath
		}

		diff, err := mgr.DiffRefs(baseRef, headRef, relPath)
		if err != nil {
			return gomcp.NewToolResultError("failed to get diff: " + err.Error()), nil
		}
		if strings.TrimSpace(diff) == "" {
			return gomcp.NewToolResultText("(no diff)"), nil
		}
		return gomcp.NewToolResultText(diff), nil
	}
}
