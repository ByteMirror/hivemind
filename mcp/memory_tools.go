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
		strings.Contains(s, "bad object") ||
		strings.Contains(s, "not a valid object name") ||
		strings.Contains(s, "invalid object name") ||
		strings.Contains(s, "pathspec") ||
		strings.Contains(s, "did not match any file(s) known to git") ||
		strings.Contains(s, "not in the working tree") ||
		strings.Contains(s, "ambiguous argument")
}

func isMissingPathError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, memory.ErrFileNotFound) {
		return true
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "memory file not found") ||
		strings.Contains(s, "does not exist in") ||
		strings.Contains(s, "exists on disk, but not in")
}

func shouldTryRepoFallback(err error) bool {
	return isMissingPathError(err) || isRefLookupError(err)
}

type pathManagerCandidate struct {
	mgr  *memory.Manager
	path string
}

func pathManagerCandidates(path string, globalMgr, repoMgr, legacyRepoMgr *memory.Manager) []pathManagerCandidate {
	mgr, scopedPath := routePathToManager(path, globalMgr, repoMgr, legacyRepoMgr)
	candidates := []pathManagerCandidate{{mgr: mgr, path: scopedPath}}

	// For unscoped paths, try repo stores after global to avoid surprising misses
	// when the file/ref exists only in repo memory.
	if _, _, scoped := parseRepoMemoryPath(path); scoped {
		return candidates
	}
	if mgr != globalMgr {
		return candidates
	}
	if repoMgr != nil && repoMgr != mgr {
		candidates = append(candidates, pathManagerCandidate{mgr: repoMgr, path: path})
	}
	if legacyRepoMgr != nil && legacyRepoMgr != mgr && legacyRepoMgr != repoMgr {
		candidates = append(candidates, pathManagerCandidate{mgr: legacyRepoMgr, path: path})
	}
	return candidates
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

func missingParamErr(param, example string) *gomcp.CallToolResult {
	msg := "missing required parameter: " + param
	if strings.TrimSpace(example) != "" {
		msg += ". Example: " + example
	}
	return gomcp.NewToolResultError(msg)
}

func toolErrWithHint(prefix string, err error, hint string) *gomcp.CallToolResult {
	msg := prefix
	if err != nil {
		msg += ": " + err.Error()
	}
	if strings.TrimSpace(hint) != "" {
		msg += " Hint: " + hint
	}
	return gomcp.NewToolResultError(msg)
}

func readPathHint(ref string) string {
	if strings.TrimSpace(ref) != "" {
		return `Verify the ref with memory_branches(scope="repo"), then retry memory_read(path="...", ref="...").`
	}
	return `Use memory_tree() to discover paths. Use path="repos/<repo-slug>/file.md" to target repo memory explicitly.`
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
			return missingParamErr("content", `memory_write(content="Remember X", scope="repo", file="notes.md")`), nil
		}

		mgr := resolveMemoryScope(scope, file, globalMgr, repoMgr)
		if file == "" && strings.ToLower(scope) == "global" {
			file = "system/global.md"
		}

		if err := mgr.WriteWithCommitMessageOnBranch(content, file, commitMessage, branch); err != nil {
			Log("memory_write error: %v", err)
			return toolErrWithHint(
				"failed to write memory",
				err,
				`Use "file" for write target. For existing files, use path-based tools like memory_read/memory_get/memory_append.`,
			), nil
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
			return missingParamErr("path", `memory_read(path="system/global.md")`), nil
		}
		ref := req.GetString("ref", "")

		candidates := pathManagerCandidates(relPath, globalMgr, repoMgr, legacyRepoMgr)
		var lastErr error
		for i, candidate := range candidates {
			if candidate.mgr == nil {
				continue
			}
			var (
				body string
				err  error
			)
			if ref == "" {
				body, err = candidate.mgr.Read(candidate.path)
			} else {
				body, err = candidate.mgr.ReadAtRef(candidate.path, ref)
			}
			if err == nil {
				return gomcp.NewToolResultText(body), nil
			}
			lastErr = err
			if i == len(candidates)-1 || !shouldTryRepoFallback(err) {
				return toolErrWithHint("failed to read memory file", err, readPathHint(ref)), nil
			}
		}

		if lastErr == nil {
			return toolErrWithHint("failed to read memory file", nil, readPathHint(ref)), nil
		}
		return toolErrWithHint("failed to read memory file", lastErr, readPathHint(ref)), nil
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
			return missingParamErr("query", `memory_search(query="commit message conventions")`), nil
		}

		maxResults := parseOptionalIntArg(req, "max_results", 10)

		globalResults, err := globalMgr.Search(query, memory.SearchOpts{MaxResults: maxResults})
		if err != nil {
			Log("memory_search global error: %v", err)
			return toolErrWithHint("search failed", err, `Retry with a shorter keyword query, e.g. memory_search(query="conventions").`), nil
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
			return missingParamErr("path", `memory_get(path="system/global.md", from=1, lines=20)`), nil
		}
		from := parseOptionalIntArg(req, "from", 0)
		lines := parseOptionalIntArg(req, "lines", 0)
		ref := req.GetString("ref", "")

		candidates := pathManagerCandidates(relPath, globalMgr, repoMgr, legacyRepoMgr)
		var lastErr error
		for i, candidate := range candidates {
			if candidate.mgr == nil {
				continue
			}
			var (
				text string
				err  error
			)
			if ref == "" {
				text, err = candidate.mgr.Get(candidate.path, from, lines)
			} else {
				text, err = candidate.mgr.GetAtRef(candidate.path, from, lines, ref)
			}
			if err == nil {
				return gomcp.NewToolResultText(text), nil
			}
			lastErr = err
			if i == len(candidates)-1 || !shouldTryRepoFallback(err) {
				return toolErrWithHint("failed to read memory file", err, readPathHint(ref)), nil
			}
		}

		if lastErr == nil {
			return toolErrWithHint("failed to read memory file", nil, readPathHint(ref)), nil
		}
		return toolErrWithHint("failed to read memory file", lastErr, readPathHint(ref)), nil
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
			return toolErrWithHint(
				"failed to list memory",
				err,
				`If using ref, verify it with memory_branches(scope="repo"), or retry without ref.`,
			), nil
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
			return toolErrWithHint(
				"failed to get memory tree",
				err,
				`If using ref, verify it with memory_branches(scope="repo"), or retry without ref.`,
			), nil
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
			return gomcp.NewToolResultError(`invalid scope; expected one of: all, global, repo. Example: memory_history(scope="repo", count=10)`), nil
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
				return toolErrWithHint(
					"failed to get history",
					err,
					`Verify branch/ref with memory_branches(scope="repo"), then retry memory_history(..., branch="...").`,
				), nil
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
			return missingParamErr("path", `memory_append(path="notes.md", content="new note")`), nil
		}
		if content == "" {
			return missingParamErr("content", `memory_append(path="notes.md", content="new note")`), nil
		}

		candidates := pathManagerCandidates(relPath, globalMgr, repoMgr, legacyRepoMgr)
		var lastErr error
		for i, candidate := range candidates {
			if candidate.mgr == nil {
				continue
			}
			if err := candidate.mgr.AppendOnBranch(candidate.path, content, branch); err != nil {
				lastErr = err
				if i == len(candidates)-1 || !shouldTryRepoFallback(err) {
					Log("memory_append error: %v", err)
					return toolErrWithHint(
						"failed to append",
						err,
						`Use path for an existing file. For repo disambiguation use path="repos/<repo-slug>/file.md".`,
					), nil
				}
				continue
			}
			return gomcp.NewToolResultText(fmt.Sprintf("Appended %d chars to %s.", len(content), relPath)), nil
		}

		if lastErr != nil {
			Log("memory_append error: %v", lastErr)
			return toolErrWithHint(
				"failed to append",
				lastErr,
				`Use path for an existing file. For repo disambiguation use path="repos/<repo-slug>/file.md".`,
			), nil
		}
		return toolErrWithHint(
			"failed to append",
			nil,
			`Use path for an existing file. For repo disambiguation use path="repos/<repo-slug>/file.md".`,
		), nil
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
			return gomcp.NewToolResultError(`missing required parameters: from, to. Example: memory_move(from="notes.md", to="archive/notes.md")`), nil
		}
		fromMgr, fromRel := routePathToManager(from, globalMgr, repoMgr, legacyRepoMgr)
		toMgr, toRel := routePathToManager(to, globalMgr, repoMgr, legacyRepoMgr)
		if fromMgr != toMgr {
			return gomcp.NewToolResultError(`cross-scope move is not supported. Hint: use the same store for both paths, e.g. repos/<repo-slug>/from.md -> repos/<repo-slug>/to.md`), nil
		}
		if err := fromMgr.MoveOnBranch(fromRel, toRel, branch); err != nil {
			return toolErrWithHint("failed to move", err, `Use "from" and "to" as existing/target file paths within the same store.`), nil
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
			return missingParamErr("path", `memory_delete(path="notes.md")`), nil
		}
		mgr, scopedPath := routePathToManager(relPath, globalMgr, repoMgr, legacyRepoMgr)
		if err := mgr.DeleteOnBranch(scopedPath, branch); err != nil {
			return toolErrWithHint("failed to delete", err, `Use path for an existing file. Use repos/<repo-slug>/... to target repo memory explicitly.`), nil
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
			return missingParamErr("path", `memory_pin(path="conventions.md")`), nil
		}
		mgr, scopedPath := routePathToManager(relPath, globalMgr, repoMgr, legacyRepoMgr)
		if err := mgr.PinOnBranch(scopedPath, branch); err != nil {
			return toolErrWithHint("failed to pin", err, `Use path for an existing file. This moves it to system/<name>.md.`), nil
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
			return missingParamErr("path", `memory_unpin(path="system/conventions.md")`), nil
		}
		mgr, scopedPath := routePathToManager(relPath, globalMgr, repoMgr, legacyRepoMgr)
		if err := mgr.UnpinOnBranch(scopedPath, branch); err != nil {
			return toolErrWithHint("failed to unpin", err, `Path must currently be under system/, e.g. system/conventions.md.`), nil
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
			return gomcp.NewToolResultError(`invalid scope; expected one of: all, global, repo. Example: memory_branches(scope="repo")`), nil
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
				return toolErrWithHint("failed to list branches", err, `Retry with scope="repo" or scope="global" to isolate the failing store.`), nil
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
			return missingParamErr("name", `memory_branch_create(name="feature/memory", scope="repo")`), nil
		}
		mgr := resolveScopedManager(scope, globalMgr, repoMgr)
		if err := mgr.CreateBranch(name, fromRef); err != nil {
			return toolErrWithHint("failed to create branch", err, `Check branch name format and from_ref, e.g. from_ref="main".`), nil
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
			return missingParamErr("name", `memory_branch_delete(name="feature/memory", force=true, scope="repo")`), nil
		}
		mgr := resolveScopedManager(scope, globalMgr, repoMgr)
		if err := mgr.DeleteBranch(name, force); err != nil {
			return toolErrWithHint("failed to delete branch", err, `Set force=true if the branch is not fully merged.`), nil
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
			return missingParamErr("source", `memory_branch_merge(source="feature/memory", target="main", strategy="ff-only", scope="repo")`), nil
		}
		mgr := resolveScopedManager(scope, globalMgr, repoMgr)
		if err := mgr.MergeBranch(source, target, strategy); err != nil {
			return toolErrWithHint("failed to merge branch", err, `Try strategy="no-ff" or resolve conflicts manually, then retry.`), nil
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
			return gomcp.NewToolResultError(`missing required parameters: base_ref, head_ref. Example: memory_diff(base_ref="main", head_ref="feature/memory", scope="repo")`), nil
		}

		mgr := resolveScopedManager(scope, globalMgr, repoMgr)
		relPath := path
		if path != "" {
			routedMgr, routedPath := routePathToManager(path, globalMgr, repoMgr, legacyRepoMgr)
			if _, _, hasRepoPrefix := parseRepoMemoryPath(path); hasRepoPrefix || strings.TrimSpace(scope) == "" {
				mgr = routedMgr
				relPath = routedPath
			}
		}

		candidates := []pathManagerCandidate{{mgr: mgr, path: relPath}}
		if path != "" && strings.TrimSpace(scope) == "" {
			candidates = pathManagerCandidates(path, globalMgr, repoMgr, legacyRepoMgr)
		}

		var (
			diff    string
			err     error
			lastErr error
		)
		for i, candidate := range candidates {
			if candidate.mgr == nil {
				continue
			}
			diff, err = candidate.mgr.DiffRefs(baseRef, headRef, candidate.path)
			if err == nil {
				break
			}
			lastErr = err
			if i == len(candidates)-1 || !shouldTryRepoFallback(err) {
				return toolErrWithHint("failed to get diff", err, `Verify refs with memory_branches(scope="repo"). Use path="repos/<repo-slug>/file.md" to target repo memory explicitly.`), nil
			}
		}
		if err != nil {
			if lastErr != nil {
				return toolErrWithHint("failed to get diff", lastErr, `Verify refs with memory_branches(scope="repo").`), nil
			}
			return toolErrWithHint("failed to get diff", nil, `No memory manager available for this scope/path.`), nil
		}
		if strings.TrimSpace(diff) == "" {
			return gomcp.NewToolResultText("(no diff)"), nil
		}
		return gomcp.NewToolResultText(diff), nil
	}
}
