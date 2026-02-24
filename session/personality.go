package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ByteMirror/hivemind/config"
	"github.com/ByteMirror/hivemind/log"
)

const (
	personalityInjectHeader = "<!-- hivemind-personality-start -->"
	personalityInjectFooter = "<!-- hivemind-personality-end -->"
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

// WriteCompanionClaudeMD writes the given content as CLAUDE.md in personalityDir.
// Claude Code reads CLAUDE.md as primary project instructions, giving it higher
// priority than --append-system-prompt. Always overwritten so instructions stay
// in sync with the current bootstrap/identity state.
func WriteCompanionClaudeMD(personalityDir, content string) error {
	dest := filepath.Join(personalityDir, "CLAUDE.md")
	return config.AtomicWriteFile(dest, []byte(content), 0600)
}

// InjectPersonalityContext reads the companion's personality (SOUL.md, IDENTITY.md)
// and upserts it into CLAUDE.md at the given path. This gives every agent — chat
// or coding — the companion's identity so it knows who it is.
//
// The section is wrapped in HTML comment markers so it can be cleanly replaced on
// subsequent starts, following the same pattern as InjectMemoryContext.
//
// If the companion hasn't been bootstrapped yet, this is a no-op.
func InjectPersonalityContext(claudeMDPath string) error {
	companionDir, err := GetAgentPersonalityDir("companion")
	if err != nil {
		return nil // no companion dir — nothing to inject
	}
	state, err := ReadWorkspaceState("companion")
	if err != nil || !state.Bootstrapped {
		return nil // companion not set up yet — skip
	}

	// Read the companion's personality files.
	soul, _ := readFileIfExists(filepath.Join(companionDir, "SOUL.md"))
	identity, _ := readFileIfExists(filepath.Join(companionDir, "IDENTITY.md"))

	if soul == "" && identity == "" {
		return nil // nothing to inject
	}

	section := buildPersonalitySection(soul, identity)
	return upsertPersonalitySection(claudeMDPath, section)
}

// buildPersonalitySection formats the companion personality into a CLAUDE.md section.
func buildPersonalitySection(soul, identity string) string {
	var b strings.Builder
	b.WriteString(personalityInjectHeader + "\n")
	b.WriteString("## Companion Personality\n\n")
	b.WriteString("You are a companion inside Hivemind. Embody the persona and tone described below.\n")
	b.WriteString("Avoid stiff, generic replies — follow this guidance in every interaction.\n\n")

	if identity != "" {
		b.WriteString("### Identity\n")
		b.WriteString(identity)
		if !strings.HasSuffix(identity, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if soul != "" {
		b.WriteString("### Soul\n")
		b.WriteString(soul)
		if !strings.HasSuffix(soul, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(personalityInjectFooter + "\n")
	return b.String()
}

// upsertPersonalitySection writes the personality section into CLAUDE.md,
// replacing any existing block or appending if absent.
func upsertPersonalitySection(claudeMDPath, section string) error {
	var existing string
	if data, err := os.ReadFile(claudeMDPath); err == nil {
		existing = string(data)
	}

	startIdx := strings.Index(existing, personalityInjectHeader)
	endIdx := strings.Index(existing, personalityInjectFooter)

	var updated string
	if startIdx >= 0 && endIdx > startIdx {
		// Replace existing block.
		updated = existing[:startIdx] + section + existing[endIdx+len(personalityInjectFooter)+1:]
	} else {
		// Prepend personality at the top so it's the first thing Claude sees.
		if existing != "" {
			updated = section + "\n" + existing
		} else {
			updated = section
		}
	}

	if err := os.WriteFile(claudeMDPath, []byte(updated), 0600); err != nil {
		return fmt.Errorf("write personality to CLAUDE.md: %w", err)
	}
	log.InfoLog.Printf("personality: injected companion identity into %s", claudeMDPath)
	return nil
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
