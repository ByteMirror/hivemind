package git

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
)

// Pre-compiled regexes for branch name sanitization.
var (
	unsafeCharsRegex = regexp.MustCompile(`[^a-z0-9\-_/.]+`)
	multiDashRegex   = regexp.MustCompile(`-+`)
)

// sanitizeBranchName transforms an arbitrary string into a Git branch name friendly string.
func sanitizeBranchName(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = unsafeCharsRegex.ReplaceAllString(s, "")
	s = multiDashRegex.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-/")
	return s
}

// isValidBranchName performs a conservative validation for git branch names.
// It intentionally rejects ambiguous or unsafe forms before we pass them to git.
func isValidBranchName(s string) bool {
	if s == "" || s == "@" {
		return false
	}
	if strings.HasPrefix(s, "/") || strings.HasSuffix(s, "/") {
		return false
	}
	if strings.Contains(s, "//") || strings.Contains(s, "..") || strings.Contains(s, "@{") {
		return false
	}
	if strings.ContainsAny(s, " ~^:?*[\\'\"`") {
		return false
	}
	parts := strings.Split(s, "/")
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			return false
		}
		if strings.HasPrefix(p, ".") || strings.HasSuffix(p, ".") {
			return false
		}
		if strings.HasSuffix(p, ".lock") {
			return false
		}
	}
	return true
}

// makeSafeBranchName derives a valid branch name from prefix+sessionName.
// It never returns an empty or invalid branch name.
func makeSafeBranchName(prefix, sessionName string) string {
	p := sanitizeBranchName(prefix)
	p = strings.Trim(p, "-/")
	if p != "" {
		p += "/"
	}

	candidate := sanitizeBranchName(p + sessionName)
	if isValidBranchName(candidate) {
		return candidate
	}

	base := sanitizeBranchName(sessionName)
	base = strings.Trim(base, "-/")
	if base == "" {
		base = "session"
	}
	suffix := fmt.Sprintf("%x", time.Now().UnixNano())

	if p != "" {
		candidate = p + base + "-" + suffix
		if isValidBranchName(candidate) {
			return candidate
		}
	}

	candidate = "session/" + base + "-" + suffix
	if isValidBranchName(candidate) {
		return candidate
	}

	// Final fallback should always be valid.
	return "session-" + suffix
}

// checkGHCLI checks if GitHub CLI is installed and configured
func checkGHCLI() error {
	// Check if gh is installed
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("GitHub CLI (gh) is not installed. Please install it first")
	}

	// Check if gh is authenticated
	cmd := exec.Command("gh", "auth", "status")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("GitHub CLI is not configured. Please run 'gh auth login' first")
	}

	return nil
}

// IsGitRepo checks if the given path is within a git repository
func IsGitRepo(path string) bool {
	_, err := findGitRepoRoot(path)
	return err == nil
}

func findGitRepoRoot(path string) (string, error) {
	currentPath := path
	for {
		_, err := git.PlainOpen(currentPath)
		if err == nil {
			// Found the repository root
			return currentPath, nil
		}

		parent := filepath.Dir(currentPath)
		if parent == currentPath {
			// Reached the filesystem root without finding a repository
			return "", fmt.Errorf("failed to find Git repository root from path: %s", path)
		}
		currentPath = parent
	}
}
