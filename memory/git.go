package memory

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrNoChanges is returned by AutoCommit when the working tree is clean.
var ErrNoChanges = errors.New("no changes to commit")

// GitRepo manages a git repository for memory versioning.
type GitRepo struct {
	dir string
}

// GitLogEntry represents a single commit in the memory git log.
type GitLogEntry struct {
	SHA     string   `json:"sha"`
	Message string   `json:"message"`
	Date    string   `json:"date"`
	Files   []string `json:"files,omitempty"`
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
	if _, err := g.gitExec("add", "-A"); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	// Check if there's anything to commit.
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
	if count <= 0 {
		count = 10
	}

	args := []string{"log", fmt.Sprintf("--max-count=%d", count),
		"--format=%H\x1f%s\x1f%aI", "--name-only"}
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

	return parseGitLog(out), nil
}

func parseGitLog(raw string) []GitLogEntry {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	// Commits are separated by blank lines. Each commit block looks like:
	// SHA\x1fSubject\x1fDate
	// file1
	// file2
	var entries []GitLogEntry
	blocks := strings.Split(raw, "\n\n")
	for _, block := range blocks {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		if len(lines) == 0 {
			continue
		}
		parts := strings.SplitN(lines[0], "\x1f", 3)
		if len(parts) < 3 {
			continue
		}
		entry := GitLogEntry{
			SHA:     parts[0],
			Message: parts[1],
			Date:    parts[2],
		}
		for _, l := range lines[1:] {
			l = strings.TrimSpace(l)
			if l != "" {
				entry.Files = append(entry.Files, l)
			}
		}
		entries = append(entries, entry)
	}
	return entries
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
