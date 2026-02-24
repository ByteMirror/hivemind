package session

import (
	"sync"

	"github.com/ByteMirror/hivemind/memory"
)

const defaultSystemBudget = 4000

var (
	globalMemMgr    *memory.Manager
	globalMemCount  int
	globalSysBudget int
	memMu           sync.RWMutex
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
