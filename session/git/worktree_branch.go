package git

import (
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// cleanupExistingBranch performs a thorough cleanup of any existing branch or reference
func (g *GitWorktree) cleanupExistingBranch(repo *git.Repository) error {
	branchRef := plumbing.NewBranchReferenceName(g.branchName)

	// Try to remove the branch reference. go-git's RemoveReference deletes the
	// ref file and then tries to prune empty parent directories. With namespaced
	// branches (e.g. "user/task"), the parent directory may still contain sibling
	// branches, causing a "directory not empty" error. This is harmless — the
	// reference itself was already removed.
	if err := repo.Storer.RemoveReference(branchRef); err != nil && err != plumbing.ErrReferenceNotFound {
		if !strings.Contains(err.Error(), "directory not empty") {
			return fmt.Errorf("failed to remove branch reference %s: %w", g.branchName, err)
		}
	}

	// Remove any worktree-specific references
	worktreeRef := plumbing.NewReferenceFromStrings(
		fmt.Sprintf("worktrees/%s/HEAD", g.branchName),
		"",
	)
	if err := repo.Storer.RemoveReference(worktreeRef.Name()); err != nil && err != plumbing.ErrReferenceNotFound {
		if !strings.Contains(err.Error(), "directory not empty") {
			return fmt.Errorf("failed to remove worktree reference for %s: %w", g.branchName, err)
		}
	}

	// Clean up configuration entries
	cfg, err := repo.Config()
	if err != nil {
		return fmt.Errorf("failed to get repository config: %w", err)
	}

	delete(cfg.Branches, g.branchName)
	worktreeSection := fmt.Sprintf("worktree.%s", g.branchName)
	cfg.Raw.RemoveSection(worktreeSection)

	if err := repo.Storer.SetConfig(cfg); err != nil {
		return fmt.Errorf("failed to update repository config after removing branch %s: %w", g.branchName, err)
	}

	return nil
}
