package memory

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// Frontmatter is optional YAML metadata at the top of a memory file.
type Frontmatter struct {
	Description string `yaml:"description,omitempty"`
	ReadOnly    bool   `yaml:"read-only,omitempty"`
}

// ParseFrontmatter extracts YAML frontmatter from content.
// If no frontmatter is present, it returns a zero-value Frontmatter and the
// full content unchanged. Frontmatter must be delimited by "---" on its own line.
func ParseFrontmatter(content string) (Frontmatter, string) {
	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		return Frontmatter{}, content
	}

	// Find closing delimiter. Start searching after the opening "---\n".
	rest := content[4:]
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		// Try with trailing content (file ends right after ---)
		if strings.HasSuffix(rest, "\n---") || rest == "---" {
			idx = len(rest) - 3
		} else {
			return Frontmatter{}, content
		}
	}

	yamlBlock := rest[:idx]
	body := rest[idx+4:] // skip "\n---\n"
	if strings.HasPrefix(body, "\n") {
		body = body[1:] // strip leading blank line after closing ---
	}

	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(yamlBlock), &fm); err != nil {
		return Frontmatter{}, content
	}
	return fm, body
}

// FormatFrontmatter prepends a YAML frontmatter block to body.
// If fm is zero-value, body is returned unchanged.
func FormatFrontmatter(fm Frontmatter, body string) string {
	if fm.Description == "" && !fm.ReadOnly {
		return body
	}

	data, err := yaml.Marshal(&fm)
	if err != nil {
		return body
	}

	var b strings.Builder
	b.WriteString("---\n")
	b.Write(data)
	b.WriteString("---\n")
	b.WriteString(body)
	return b.String()
}

// ReadFileFrontmatter reads a memory file's frontmatter without loading the
// full file. It reads up to maxBytes from the start of the file.
func (m *Manager) ReadFileFrontmatter(relPath string) (Frontmatter, error) {
	absPath, err := m.absPath(relPath)
	if err != nil {
		return Frontmatter{}, err
	}

	data, err := readFileHead(absPath, 512)
	if err != nil {
		return Frontmatter{}, err
	}

	fm, _ := ParseFrontmatter(string(data))
	return fm, nil
}
