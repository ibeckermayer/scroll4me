package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/anthropics/anthropic-sdk-go"
)

// Config holds all application configuration
type Config struct {
	Version   int             `toml:"version"`
	Interests InterestsConfig `toml:"interests"`
	Scraping  ScrapingConfig  `toml:"scraping"`
	Analysis  AnalysisConfig  `toml:"analysis"`
	Digest    DigestConfig    `toml:"digest"`
}

type InterestsConfig struct {
	Keywords         []string `toml:"keywords"`
	PriorityAccounts []string `toml:"priority_accounts"`
	MutedAccounts    []string `toml:"muted_accounts"`
	MutedKeywords    []string `toml:"muted_keywords"`
}

type ScrapingConfig struct {
	PostsPerScrape        int  `toml:"posts_per_scrape"`
	Headless              bool `toml:"headless"`
	DebugPauseAfterScrape bool `toml:"debug_pause_after_scrape"`
}

type AnalysisConfig struct {
	LLMProvider        string  `toml:"llm_provider"`
	APIKey             string  `toml:"api_key"`
	Model              string  `toml:"model"`
	RelevanceThreshold float64 `toml:"relevance_threshold"`
	BatchSize          int     `toml:"batch_size"`
}

type DigestConfig struct {
	OutputDir      string `toml:"output_dir"`
	MaxPosts       int    `toml:"max_posts"`
	IncludeContext bool   `toml:"include_context"`
}

// Default returns a Config with sensible defaults
func Default() *Config {
	outputDir, _ := DefaultDigestDir()
	return &Config{
		Version: 1,
		Interests: InterestsConfig{
			Keywords:         []string{},
			PriorityAccounts: []string{},
			MutedAccounts:    []string{},
			MutedKeywords:    []string{},
		},
		Scraping: ScrapingConfig{
			PostsPerScrape:        100,
			Headless:              true,
			DebugPauseAfterScrape: false,
		},
		Analysis: AnalysisConfig{
			LLMProvider:        "claude",
			Model:              string(anthropic.ModelClaudeSonnet4_5_20250929),
			APIKey:             "<replace with your API key>",
			RelevanceThreshold: 0.6,
			BatchSize:          10,
		},
		Digest: DigestConfig{
			OutputDir:      outputDir,
			MaxPosts:       20,
			IncludeContext: true,
		},
	}
}

// ConfigDir returns the platform-appropriate config directory
func ConfigDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "scroll4me"), nil
}

// CacheDir returns the platform-appropriate cache directory.
// On macOS this is ~/Library/Caches/scroll4me/
func CacheDir() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, "scroll4me"), nil
}

// DefaultDigestDir returns the default digest output directory
func DefaultDigestDir() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "digests"), nil
}

// ConfigPath returns the full path to the config file
func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// Load reads config from disk
func Load() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Save writes config to disk
func (c *Config) Save() error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	path, err := ConfigPath()
	if err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := toml.NewEncoder(f)
	encoder.Indent = ""
	return encoder.Encode(c)
}
