package memory

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// StartWatcher watches the memory directory (recursively) for file changes and
// triggers re-indexing + auto-commit. Returns a stop function.
func (m *Manager) StartWatcher() (stop func(), err error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return func() {}, err
	}

	// Recursively add all subdirectories (skip .git/ and .index/).
	if walkErr := addWatchDirs(watcher, m.dir); walkErr != nil {
		watcher.Close()
		return func() {}, walkErr
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

				// Dynamic dir watching: add newly created directories.
				if event.Has(fsnotify.Create) {
					if info, statErr := os.Stat(event.Name); statErr == nil && info.IsDir() {
						name := filepath.Base(event.Name)
						if name != ".git" && name != ".index" {
							_ = watcher.Add(event.Name)
						}
						continue
					}
				}

				if !strings.HasSuffix(event.Name, ".md") {
					continue
				}
				rel, relErr := filepath.Rel(m.dir, event.Name)
				if relErr != nil {
					continue
				}
				pending[rel] = struct{}{}
				timer.Reset(500 * time.Millisecond)

			case <-timer.C:
				for rel := range pending {
					_ = m.Sync(rel)
				}
				if len(pending) > 0 {
					m.autoCommit("memory: auto-sync")
				}
				pending = map[string]struct{}{}

			case <-done:
				return
			}
		}
	}()

	return func() { close(done) }, nil
}

// addWatchDirs walks dir and adds all subdirectories to the watcher,
// skipping .git/ and .index/.
func addWatchDirs(watcher *fsnotify.Watcher, dir string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".index" {
				return filepath.SkipDir
			}
			return watcher.Add(path)
		}
		return nil
	})
}
