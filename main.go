package main

import (
	"log"
	"os"

	"github.com/getlantern/systray"

	"github.com/ibeckermayer/scroll4me/internal/analyzer"
	"github.com/ibeckermayer/scroll4me/internal/analyzer/providers"
	"github.com/ibeckermayer/scroll4me/internal/app"
	"github.com/ibeckermayer/scroll4me/internal/auth"
	"github.com/ibeckermayer/scroll4me/internal/config"
	"github.com/ibeckermayer/scroll4me/internal/scraper"
	"github.com/ibeckermayer/scroll4me/internal/tray"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Load or create configuration
	cfg, err := config.Load()
	if err != nil {
		if os.IsNotExist(err) {
			// First run - create default config
			cfg = config.Default()
			if err := cfg.Save(); err != nil {
				log.Printf("Warning: could not save default config: %v", err)
			} else {
				path, _ := config.ConfigPath()
				log.Printf("Created default config at: %s", path)
			}
		} else {
			log.Printf("Warning: could not load config: %v (using defaults)", err)
			cfg = config.Default()
		}
	}

	// Initialize cookie store and auth manager
	cookieStorePath, err := auth.DefaultCookieStorePath()
	if err != nil {
		log.Fatalf("Failed to get cookie store path: %v", err)
	}
	cookieStore := auth.NewCookieStore(cookieStorePath)
	authManager := auth.NewManager(cookieStore)

	// Initialize scraper
	postScraper := scraper.New(cfg.Scraping.Headless)

	// Initialize analyzer
	provider := providers.NewClaudeProvider(cfg.Analysis.APIKey, cfg.Analysis.Model)
	postAnalyzer := analyzer.New(provider, cfg.Interests, cfg.Analysis.BatchSize)

	// Create app
	a := app.New(cfg, authManager, postScraper, postAnalyzer)

	log.Println("scroll4me starting...")

	// Run systray (blocks until Quit)
	systray.Run(tray.OnReady(a), tray.OnExit)
}
