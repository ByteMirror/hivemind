package memory

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// StartWatcher watches the memory directory for file changes and triggers
// re-indexing. Returns a stop function. Errors are logged but non-fatal.
func (m *Manager) StartWatcher() (stop func(), err error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return func() {}, err
	}
	if err := watcher.Add(m.dir); err != nil {
		watcher.Close()
		return func() {}, err
	}

	done := make(chan struct{})
	go func() {
		defer watcher.Close()
		// Debounce: collect events and process after 500ms quiet period.
		pending := map[string]struct{}{}
		timer := time.NewTimer(24 * time.Hour) // initially idle
		timer.Stop()

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if !strings.HasSuffix(event.Name, ".md") {
					continue
				}
				rel, err := filepath.Rel(m.dir, event.Name)
				if err != nil {
					continue
				}
				pending[rel] = struct{}{}
				timer.Reset(500 * time.Millisecond)

			case <-timer.C:
				for rel := range pending {
					_ = m.Sync(rel)
				}
				pending = map[string]struct{}{}

			case <-done:
				return
			}
		}
	}()

	return func() { close(done) }, nil
}
