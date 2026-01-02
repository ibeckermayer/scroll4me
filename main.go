package main

import (
	"context"
	"embed"
	_ "embed"
	"log"
	"path/filepath"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/icons"

	"github.com/ibeckermayer/scroll4me/internal/auth"
	"github.com/ibeckermayer/scroll4me/internal/config"
	"github.com/ibeckermayer/scroll4me/internal/scraper"
	"github.com/ibeckermayer/scroll4me/internal/store"
)

// Wails uses Go's `embed` package to embed the frontend files into the binary.
// Any files in the frontend/dist folder will be embedded into the binary and
// made available to the frontend.

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

// App struct holds the application state exposed to the frontend
type App struct {
	config      *config.Config
	authManager *auth.Manager
	store       *store.Store
	scraper     *scraper.Scraper
}

// NewApp creates a new App instance
func NewApp(cfg *config.Config, authManager *auth.Manager, st *store.Store, sc *scraper.Scraper) *App {
	return &App{
		config:      cfg,
		authManager: authManager,
		store:       st,
		scraper:     sc,
	}
}

// GetConfig returns the current configuration (called from frontend)
func (a *App) GetConfig() *config.Config {
	return a.config
}

// SaveConfig saves the configuration (called from frontend)
func (a *App) SaveConfig(cfg *config.Config) error {
	a.config = cfg
	return cfg.Save()
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

// TriggerScrape manually triggers a scrape
func (a *App) TriggerScrape() error {
	log.Println("Manual scrape triggered - starting scrape...")

	// Check if authenticated
	if !a.authManager.IsAuthenticated() {
		return &ScrapeError{Message: "not authenticated - please login to X first"}
	}

	// Get cookies for scraping
	cookies, err := a.authManager.GetCookies()
	if err != nil {
		log.Printf("Failed to get cookies: %v", err)
		return &ScrapeError{Message: "failed to get authentication cookies"}
	}

	// Scrape the For You feed
	ctx := context.Background()
	posts, err := a.scraper.ScrapeForYou(ctx, cookies, a.config.Scraping.PostsPerScrape)
	if err != nil {
		log.Printf("Scrape failed: %v", err)
		return &ScrapeError{Message: "scraping failed: " + err.Error()}
	}

	log.Printf("Scraped %d posts, saving to database...", len(posts))

	// Save posts to the store
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

// ScrapeError represents a scraping error
type ScrapeError struct {
	Message string
}

func (e *ScrapeError) Error() string {
	return e.Message
}

// TriggerDigest manually triggers digest generation and sending
func (a *App) TriggerDigest() error {
	// TODO: Call digest builder + notifier
	log.Println("Manual digest triggered")
	return nil
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Warning: could not load config: %v (using defaults)", err)
		cfg = config.Default()
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
	defer contentStore.Close()

	// Initialize scraper
	postScraper := scraper.New(cfg.Scraping.Headless)

	// Create app service for frontend bindings
	appService := NewApp(cfg, authManager, contentStore, postScraper)

	// Create Wails application
	app := application.New(application.Options{
		Name:        "scroll4me",
		Description: "X digest without the doomscrolling",
		Services: []application.Service{
			application.NewService(appService),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			// Makes it a menu bar app (no dock icon)
			ActivationPolicy: application.ActivationPolicyAccessory,
		},
	})

	// Create system tray
	systemTray := app.SystemTray.New()

	// Create settings window (hidden by default, attached to tray)
	settingsWindow := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:            "Settings",
		Title:           "scroll4me Settings",
		Width:           450,
		Height:          600,
		Frameless:       false,
		AlwaysOnTop:     true,
		Hidden:          true,
		DisableResize:   false,
		URL:             "/",
		DevToolsEnabled: true, // Enable DevTools for debugging (Cmd+Option+I on macOS)
		Windows: application.WindowsWindow{
			HiddenOnTaskbar: true,
		},
	})

	// Hide window on close instead of quitting
	settingsWindow.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		settingsWindow.Hide()
		e.Cancel()
	})

	// Set tray icon
	if runtime.GOOS == "darwin" {
		systemTray.SetTemplateIcon(icons.SystrayMacTemplate)
	} else {
		systemTray.SetIcon(appIcon)
	}

	// Build tray menu
	menu := app.NewMenu()

	// Status section
	statusItem := menu.Add("✓ Running")
	statusItem.SetEnabled(false)

	menu.AddSeparator()

	lastScrapeItem := menu.Add("Last scrape: never")
	lastScrapeItem.SetEnabled(false)

	nextDigestItem := menu.Add("Next digest: not scheduled")
	nextDigestItem.SetEnabled(false)

	menu.AddSeparator()

	// Actions
	menu.Add("Scrape Now").OnClick(func(ctx *application.Context) {
		go func() {
			if err := appService.TriggerScrape(); err != nil {
				log.Printf("Scrape error: %v", err)
			}
		}()
	})

	menu.Add("Send Digest Now").OnClick(func(ctx *application.Context) {
		go func() {
			if err := appService.TriggerDigest(); err != nil {
				log.Printf("Digest error: %v", err)
			}
		}()
	})

	menu.AddSeparator()

	// Settings
	menu.Add("Settings...").OnClick(func(ctx *application.Context) {
		settingsWindow.Show()
	})

	// View last digest in browser
	menu.Add("View Last Digest").OnClick(func(ctx *application.Context) {
		// TODO: Open last digest HTML in default browser
		log.Println("View Last Digest clicked")
	})

	menu.AddSeparator()

	// Pause/Resume
	pauseItem := menu.Add("Pause")
	pauseItem.OnClick(func(ctx *application.Context) {
		// TODO: Implement pause/resume
		if pauseItem.Label() == "Pause" {
			pauseItem.SetLabel("Resume")
			statusItem.SetLabel("⏸ Paused")
		} else {
			pauseItem.SetLabel("Pause")
			statusItem.SetLabel("✓ Running")
		}
	})

	menu.AddSeparator()

	// Quit
	menu.Add("Quit").OnClick(func(ctx *application.Context) {
		app.Quit()
	})

	// Attach menu to tray (clicking tray icon shows this menu)
	systemTray.SetMenu(menu)

	// TODO: Initialize and start scheduler

	log.Println("scroll4me starting...")

	// Run the application
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
