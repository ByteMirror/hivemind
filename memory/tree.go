package memory

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// TreeEntry describes a single file in the memory tree.
type TreeEntry struct {
	Path        string `json:"path"`
	Description string `json:"description,omitempty"`
	SizeBytes   int64  `json:"size_bytes"`
	IsSystem    bool   `json:"is_system"`
}

// Tree returns all .md files in the memory directory with frontmatter descriptions.
func (m *Manager) Tree() ([]TreeEntry, error) {
	var entries []TreeEntry
	err := filepath.WalkDir(m.dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		name := d.Name()
		if d.IsDir() {
			if name == ".git" || name == ".index" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".md") {
			return nil
		}

		rel, err := filepath.Rel(m.dir, path)
		if err != nil {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		// Read just the head of the file for frontmatter.
		desc := ""
		if data, readErr := readFileHead(path, 512); readErr == nil {
			fm, _ := ParseFrontmatter(string(data))
			desc = fm.Description
		}

		entries = append(entries, TreeEntry{
			Path:        rel,
			Description: desc,
			SizeBytes:   info.Size(),
			IsSystem:    strings.HasPrefix(rel, "system/") || strings.HasPrefix(rel, "system\\"),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk memory dir: %w", err)
	}
	return entries, nil
}

// FormatTree renders entries as a compact ASCII tree suitable for CLAUDE.md injection.
func FormatTree(entries []TreeEntry) string {
	if len(entries) == 0 {
		return "(empty)\n"
	}

	var b strings.Builder
	for _, e := range entries {
		prefix := "  "
		if e.IsSystem {
			prefix = "* "
		}
		sizeKB := float64(e.SizeBytes) / 1024.0
		if e.Description != "" {
			b.WriteString(fmt.Sprintf("%s%-30s %5.1fK  %s\n", prefix, e.Path, sizeKB, e.Description))
		} else {
			b.WriteString(fmt.Sprintf("%s%-30s %5.1fK\n", prefix, e.Path, sizeKB))
		}
	}
	return b.String()
}

// readFileHead reads up to maxBytes from the start of a file.
func readFileHead(absPath string, maxBytes int) ([]byte, error) {
	f, err := os.Open(absPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	buf := make([]byte, maxBytes)
	n, _ := f.Read(buf)
	return buf[:n], nil
}
