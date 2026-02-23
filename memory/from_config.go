package memory

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ByteMirror/hivemind/config"
)

// NewManagerFromConfig creates a MemoryManager from the application config.
// Returns (nil, nil) if memory is disabled or not configured.
func NewManagerFromConfig(cfg *config.Config) (*Manager, error) {
	if cfg.Memory == nil || !cfg.Memory.Enabled {
		return nil, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("memory: get home dir: %w", err)
	}
	memDir := filepath.Join(homeDir, ".hivemind", "memory")

	var provider EmbeddingProvider
	switch cfg.Memory.EmbeddingProvider {
	case "openai":
		if cfg.Memory.OpenAIAPIKey == "" {
			return nil, fmt.Errorf("memory: openai provider requires openai_api_key in config")
		}
		provider = NewOpenAIProvider(cfg.Memory.OpenAIAPIKey, cfg.Memory.OpenAIModel)
	case "ollama":
		provider = NewOllamaProvider(cfg.Memory.OllamaURL, cfg.Memory.OllamaModel)
	default:
		provider = &noopProvider{} // FTS-only
	}

	return NewManager(memDir, provider)
}
