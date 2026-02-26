package memory

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Frontmatter is optional YAML metadata at the top of a memory file.
type Frontmatter struct {
	Description string                 `yaml:"description,omitempty"`
	ReadOnly    bool                   `yaml:"read-only,omitempty"`
	Tags        []string               `yaml:"tags,omitempty"`
	Source      string                 `yaml:"source,omitempty"`
	Limit       int                    `yaml:"limit,omitempty"`
	Metadata    map[string]interface{} `yaml:"metadata,omitempty"`
	Extra       map[string]interface{} `yaml:"-"`
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

	var raw map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlBlock), &raw); err != nil {
		return Frontmatter{}, content
	}
	if raw == nil {
		raw = map[string]interface{}{}
	}

	fm := Frontmatter{}
	usedKeys := map[string]struct{}{}

	if v, ok := raw["description"]; ok {
		usedKeys["description"] = struct{}{}
		if s, ok := v.(string); ok {
			fm.Description = s
		}
	}
	if v, ok := raw["read-only"]; ok {
		usedKeys["read-only"] = struct{}{}
		fm.ReadOnly = parseBool(v)
	}
	if v, ok := raw["read_only"]; ok {
		usedKeys["read_only"] = struct{}{}
		fm.ReadOnly = parseBool(v)
	}
	if v, ok := raw["tags"]; ok {
		usedKeys["tags"] = struct{}{}
		fm.Tags = parseStringSlice(v)
	}
	if v, ok := raw["source"]; ok {
		usedKeys["source"] = struct{}{}
		if s, ok := v.(string); ok {
			fm.Source = s
		}
	}
	if v, ok := raw["limit"]; ok {
		usedKeys["limit"] = struct{}{}
		fm.Limit = parseInt(v)
	}
	if v, ok := raw["metadata"]; ok {
		usedKeys["metadata"] = struct{}{}
		if m, ok := v.(map[string]interface{}); ok {
			fm.Metadata = m
		}
	}

	extra := map[string]interface{}{}
	for k, v := range raw {
		if _, ok := usedKeys[k]; ok {
			continue
		}
		extra[k] = v
	}
	if len(extra) > 0 {
		fm.Extra = extra
	}

	return fm, body
}

// FormatFrontmatter prepends a YAML frontmatter block to body.
// If fm is zero-value, body is returned unchanged.
func FormatFrontmatter(fm Frontmatter, body string) string {
	if fm.Description == "" && !fm.ReadOnly && len(fm.Tags) == 0 && fm.Source == "" && fm.Limit == 0 && len(fm.Metadata) == 0 && len(fm.Extra) == 0 {
		return body
	}

	front := map[string]interface{}{}
	if fm.Description != "" {
		front["description"] = fm.Description
	}
	if fm.ReadOnly {
		front["read-only"] = fm.ReadOnly
	}
	if len(fm.Tags) > 0 {
		front["tags"] = fm.Tags
	}
	if fm.Source != "" {
		front["source"] = fm.Source
	}
	if fm.Limit > 0 {
		front["limit"] = fm.Limit
	}
	if len(fm.Metadata) > 0 {
		front["metadata"] = fm.Metadata
	}
	for k, v := range fm.Extra {
		if _, exists := front[k]; !exists {
			front[k] = v
		}
	}

	data, err := yaml.Marshal(front)
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

func parseBool(v interface{}) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(strings.TrimSpace(t), "true")
	default:
		return false
	}
}

func parseInt(v interface{}) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case float32:
		return int(t)
	case string:
		var parsed int
		_, _ = fmt.Sscanf(strings.TrimSpace(t), "%d", &parsed)
		return parsed
	default:
		return 0
	}
}

func parseStringSlice(v interface{}) []string {
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
