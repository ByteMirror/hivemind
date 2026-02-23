package session

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/ByteMirror/hivemind/memory"
)

const defaultSystemBudget = 4000

var (
	globalMemMgr    *memory.Manager
	globalMemCount  int
	globalSysBudget int
	memMu           sync.RWMutex

	repoMemMgrs map[string]*memory.Manager // key: repo slug
	memFactory  func(dir string) (*memory.Manager, error)
)

// SetMemoryManager configures the memory manager used for startup injection.
// Called once from app.go when the TUI starts.
func SetMemoryManager(mgr *memory.Manager, count, systemBudget int) {
	memMu.Lock()
	defer memMu.Unlock()
	globalMemMgr = mgr
	globalMemCount = count
	globalSysBudget = systemBudget
}

// SetMemoryFactory stores a function used to create new Managers for per-repo dirs.
// If not set, GetOrCreateRepoManager falls back to creating a keyword-only (FTS) manager.
func SetMemoryFactory(fn func(dir string) (*memory.Manager, error)) {
	memMu.Lock()
	defer memMu.Unlock()
	memFactory = fn
}

func getMemoryManager() *memory.Manager {
	memMu.RLock()
	defer memMu.RUnlock()
	return globalMemMgr
}

func getMemoryInjectCount() int {
	memMu.RLock()
	defer memMu.RUnlock()
	if globalMemCount <= 0 {
		return 5
	}
	return globalMemCount
}

func getSystemBudget() int {
	memMu.RLock()
	defer memMu.RUnlock()
	if globalSysBudget <= 0 {
		return defaultSystemBudget
	}
	return globalSysBudget
}

// GetMemoryManager returns the application-wide memory manager, or nil if memory is disabled.
// It is safe for concurrent use.
func GetMemoryManager() *memory.Manager {
	return getMemoryManager()
}

// GetOrCreateRepoManager returns (creating if necessary) a Manager for the given repo slug.
// The repo memory dir is ~/.hivemind/memory/repos/{slug}/.
// Returns nil if memory is globally disabled (no global manager set).
func GetOrCreateRepoManager(slug string) (*memory.Manager, error) {
	memMu.RLock()
	// Memory system must be active (global manager set) to create repo managers.
	if globalMemMgr == nil {
		memMu.RUnlock()
		return nil, nil
	}
	if repoMemMgrs != nil {
		if mgr, ok := repoMemMgrs[slug]; ok {
			memMu.RUnlock()
			return mgr, nil
		}
	}
	// Capture under read lock to avoid a nil-pointer race after unlock.
	globalMgrLocal := globalMemMgr
	factory := memFactory
	memMu.RUnlock()

	// Compute repo dir from the global manager's parent dir.
	globalDir := globalMgrLocal.Dir() // safe: local copy captured under read lock
	repoDir := filepath.Join(globalDir, "repos", slug)
	if err := os.MkdirAll(repoDir, 0700); err != nil {
		return nil, fmt.Errorf("create repo memory dir: %w", err)
	}

	var mgr *memory.Manager
	var err error
	if factory != nil {
		mgr, err = factory(repoDir)
	} else {
		// Fallback: create a keyword-only (FTS) manager using the same dir structure.
		mgr, err = memory.NewManager(repoDir, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("create repo memory manager for %q: %w", slug, err)
	}

	memMu.Lock()
	defer memMu.Unlock()
	if repoMemMgrs == nil {
		repoMemMgrs = make(map[string]*memory.Manager)
	}
	// Check again under write lock in case another goroutine raced.
	if existing, ok := repoMemMgrs[slug]; ok {
		// Another goroutine already created one — close the duplicate and return existing.
		mgr.Close()
		return existing, nil
	}
	repoMemMgrs[slug] = mgr
	return mgr, nil
}

// CloseAllRepoManagers closes and removes all per-repo memory managers.
// Should be called on TUI shutdown.
func CloseAllRepoManagers() {
	memMu.Lock()
	defer memMu.Unlock()
	for slug, mgr := range repoMemMgrs {
		mgr.Close()
		delete(repoMemMgrs, slug)
	}
}
