package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ibeckermayer/scroll4me/internal/config"
)

// StepName identifies a pipeline step for caching purposes.
type StepName string

const (
	Step1Posts    StepName = "step1_posts"
	Step2Analyses StepName = "step2_analyses"
	Step3Filtered StepName = "step3_filtered"
	Step4Context  StepName = "step4_context"
	Step5Digests  StepName = "step5_digests"
)

// stepDir returns the cache directory for a given step.
func stepDir(step StepName) (string, error) {
	cacheDir, err := config.CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, string(step)), nil
}

// generateFilename creates a timestamped filename with the given extension.
func generateFilename(ext string) string {
	return time.Now().Format("2006-01-02T15-04-05") + ext
}

// SaveStepOutput saves JSON-serializable data to the step's cache directory.
// Returns the path to the saved file.
func SaveStepOutput[T any](step StepName, data T) (string, error) {
	dir, err := stepDir(step)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create step cache dir: %w", err)
	}

	path := filepath.Join(dir, generateFilename(".json"))

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal step output: %w", err)
	}

	if err := os.WriteFile(path, jsonData, 0644); err != nil {
		return "", fmt.Errorf("failed to write step output: %w", err)
	}

	return path, nil
}

// SaveTextOutput saves text content (e.g., markdown) to the step's cache directory.
// Returns the path to the saved file.
func SaveTextOutput(step StepName, content string, ext string) (string, error) {
	dir, err := stepDir(step)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create step cache dir: %w", err)
	}

	path := filepath.Join(dir, generateFilename(ext))

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write step output: %w", err)
	}

	return path, nil
}

// LoadLatestStepOutput loads the most recent output from a step's cache directory.
// Returns the data, the filepath it was loaded from, and any error.
func LoadLatestStepOutput[T any](step StepName) (T, string, error) {
	var zero T

	latestPath, err := LatestStepFile(step)
	if err != nil {
		return zero, "", err
	}

	data, err := LoadStepOutput[T](latestPath)
	if err != nil {
		return zero, "", err
	}

	return data, latestPath, nil
}

// LoadStepOutput loads JSON data from a specific file path.
func LoadStepOutput[T any](filepath string) (T, error) {
	var data T

	jsonData, err := os.ReadFile(filepath)
	if err != nil {
		return data, fmt.Errorf("failed to read step output: %w", err)
	}

	if err := json.Unmarshal(jsonData, &data); err != nil {
		return data, fmt.Errorf("failed to unmarshal step output: %w", err)
	}

	return data, nil
}

// LatestStepFile returns the path to the most recent file in a step's cache directory.
func LatestStepFile(step StepName) (string, error) {
	dir, err := stepDir(step)
	if err != nil {
		return "", err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no cached output for step %s", step)
		}
		return "", err
	}

	// Filter to regular files (os.ReadDir already sorts by name, which is chronological for our timestamps)
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry.Name())
		}
	}

	if len(files) == 0 {
		return "", fmt.Errorf("no cached output for step %s", step)
	}

	latestFile := files[len(files)-1]

	return filepath.Join(dir, latestFile), nil
}
