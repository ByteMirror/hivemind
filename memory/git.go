package memory

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	hlog "github.com/ByteMirror/hivemind/log"
)

// ErrNoChanges is returned by AutoCommit when the working tree is clean.
var ErrNoChanges = errors.New("no changes to commit")

// ErrRepoBusy is returned when a memory repo lock cannot be acquired in time.
var ErrRepoBusy = errors.New("memory repo busy")

const (
	gitLogRecordSep = "\x1e"
	gitLogFieldSep  = "\x1f"

	repoLockFileName    = ".hivemind-memory.lock"
	repoLockTimeout     = 10 * time.Second
	repoLockRetry       = 50 * time.Millisecond
	repoLockStaleAfter  = 10 * time.Minute
	maxHistoryScanCount = 5000
)

// GitRepo manages a git repository for memory versioning.
type GitRepo struct {
	dir string
}

// GitLogEntry represents a single commit in the memory git log.
type GitLogEntry struct {
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

// GitBranchInfo describes branch metadata for a memory repo.
type GitBranchInfo struct {
	Current string   `json:"current"`
	Default string   `json:"default"`
	All     []string `json:"all"`
}

type repoLockMeta struct {
	PID       int   `json:"pid"`
	CreatedAt int64 `json:"created_at"`
}

type repoFileLock struct {
	path string
	file *os.File
}

func (l *repoFileLock) release() {
	if l == nil {
		return
	}
	if l.file != nil {
		_ = l.file.Close()
	}
	_ = os.Remove(l.path)
}

// InitGitRepo initialises a git repo in dir if one doesn't already exist.
// It creates a .gitignore that excludes the .index/ directory.
// This is idempotent — calling it on an already-initialised repo is safe.
func InitGitRepo(dir string) (*GitRepo, error) {
	g := &GitRepo{dir: dir}

	if !g.IsInitialized() {
		if _, err := g.gitExec("init"); err != nil {
			return nil, fmt.Errorf("git init: %w", err)
		}
	}

	// Configure committer identity for this repo.
	if _, err := g.gitExec("config", "user.name", "Hivemind Memory"); err != nil {
		return nil, fmt.Errorf("git config user.name: %w", err)
	}
	if _, err := g.gitExec("config", "user.email", "memory@hivemind.local"); err != nil {
		return nil, fmt.Errorf("git config user.email: %w", err)
	}

	// Ensure .gitignore excludes .index/
	gitignorePath := filepath.Join(dir, ".gitignore")
	desired := ".index/\n"
	if data, err := os.ReadFile(gitignorePath); err != nil || !strings.Contains(string(data), ".index/") {
		existing := ""
		if err == nil {
			existing = string(data)
		}
		if existing != "" && !strings.HasSuffix(existing, "\n") {
			existing += "\n"
		}
		if err := os.WriteFile(gitignorePath, []byte(existing+desired), 0600); err != nil {
			return nil, fmt.Errorf("write .gitignore: %w", err)
		}
	}

	return g, nil
}

// IsInitialized returns true if the directory contains a .git directory.
func (g *GitRepo) IsInitialized() bool {
	info, err := os.Stat(filepath.Join(g.dir, ".git"))
	return err == nil && info.IsDir()
}

// AutoCommit stages all changes and commits with the given message.
// Returns ErrNoChanges if the working tree is clean.
func (g *GitRepo) AutoCommit(message string) error {
	return g.withRepoLock("auto_commit", func() error {
		return g.autoCommitUnlocked(message)
	})
}

func (g *GitRepo) autoCommitUnlocked(message string) error {
	if _, err := g.gitExec("add", "-A"); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	status, err := g.gitExec("status", "--porcelain")
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}
	if strings.TrimSpace(status) == "" {
		return ErrNoChanges
	}

	if _, err := g.gitExec("commit", "-m", message); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}
	return nil
}

// Log returns the most recent commits, optionally filtered by file path.
// If path is empty, all commits are returned.
func (g *GitRepo) Log(path string, count int) ([]GitLogEntry, error) {
	return g.LogWithBranch(path, count, "")
}

// LogWithBranch returns the most recent commits on a specific branch/ref.
// When branch is empty, history is read from HEAD.
func (g *GitRepo) LogWithBranch(path string, count int, branch string) ([]GitLogEntry, error) {
	if count <= 0 {
		count = 10
	}
	if count > maxHistoryScanCount {
		count = maxHistoryScanCount
	}

	args := []string{
		"log",
		fmt.Sprintf("--max-count=%d", count),
		fmt.Sprintf("--format=%s%%H%s%%P%s%%s%s%%aI%s%%an%s%%ae", gitLogRecordSep, gitLogFieldSep, gitLogFieldSep, gitLogFieldSep, gitLogFieldSep, gitLogFieldSep),
		"--numstat",
	}
	if branch != "" {
		args = append(args, branch)
	}
	if path != "" {
		args = append(args, "--", path)
	}

	out, err := g.gitExec(args...)
	if err != nil {
		// Empty repo (no commits yet) returns an error — treat as empty.
		if strings.Contains(err.Error(), "does not have any commits") {
			return nil, nil
		}
		return nil, fmt.Errorf("git log: %w", err)
	}

	return parseGitLog(out, branch), nil
}

func parseGitLog(raw string, branch string) []GitLogEntry {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	var entries []GitLogEntry
	raw = strings.TrimPrefix(raw, gitLogRecordSep)
	blocks := strings.Split(raw, gitLogRecordSep)
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}

		lines := strings.Split(block, "\n")
		if len(lines) == 0 {
			continue
		}

		parts := strings.SplitN(strings.TrimSpace(lines[0]), gitLogFieldSep, 6)
		if len(parts) < 6 {
			continue
		}

		entry := GitLogEntry{
			SHA:         parts[0],
			ParentSHA:   firstParentSHA(parts[1]),
			Message:     parts[2],
			Date:        parts[3],
			AuthorName:  parts[4],
			AuthorEmail: parts[5],
			Branch:      branch,
		}

		seenFiles := map[string]struct{}{}
		for _, l := range lines[1:] {
			l = strings.TrimSpace(l)
			if l == "" {
				continue
			}

			cols := strings.SplitN(l, "\t", 3)
			if len(cols) == 3 {
				file := strings.TrimSpace(cols[2])
				if file != "" {
					if _, ok := seenFiles[file]; !ok {
						seenFiles[file] = struct{}{}
						entry.Files = append(entry.Files, file)
					}
				}
				if n, err := strconv.Atoi(cols[0]); err == nil {
					entry.Additions += n
				}
				if n, err := strconv.Atoi(cols[1]); err == nil {
					entry.Deletions += n
				}
				continue
			}

			if _, ok := seenFiles[l]; !ok {
				seenFiles[l] = struct{}{}
				entry.Files = append(entry.Files, l)
			}
		}
		entries = append(entries, entry)
	}
	return entries
}

func firstParentSHA(parents string) string {
	parents = strings.TrimSpace(parents)
	if parents == "" {
		return ""
	}
	parts := strings.Fields(parents)
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

// CurrentBranch returns the currently checked-out branch name.
func (g *GitRepo) CurrentBranch() (string, error) {
	// symbolic-ref works even on an unborn branch (no commits yet).
	if out, err := g.gitExec("symbolic-ref", "--short", "HEAD"); err == nil {
		name := strings.TrimSpace(out)
		if name != "" {
			return name, nil
		}
	}

	out, err := g.gitExec("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git current branch: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// DefaultBranch returns the preferred default branch for this memory repo.
func (g *GitRepo) DefaultBranch() (string, error) {
	headBranch, headErr := g.CurrentBranch()

	if out, err := g.gitExec("symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"); err == nil {
		ref := strings.TrimSpace(out)
		ref = strings.TrimPrefix(ref, "refs/remotes/origin/")
		if ref != "" {
			return ref, nil
		}
	}

	branches, err := g.Branches()
	if err != nil {
		return "", err
	}
	for _, b := range branches {
		if b == "main" {
			return "main", nil
		}
	}
	for _, b := range branches {
		if b == "master" {
			return "master", nil
		}
	}
	if headErr == nil && headBranch != "" {
		return headBranch, nil
	}

	if headErr != nil {
		// Empty repo edge case: use main as default fallback.
		if len(branches) == 0 {
			return "main", nil
		}
		return "", headErr
	}
	if headBranch == "" {
		if len(branches) > 0 {
			return branches[0], nil
		}
		return "main", nil
	}
	return headBranch, nil
}

// Branches returns all branch names in the repo.
func (g *GitRepo) Branches() ([]string, error) {
	out, err := g.gitExec("branch", "--format=%(refname:short)")
	if err != nil {
		return nil, fmt.Errorf("git branch: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var branches []string
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name != "" {
			branches = append(branches, name)
		}
	}
	return branches, nil
}

// BranchInfo returns current/default branch and the complete branch list.
func (g *GitRepo) BranchInfo() (GitBranchInfo, error) {
	branches, err := g.Branches()
	if err != nil {
		return GitBranchInfo{}, err
	}
	current, err := g.CurrentBranch()
	if err != nil {
		return GitBranchInfo{}, err
	}
	def, err := g.DefaultBranch()
	if err != nil {
		return GitBranchInfo{}, err
	}
	return GitBranchInfo{Current: current, Default: def, All: branches}, nil
}

// CreateBranch creates a new branch from fromRef (or default branch when empty).
func (g *GitRepo) CreateBranch(name, fromRef string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("branch name is required")
	}
	return g.withRepoLock("create_branch", func() error {
		if _, err := g.gitExec("check-ref-format", "--branch", name); err != nil {
			return fmt.Errorf("invalid branch name %q: %w", name, err)
		}
		// Handle unborn HEAD (repo with no commits yet).
		if _, err := g.gitExec("rev-parse", "--verify", "HEAD"); err != nil {
			current, currentErr := g.CurrentBranch()
			if currentErr != nil {
				return currentErr
			}
			if _, err := g.gitExec("checkout", "--quiet", "-b", name); err != nil {
				return fmt.Errorf("create branch %q on empty repo: %w", name, err)
			}
			if current != "" && current != name {
				_, _ = g.gitExec("checkout", "--quiet", current)
			}
			return nil
		}
		if fromRef == "" {
			var err error
			fromRef, err = g.DefaultBranch()
			if err != nil {
				return err
			}
		}
		if _, err := g.gitExec("branch", name, fromRef); err != nil {
			return fmt.Errorf("create branch %q from %q: %w", name, fromRef, err)
		}
		return nil
	})
}

// DeleteBranch deletes a branch. If force=true it uses -D.
func (g *GitRepo) DeleteBranch(name string, force bool) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("branch name is required")
	}
	return g.withRepoLock("delete_branch", func() error {
		current, err := g.CurrentBranch()
		if err != nil {
			return err
		}
		if current == name {
			return fmt.Errorf("cannot delete current branch %q", name)
		}

		flag := "-d"
		if force {
			flag = "-D"
		}
		if _, err := g.gitExec("branch", flag, name); err != nil {
			return fmt.Errorf("delete branch %q: %w", name, err)
		}
		return nil
	})
}

// MergeBranch merges source into target (or default branch when target is empty).
// Supported strategies: "ff-only" (default), "no-ff".
func (g *GitRepo) MergeBranch(source, target, strategy string) ([]string, error) {
	source = strings.TrimSpace(source)
	target = strings.TrimSpace(target)
	strategy = strings.TrimSpace(strategy)
	if source == "" {
		return nil, fmt.Errorf("source branch is required")
	}
	if strategy == "" {
		strategy = "ff-only"
	}
	if strategy != "ff-only" && strategy != "no-ff" {
		return nil, fmt.Errorf("unsupported merge strategy %q", strategy)
	}

	var changedFiles []string
	err := g.withRepoLock("merge_branch", func() error {
		if target == "" {
			def, err := g.DefaultBranch()
			if err != nil {
				return err
			}
			target = def
		}

		current, err := g.CurrentBranch()
		if err != nil {
			return err
		}

		if current != target {
			if _, err := g.gitExec("checkout", "--quiet", target); err != nil {
				return fmt.Errorf("checkout target branch %q: %w", target, err)
			}
		}

		restore := func() {
			if current != target {
				_, _ = g.gitExec("checkout", "--quiet", current)
			}
		}
		defer restore()

		oldHead, err := g.gitExec("rev-parse", "HEAD")
		if err != nil {
			return err
		}
		oldHead = strings.TrimSpace(oldHead)

		var mergeArgs []string
		switch strategy {
		case "ff-only":
			mergeArgs = []string{"merge", "--ff-only", source}
		case "no-ff":
			mergeArgs = []string{"merge", "--no-ff", "--no-edit", source}
		}
		if _, err := g.gitExec(mergeArgs...); err != nil {
			_, _ = g.gitExec("merge", "--abort")
			return fmt.Errorf("merge %q into %q: %w", source, target, err)
		}

		newHead, err := g.gitExec("rev-parse", "HEAD")
		if err != nil {
			return err
		}
		newHead = strings.TrimSpace(newHead)

		if oldHead == newHead {
			changedFiles = nil
			return nil
		}
		out, err := g.gitExec("diff", "--name-only", oldHead, newHead)
		if err != nil {
			return nil
		}
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				changedFiles = append(changedFiles, line)
			}
		}
		return nil
	})
	return changedFiles, err
}

// DiffRefs returns the git diff between two refs. Optional path limits the diff.
func (g *GitRepo) DiffRefs(baseRef, headRef, path string) (string, error) {
	if strings.TrimSpace(baseRef) == "" || strings.TrimSpace(headRef) == "" {
		return "", fmt.Errorf("base_ref and head_ref are required")
	}
	args := []string{"diff", fmt.Sprintf("%s..%s", baseRef, headRef)}
	if strings.TrimSpace(path) != "" {
		args = append(args, "--", path)
	}
	out, err := g.gitExec(args...)
	if err != nil {
		return "", fmt.Errorf("git diff: %w", err)
	}
	return out, nil
}

// ReadFileAtRef reads file content at a specific git ref.
func (g *GitRepo) ReadFileAtRef(ref, relPath string) (string, error) {
	if strings.TrimSpace(ref) == "" {
		return "", fmt.Errorf("ref is required")
	}
	spec := fmt.Sprintf("%s:%s", ref, filepath.ToSlash(relPath))
	out, err := g.gitExec("show", spec)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "does not exist in") || strings.Contains(errStr, "exists on disk, but not in") {
			return "", os.ErrNotExist
		}
		return "", fmt.Errorf("git show %s: %w", spec, err)
	}
	return out, nil
}

// ListMarkdownFilesAtRef returns markdown files and raw content at a ref.
func (g *GitRepo) ListMarkdownFilesAtRef(ref string) (map[string]string, error) {
	if strings.TrimSpace(ref) == "" {
		return nil, fmt.Errorf("ref is required")
	}
	out, err := g.gitExec("ls-tree", "-r", "--name-only", ref)
	if err != nil {
		return nil, fmt.Errorf("git ls-tree: %w", err)
	}

	result := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasSuffix(line, ".md") {
			continue
		}
		content, readErr := g.ReadFileAtRef(ref, line)
		if readErr != nil {
			continue
		}
		result[line] = content
	}
	return result, nil
}

// gitExec runs a git command in the repo directory and returns stdout.
func (g *GitRepo) gitExec(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func (g *GitRepo) withRepoLock(op string, fn func() error) error {
	startWait := time.Now()
	lock, err := g.acquireRepoLock(repoLockTimeout)
	if err != nil {
		if errors.Is(err, ErrRepoBusy) {
			return fmt.Errorf("%w: timeout acquiring lock for %s", ErrRepoBusy, op)
		}
		return err
	}
	waitMs := time.Since(startWait).Milliseconds()
	if waitMs > 0 {
		memoryGitLogf("lock acquired op=%s wait_ms=%d repo=%s", op, waitMs, g.dir)
	}

	startOp := time.Now()
	defer func() {
		lock.release()
		memoryGitLogf("op finished op=%s duration_ms=%d repo=%s", op, time.Since(startOp).Milliseconds(), g.dir)
	}()

	return fn()
}

func (g *GitRepo) acquireRepoLock(timeout time.Duration) (*repoFileLock, error) {
	if timeout <= 0 {
		timeout = repoLockTimeout
	}
	lockPath := filepath.Join(g.dir, ".git", repoLockFileName)
	deadline := time.Now().Add(timeout)

	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			meta := repoLockMeta{PID: os.Getpid(), CreatedAt: time.Now().UnixMilli()}
			if b, marshalErr := json.Marshal(meta); marshalErr == nil {
				_, _ = f.Write(b)
			}
			return &repoFileLock{path: lockPath, file: f}, nil
		}

		if !os.IsExist(err) {
			return nil, fmt.Errorf("create repo lock: %w", err)
		}

		if info, statErr := os.Stat(lockPath); statErr == nil {
			if time.Since(info.ModTime()) > repoLockStaleAfter {
				_ = os.Remove(lockPath)
				continue
			}
		}

		if time.Now().After(deadline) {
			return nil, ErrRepoBusy
		}
		time.Sleep(repoLockRetry)
	}
}

func memoryGitLogf(format string, args ...interface{}) {
	if hlog.InfoLog != nil {
		hlog.InfoLog.Printf("[memory-git] "+format, args...)
	}
}
