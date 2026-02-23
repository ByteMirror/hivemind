package memory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// noopProvider returns nil embeddings (FTS-only mode).
type noopProvider struct{}

func (n *noopProvider) Embed(_ []string) ([][]float32, error) { return nil, nil }
func (n *noopProvider) Dims() int                             { return 0 }
func (n *noopProvider) Name() string                          { return "none" }

// OpenAIProvider calls the OpenAI embeddings API.
type OpenAIProvider struct {
	APIKey string
	Model  string // default "text-embedding-3-small"
	client *http.Client
}

func NewOpenAIProvider(apiKey, model string) *OpenAIProvider {
	if model == "" {
		model = "text-embedding-3-small"
	}
	return &OpenAIProvider{APIKey: apiKey, Model: model, client: &http.Client{Timeout: 30 * time.Second}}
}

func (p *OpenAIProvider) Dims() int    { return 1536 }
func (p *OpenAIProvider) Name() string { return "openai/" + p.Model }

func (p *OpenAIProvider) Embed(texts []string) ([][]float32, error) {
	body, _ := json.Marshal(map[string]any{
		"input": texts,
		"model": p.Model,
	})
	req, _ := http.NewRequest("POST", "https://api.openai.com/v1/embeddings", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai embed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("openai decode: %w", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("openai api: %s", result.Error.Message)
	}
	vecs := make([][]float32, len(result.Data))
	for i, d := range result.Data {
		vecs[i] = d.Embedding
	}
	return vecs, nil
}

// OllamaProvider calls a local Ollama server for embeddings.
type OllamaProvider struct {
	URL    string // e.g. "http://localhost:11434"
	Model  string // e.g. "nomic-embed-text"
	client *http.Client
}

func NewOllamaProvider(url, model string) *OllamaProvider {
	if url == "" {
		url = "http://localhost:11434"
	}
	if model == "" {
		model = "nomic-embed-text"
	}
	return &OllamaProvider{URL: url, Model: model, client: &http.Client{Timeout: 60 * time.Second}}
}

func (p *OllamaProvider) Dims() int    { return 768 }
func (p *OllamaProvider) Name() string { return "ollama/" + p.Model }

func (p *OllamaProvider) Embed(texts []string) ([][]float32, error) {
	vecs := make([][]float32, 0, len(texts))
	for _, text := range texts {
		vec, err := p.embedOne(text)
		if err != nil {
			return nil, err
		}
		vecs = append(vecs, vec)
	}
	return vecs, nil
}

func (p *OllamaProvider) embedOne(text string) ([]float32, error) {
	body, _ := json.Marshal(map[string]any{
		"model":  p.Model,
		"prompt": text,
	})
	req, _ := http.NewRequest("POST", p.URL+"/api/embeddings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ollama decode: %w", err)
	}
	return result.Embedding, nil
}
