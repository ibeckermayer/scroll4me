package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/ibeckermayer/scroll4me/internal/config"
)

// LLMExchange represents a prompt/response pair for caching
type LLMExchange struct {
	Timestamp time.Time `json:"timestamp"`
	Provider  string    `json:"provider"` // e.g. "claude"
	Model     string    `json:"model"`
	Prompt    string    `json:"prompt"`
	Response  string    `json:"response"`
	Error     string    `json:"error,omitempty"`
}

// LLMCacheDir returns the path to the LLM cache directory.
// On macOS this is ~/Library/Caches/scroll4me/llm/
func LLMCacheDir() (string, error) {
	cacheDir, err := config.CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, "llm"), nil
}

// SaveLLMExchange serializes an LLM exchange to JSON and writes it to a timestamped file.
// Returns the path to the saved file.
func SaveLLMExchange(exchange LLMExchange) (string, error) {
	dir, err := LLMCacheDir()
	if err != nil {
		return "", err
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	// Generate filename with timestamp (using dashes instead of colons for filesystem compatibility)
	filename := time.Now().Format("2006-01-02T15-04-05") + ".json"
	path := filepath.Join(dir, filename)

	// Serialize exchange to JSON with indentation for readability
	data, err := json.MarshalIndent(exchange, "", "  ")
	if err != nil {
		return "", err
	}

	// Write to file
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}

	return path, nil
}
