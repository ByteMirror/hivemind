package memory

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ByteMirror/hivemind/config"
)

// GitEnabledFromConfig returns whether memory git versioning should be enabled.
// Defaults to true when unset.
func GitEnabledFromConfig(cfg *config.Config) bool {
	if cfg == nil || cfg.Memory == nil || cfg.Memory.GitEnabled == nil {
		return true
	}
	return *cfg.Memory.GitEnabled
}

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

	mgr, err := NewManagerWithOptions(memDir, provider, ManagerOptions{
		GitEnabled: GitEnabledFromConfig(cfg),
	})
	if err != nil {
		return nil, err
	}

	// "claude" provider: use the local claude CLI for re-ranking.
	// Works with API key and Max subscription — auth is handled by claude itself.
	if cfg.Memory.EmbeddingProvider == "claude" {
		reranker, rerankErr := NewClaudeReranker(cfg.Memory.ClaudeModel)
		if rerankErr == nil {
			mgr.SetReranker(reranker)
		}
		// If claude binary not found, silently fall back to FTS-only.
	}

	return mgr, nil
}
