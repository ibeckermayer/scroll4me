package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config holds all application configuration
type Config struct {
	Version   int             `toml:"version"`
	Interests InterestsConfig `toml:"interests"`
	Scraping  ScrapingConfig  `toml:"scraping"`
	Analysis  AnalysisConfig  `toml:"analysis"`
	Digest    DigestConfig    `toml:"digest"`
	Email     EmailConfig     `toml:"email"`
}

type InterestsConfig struct {
	Keywords         []string `toml:"keywords"`
	PriorityAccounts []string `toml:"priority_accounts"`
	MutedAccounts    []string `toml:"muted_accounts"`
	MutedKeywords    []string `toml:"muted_keywords"`
}

type ScrapingConfig struct {
	PostsPerScrape      int  `toml:"posts_per_scrape"`
	ScrapeIntervalHours int  `toml:"scrape_interval_hours"`
	Headless            bool `toml:"headless"`
}

type AnalysisConfig struct {
	LLMProvider        string  `toml:"llm_provider"`
	APIKey             string  `toml:"api_key"`
	Model              string  `toml:"model"`
	RelevanceThreshold float64 `toml:"relevance_threshold"`
	BatchSize          int     `toml:"batch_size"`
}

type DigestConfig struct {
	MorningTime       string `toml:"morning_time"`
	EveningTime       string `toml:"evening_time"`
	Timezone          string `toml:"timezone"`
	MaxPostsPerDigest int    `toml:"max_posts_per_digest"`
	IncludeContext    bool   `toml:"include_context"`
}

type EmailConfig struct {
	Provider string `toml:"provider"`
	SMTPHost string `toml:"smtp_host"`
	SMTPPort int    `toml:"smtp_port"`
	SMTPUser string `toml:"smtp_user"`
	SMTPPass string `toml:"smtp_pass"`
	FromAddr string `toml:"from_address"`
	ToAddr   string `toml:"to_address"`
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
	return encoder.Encode(c)
}
