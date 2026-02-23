package session

import (
	"sync"

	"github.com/ByteMirror/hivemind/memory"
)

var (
	globalMemMgr   *memory.Manager
	globalMemCount int
	memMu          sync.RWMutex
)

// SetMemoryManager configures the memory manager used for startup injection.
// Called once from app.go when the TUI starts.
func SetMemoryManager(mgr *memory.Manager, count int) {
	memMu.Lock()
	defer memMu.Unlock()
	globalMemMgr = mgr
	globalMemCount = count
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
