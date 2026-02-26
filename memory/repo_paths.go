package memory

import (
	"fmt"
	"os"
	"path/filepath"
)

// RepoStoreResolution describes canonical and legacy per-repo memory locations.
// LegacyPath is only set when both canonical and legacy dirs currently exist.
type RepoStoreResolution struct {
	CanonicalSlug string
	CanonicalPath string
	LegacySlug    string
	LegacyPath    string
}

// ResolveRepoStorePaths resolves canonical/legacy repo memory dirs and performs
// one-time migration from legacy (worktree-derived slug) to canonical
// (repo-derived slug) when canonical does not yet exist.
func ResolveRepoStorePaths(baseDir, repoPath, worktreePath string) (RepoStoreResolution, error) {
	var res RepoStoreResolution

	canonicalSlug := repoSlug(repoPath)
	if canonicalSlug == "" {
		return res, nil
	}
	res.CanonicalSlug = canonicalSlug
	res.CanonicalPath = filepath.Join(baseDir, "repos", canonicalSlug)

	legacySlug := repoSlug(worktreePath)
	if legacySlug == "" || legacySlug == canonicalSlug {
		return res, nil
	}
	res.LegacySlug = legacySlug
	legacyPath := filepath.Join(baseDir, "repos", legacySlug)

	canonicalExists, err := dirExists(res.CanonicalPath)
	if err != nil {
		return RepoStoreResolution{}, err
	}
	legacyExists, err := dirExists(legacyPath)
	if err != nil {
		return RepoStoreResolution{}, err
	}
	if !legacyExists {
		return res, nil
	}

	// One-time migration: if canonical does not exist yet, move legacy into place.
	if !canonicalExists {
		if err := os.MkdirAll(filepath.Dir(res.CanonicalPath), 0700); err != nil {
			return RepoStoreResolution{}, fmt.Errorf("create canonical parent dir: %w", err)
		}
		if err := os.Rename(legacyPath, res.CanonicalPath); err != nil {
			return RepoStoreResolution{}, fmt.Errorf("migrate legacy repo memory: %w", err)
		}
		return res, nil
	}

	// Non-destructive transition mode when both exist.
	res.LegacyPath = legacyPath
	return res, nil
}

func repoSlug(path string) string {
	if path == "" {
		return ""
	}
	base := filepath.Base(filepath.Clean(path))
	if base == "" || base == "." || base == string(filepath.Separator) {
		return ""
	}
	return base
}

func dirExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err == nil {
		return info.IsDir(), nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
