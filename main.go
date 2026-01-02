package main

import (
	"context"
	_ "embed"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/getlantern/systray"
	"github.com/pkg/browser"

	"github.com/ibeckermayer/scroll4me/internal/auth"
	"github.com/ibeckermayer/scroll4me/internal/config"
	"github.com/ibeckermayer/scroll4me/internal/scraper"
	"github.com/ibeckermayer/scroll4me/internal/store"
)

//go:embed assets/icon.png
var iconBytes []byte

// App holds the application state
type App struct {
	mu          sync.RWMutex
	config      *config.Config
	authManager *auth.Manager
	store       *store.Store
	scraper     *scraper.Scraper
	paused      bool
}

// NewApp creates a new App instance
func NewApp(cfg *config.Config, authManager *auth.Manager, st *store.Store, sc *scraper.Scraper) *App {
	return &App{
		config:      cfg,
		authManager: authManager,
		store:       st,
		scraper:     sc,
		paused:      false,
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

// TriggerScrape manually triggers a scrape
func (a *App) TriggerScrape() error {
	log.Println("Manual scrape triggered - starting scrape...")

	if !a.authManager.IsAuthenticated() {
		log.Println("Not authenticated - please login to X first")
		return nil
	}

	cookies, err := a.authManager.GetCookies()
	if err != nil {
		log.Printf("Failed to get cookies: %v", err)
		return err
	}

	// Read config with lock
	a.mu.RLock()
	postsPerScrape := a.config.Scraping.PostsPerScrape
	a.mu.RUnlock()

	ctx := context.Background()
	posts, err := a.scraper.ScrapeForYou(ctx, cookies, postsPerScrape)
	if err != nil {
		log.Printf("Scrape failed: %v", err)
		return err
	}

	log.Printf("Scraped %d posts, saving to database...", len(posts))

	savedCount := 0
	for _, post := range posts {
		if err := a.store.SavePost(&post); err != nil {
			log.Printf("Failed to save post %s: %v", post.ID, err)
			continue
		}
		savedCount++
	}

	log.Printf("Scrape complete: saved %d/%d posts", savedCount, len(posts))
	return nil
}

// TriggerDigest manually triggers digest generation and sending
func (a *App) TriggerDigest() error {
	// TODO: Call digest builder + notifier
	log.Println("Manual digest triggered")
	return nil
}

// ReloadConfig reloads the configuration from disk
func (a *App) ReloadConfig() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.config = cfg
	a.mu.Unlock()
	log.Println("Configuration reloaded")
	return nil
}

// IsPaused returns whether the app is paused (thread-safe)
func (a *App) IsPaused() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.paused
}

// SetPaused sets the paused state (thread-safe)
func (a *App) SetPaused(paused bool) {
	a.mu.Lock()
	a.paused = paused
	a.mu.Unlock()
}

// global app instance for cleanup in onExit
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

	// Initialize SQLite store
	configDir, err := config.ConfigDir()
	if err != nil {
		log.Fatalf("Failed to get config directory: %v", err)
	}
	dbPath := filepath.Join(configDir, "scroll4me.db")
	contentStore, err := store.New(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize store: %v", err)
	}
	defer contentStore.Close() // Ensure cleanup on any exit from main()

	// Initialize scraper
	postScraper := scraper.New(cfg.Scraping.Headless)

	// Create app
	app = NewApp(cfg, authManager, contentStore, postScraper)

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

	// Actions
	mScrape := systray.AddMenuItem("Scrape Now", "Trigger a manual scrape")
	mDigest := systray.AddMenuItem("Send Digest Now", "Generate and send digest")

	systray.AddSeparator()

	// View last digest
	mViewDigest := systray.AddMenuItem("View Last Digest", "Open last digest in browser")

	// Edit config
	mEditConfig := systray.AddMenuItem("Edit Config", "Open config file in editor")

	// Reload config
	mReloadConfig := systray.AddMenuItem("Reload Config", "Reload configuration from disk")

	systray.AddSeparator()

	// Pause/Resume
	mPause := systray.AddMenuItem("Pause", "Pause scheduled tasks")

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
					// Logout (no confirmation dialog in systray)
					if err := app.TriggerLogout(); err != nil {
						log.Printf("Logout error: %v", err)
					}
				} else {
					if err := app.TriggerLogin(); err != nil {
						log.Printf("Login error: %v", err)
					}
				}
				updateAuthUI()

			case <-mScrape.ClickedCh:
				go func() {
					if err := app.TriggerScrape(); err != nil {
						log.Printf("Scrape error: %v", err)
					}
				}()

			case <-mDigest.ClickedCh:
				go func() {
					if err := app.TriggerDigest(); err != nil {
						log.Printf("Digest error: %v", err)
					}
				}()

			case <-mViewDigest.ClickedCh:
				// TODO: Open last digest HTML in default browser
				log.Println("View Last Digest clicked")

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
				// TODO: implement hot reload of config
				if err := app.ReloadConfig(); err != nil {
					log.Printf("Failed to reload config: %v", err)
				}

			case <-mPause.ClickedCh:
				if app.IsPaused() {
					app.SetPaused(false)
					mPause.SetTitle("Pause")
					log.Println("Resumed")
				} else {
					app.SetPaused(true)
					mPause.SetTitle("Resume")
					log.Println("Paused")
				}

			case <-mQuit.ClickedCh:
				systray.Quit()
			}
		}
	}()
}

func onExit() {
	log.Println("scroll4me shutting down...")
	if app != nil && app.store != nil {
		app.store.Close()
	}
}
