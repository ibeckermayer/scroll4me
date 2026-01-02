package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config holds all application configuration
type Config struct {
	Version   int             `json:"version"`
	Interests InterestsConfig `json:"interests"`
	Scraping  ScrapingConfig  `json:"scraping"`
	Analysis  AnalysisConfig  `json:"analysis"`
	Digest    DigestConfig    `json:"digest"`
	Email     EmailConfig     `json:"email"`
}

type InterestsConfig struct {
	Keywords         []string `json:"keywords"`
	PriorityAccounts []string `json:"priority_accounts"`
	MutedAccounts    []string `json:"muted_accounts"`
	MutedKeywords    []string `json:"muted_keywords"`
}

type ScrapingConfig struct {
	PostsPerScrape      int  `json:"posts_per_scrape"`
	ScrapeIntervalHours int  `json:"scrape_interval_hours"`
	Headless            bool `json:"headless"`
}

type AnalysisConfig struct {
	LLMProvider        string  `json:"llm_provider"`
	APIKey             string  `json:"api_key"`
	Model              string  `json:"model"`
	RelevanceThreshold float64 `json:"relevance_threshold"`
	BatchSize          int     `json:"batch_size"`
}

type DigestConfig struct {
	MorningTime       string `json:"morning_time"`
	EveningTime       string `json:"evening_time"`
	Timezone          string `json:"timezone"`
	MaxPostsPerDigest int    `json:"max_posts_per_digest"`
	IncludeContext    bool   `json:"include_context"`
}

type EmailConfig struct {
	Provider string `json:"provider"`
	SMTPHost string `json:"smtp_host"`
	SMTPPort int    `json:"smtp_port"`
	SMTPUser string `json:"smtp_user"`
	SMTPPass string `json:"smtp_pass"`
	FromAddr string `json:"from_address"`
	ToAddr   string `json:"to_address"`
}

// Default returns a Config with sensible defaults
func Default() *Config {
	return &Config{
		Version: 1,
		Interests: InterestsConfig{
			Keywords:         []string{},
			PriorityAccounts: []string{},
			MutedAccounts:    []string{},
			MutedKeywords:    []string{},
		},
		Scraping: ScrapingConfig{
			PostsPerScrape:      100,
			ScrapeIntervalHours: 2,
			Headless:            true,
		},
		Analysis: AnalysisConfig{
			LLMProvider:        "claude",
			Model:              "claude-sonnet-4-20250514",
			RelevanceThreshold: 0.6,
			BatchSize:          10,
		},
		Digest: DigestConfig{
			MorningTime:       "07:00",
			EveningTime:       "18:00",
			Timezone:          "America/New_York",
			MaxPostsPerDigest: 20,
			IncludeContext:    true,
		},
		Email: EmailConfig{
			Provider: "smtp",
			SMTPPort: 587,
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

// ConfigPath returns the full path to the config file
func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Load reads config from disk
func Load() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
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

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}
