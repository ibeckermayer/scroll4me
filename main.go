package main

import (
	"context"
	_ "embed"
	"log"
	"os"
	"sync"

	"github.com/getlantern/systray"
	"github.com/pkg/browser"

	"github.com/ibeckermayer/scroll4me/internal/analyzer"
	"github.com/ibeckermayer/scroll4me/internal/analyzer/providers"
	"github.com/ibeckermayer/scroll4me/internal/auth"
	"github.com/ibeckermayer/scroll4me/internal/config"
	"github.com/ibeckermayer/scroll4me/internal/digest"
	"github.com/ibeckermayer/scroll4me/internal/scraper"
	"github.com/ibeckermayer/scroll4me/internal/types"
)

//go:embed assets/icon.png
var iconBytes []byte

// App holds the application state
type App struct {
	mu          sync.RWMutex
	config      *config.Config
	authManager *auth.Manager
	scraper     *scraper.Scraper
	analyzer    *analyzer.Analyzer
}

// NewApp creates a new App instance
func NewApp(cfg *config.Config, authManager *auth.Manager, sc *scraper.Scraper, an *analyzer.Analyzer) *App {
	return &App{
		config:      cfg,
		authManager: authManager,
		scraper:     sc,
		analyzer:    an,
	}
}

// IsAuthenticated checks if X.com credentials are stored
func (a *App) IsAuthenticated() bool {
	return a.authManager.IsAuthenticated()
}

// TriggerLogin starts the X.com login flow
func (a *App) TriggerLogin() error {
	log.Println("Login triggered - opening browser for X.com authentication")
	ctx := context.Background()
	if err := a.authManager.Login(ctx); err != nil {
		log.Printf("Login failed: %v", err)
		return err
	}
	log.Println("Login successful - cookies saved")
	return nil
}

// TriggerLogout clears stored X.com credentials
func (a *App) TriggerLogout() error {
	log.Println("Logout triggered - clearing stored cookies")
	if err := a.authManager.Logout(); err != nil {
		log.Printf("Logout failed: %v", err)
		return err
	}
	log.Println("Logout successful - cookies cleared")
	return nil
}

// GenerateDigest performs the full scrape -> analyze -> build digest flow
func (a *App) GenerateDigest() error {
	log.Println("Generate Digest triggered...")

	if !a.authManager.IsAuthenticated() {
		log.Println("Not authenticated - please login to X first")
		return nil
	}

	cookies, err := a.authManager.GetCookies()
	if err != nil {
		log.Printf("Failed to get cookies: %v", err)
		return err
	}

	// Read config and components with lock to avoid race with ReloadConfig
	a.mu.RLock()
	cfg := a.config
	sc := a.scraper
	an := a.analyzer
	a.mu.RUnlock()

	ctx := context.Background()

	// Step 1: Scrape posts
	log.Printf("Scraping %d posts from For You feed...", cfg.Scraping.PostsPerScrape)
	posts, err := sc.ScrapeForYou(ctx, cookies, cfg.Scraping.PostsPerScrape)
	if err != nil {
		log.Printf("Scrape failed: %v", err)
		return err
	}
	log.Printf("Scraped %d posts", len(posts))

	if len(posts) == 0 {
		log.Println("No posts scraped - nothing to analyze")
		return nil
	}

	// Step 2: Analyze posts with LLM
	log.Println("Analyzing posts with LLM...")
	analyses, err := an.AnalyzePosts(ctx, posts)
	if err != nil {
		log.Printf("Analysis failed: %v", err)
		return err
	}
	log.Printf("Analyzed %d posts", len(analyses))

	// Step 3: Filter by relevance threshold and combine with posts
	analysisMap := make(map[string]*types.Analysis)
	for i := range analyses {
		analysisMap[analyses[i].PostID] = &analyses[i]
	}

	var relevantPosts []types.PostWithAnalysis
	for _, post := range posts {
		analysis, ok := analysisMap[post.ID]
		if !ok {
			continue
		}
		if analysis.RelevanceScore >= cfg.Analysis.RelevanceThreshold {
			relevantPosts = append(relevantPosts, types.PostWithAnalysis{
				Post:     post,
				Analysis: analysis,
			})
		}
	}

	log.Printf("Found %d posts above relevance threshold (%.0f%%)",
		len(relevantPosts), cfg.Analysis.RelevanceThreshold*100)

	if len(relevantPosts) == 0 {
		log.Println("No posts above relevance threshold - no digest generated")
		return nil
	}

	// Step 4: Fetch context (replies) for posts that need it
	if cfg.Digest.IncludeContext {
		log.Println("Fetching context for relevant posts...")
		for i := range relevantPosts {
			if relevantPosts[i].Analysis.NeedsContext {
				log.Printf("Fetching replies for post %s...", relevantPosts[i].Post.ID)
				replies, err := sc.ScrapeThread(ctx, cookies, relevantPosts[i].Post.OriginalURL, 3)
				if err != nil {
					log.Printf("Failed to fetch replies for %s: %v", relevantPosts[i].Post.ID, err)
					continue
				}
				relevantPosts[i].Context = replies
				log.Printf("Got %d replies for post %s", len(replies), relevantPosts[i].Post.ID)
			}
		}
	}

	// Step 5: Build and save digest
	log.Println("Building digest...")
	builder := digest.New(cfg.Digest.OutputDir, cfg.Digest.MaxPosts)
	d, err := builder.Build(relevantPosts, len(posts))
	if err != nil {
		log.Printf("Failed to build digest: %v", err)
		return err
	}

	log.Printf("Digest saved to: %s (%d posts)", d.FilePath, d.PostCount)
	return nil
}

// ViewLastDigest opens the most recent digest file
func (a *App) ViewLastDigest() error {
	a.mu.RLock()
	outputDir := a.config.Digest.OutputDir
	a.mu.RUnlock()

	path, err := digest.GetLatestDigest(outputDir)
	if err != nil {
		log.Printf("No digest found: %v", err)
		return err
	}

	log.Printf("Opening digest: %s", path)
	return browser.OpenFile(path)
}

// ReloadConfig reloads the configuration from disk
func (a *App) ReloadConfig() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Recreate analyzer with new config
	provider := providers.NewClaudeProvider(cfg.Analysis.APIKey, cfg.Analysis.Model)
	newAnalyzer := analyzer.New(provider, cfg.Interests, cfg.Analysis.BatchSize)

	a.mu.Lock()
	a.config = cfg
	a.analyzer = newAnalyzer
	a.scraper = scraper.New(cfg.Scraping.Headless)
	a.mu.Unlock()

	log.Println("Configuration reloaded")
	return nil
}

// global app instance for systray callbacks
var app *App

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
	app = NewApp(cfg, authManager, postScraper, postAnalyzer)

	log.Println("scroll4me starting...")

	// Run systray (blocks until Quit)
	systray.Run(onReady, onExit)
}

func onReady() {
	// Set icon (template icon for macOS menu bar styling)
	systray.SetTemplateIcon(iconBytes, iconBytes)
	systray.SetTitle("")
	systray.SetTooltip("scroll4me - X digest without the doomscrolling")

	// Auth status (disabled, just for display)
	var authStatusLabel string
	if app.IsAuthenticated() {
		authStatusLabel = "● Connected to X"
	} else {
		authStatusLabel = "○ Not connected"
	}
	mAuthStatus := systray.AddMenuItem(authStatusLabel, "Authentication status")
	mAuthStatus.Disable()

	// Auth action (Login / Logout)
	var authActionLabel string
	if app.IsAuthenticated() {
		authActionLabel = "Logout"
	} else {
		authActionLabel = "Login to X"
	}
	mAuthAction := systray.AddMenuItem(authActionLabel, "Login or logout from X")

	systray.AddSeparator()

	// Generate Digest (combined scrape + analyze + build)
	mGenerateDigest := systray.AddMenuItem("Generate Digest", "Scrape, analyze, and create digest")

	systray.AddSeparator()

	// View last digest
	mViewDigest := systray.AddMenuItem("View Last Digest", "Open last digest file")

	// Edit config
	mEditConfig := systray.AddMenuItem("Edit Config", "Open config file in editor")

	// Reload config
	mReloadConfig := systray.AddMenuItem("Reload Config", "Reload configuration from disk")

	systray.AddSeparator()

	// Quit
	mQuit := systray.AddMenuItem("Quit", "Exit scroll4me")

	// Helper to update auth UI
	updateAuthUI := func() {
		if app.IsAuthenticated() {
			mAuthStatus.SetTitle("● Connected to X")
			mAuthAction.SetTitle("Logout")
		} else {
			mAuthStatus.SetTitle("○ Not connected")
			mAuthAction.SetTitle("Login to X")
		}
	}

	// Handle menu clicks
	go func() {
		for {
			select {
			case <-mAuthAction.ClickedCh:
				if app.IsAuthenticated() {
					if err := app.TriggerLogout(); err != nil {
						log.Printf("Logout error: %v", err)
					}
				} else {
					if err := app.TriggerLogin(); err != nil {
						log.Printf("Login error: %v", err)
					}
				}
				updateAuthUI()

			case <-mGenerateDigest.ClickedCh:
				go func() {
					if err := app.GenerateDigest(); err != nil {
						log.Printf("Generate digest error: %v", err)
					}
				}()

			case <-mViewDigest.ClickedCh:
				if err := app.ViewLastDigest(); err != nil {
					log.Printf("View digest error: %v", err)
				}

			case <-mEditConfig.ClickedCh:
				path, err := config.ConfigPath()
				if err != nil {
					log.Printf("Failed to get config path: %v", err)
					continue
				}
				if err := browser.OpenFile(path); err != nil {
					log.Printf("Failed to open config file: %v", err)
				}

			case <-mReloadConfig.ClickedCh:
				if err := app.ReloadConfig(); err != nil {
					log.Printf("Failed to reload config: %v", err)
				}

			case <-mQuit.ClickedCh:
				systray.Quit()
			}
		}
	}()
}

func onExit() {
	log.Println("scroll4me shutting down...")
}
