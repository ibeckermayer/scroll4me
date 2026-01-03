package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/ibeckermayer/scroll4me/internal/config"
	"github.com/ibeckermayer/scroll4me/internal/types"
)

// PostsCacheDir returns the path to the posts cache directory.
// On macOS this is ~/Library/Caches/scroll4me/posts/
func PostsCacheDir() (string, error) {
	cacheDir, err := config.CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, "posts"), nil
}

// SavePosts serializes posts to JSON and writes them to a timestamped file.
// Returns the path to the saved file.
func SavePosts(posts []types.Post) (string, error) {
	dir, err := PostsCacheDir()
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

	// Serialize posts to JSON with indentation for readability
	data, err := json.MarshalIndent(posts, "", "  ")
	if err != nil {
		return "", err
	}

	// Write to file
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}

	return path, nil
}
