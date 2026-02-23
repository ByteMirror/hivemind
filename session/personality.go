package session

import (
	"os"
	"path/filepath"
	"strings"
)

// BuildSystemPrompt assembles the --append-system-prompt value for a chat agent.
// If not bootstrapped, returns BOOTSTRAP.md content only.
// If bootstrapped, concatenates SOUL.md + IDENTITY.md + USER.md + memory snippets.
// The _ int parameter is reserved for future use (e.g. max token budget).
func BuildSystemPrompt(personalityDir string, state ChatWorkspaceState, memorySnippets []string, _ int) (string, error) {
	if !state.Bootstrapped {
		content, err := readFileIfExists(filepath.Join(personalityDir, "BOOTSTRAP.md"))
		if err != nil {
			return "", err
		}
		return content, nil
	}

	var sb strings.Builder

	for _, name := range []string{"SOUL.md", "IDENTITY.md", "USER.md"} {
		content, err := readFileIfExists(filepath.Join(personalityDir, name))
		if err != nil {
			return "", err
		}
		if content == "" {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("## ")
		sb.WriteString(name)
		sb.WriteString("\n")
		sb.WriteString(content)
		sb.WriteString("\n")
	}

	if len(memorySnippets) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("## Recent Memory\n")
		for _, snippet := range memorySnippets {
			sb.WriteString(snippet)
			sb.WriteString("\n")
		}
	}

	result := strings.TrimRight(sb.String(), "\n")
	return result, nil
}

// readFileIfExists reads a file, returning "" (not an error) if it doesn't exist.
func readFileIfExists(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}
