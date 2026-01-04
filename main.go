package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/chromedp/chromedp"
	"github.com/getlantern/systray"
	"github.com/peterbourgon/ff/v3/ffcli"
	"github.com/pkg/browser"

	"github.com/ibeckermayer/scroll4me/internal/analyzer"
	"github.com/ibeckermayer/scroll4me/internal/app"
	"github.com/ibeckermayer/scroll4me/internal/auth"
	browseropts "github.com/ibeckermayer/scroll4me/internal/browser"
	"github.com/ibeckermayer/scroll4me/internal/config"
	"github.com/ibeckermayer/scroll4me/internal/scraper"
	"github.com/ibeckermayer/scroll4me/internal/store"
	"github.com/ibeckermayer/scroll4me/internal/tray"
	"github.com/ibeckermayer/scroll4me/internal/types"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	root := buildCLI()
	if err := root.Parse(os.Args[1:]); err != nil {
		if err == flag.ErrHelp {
			os.Exit(0)
		}
		log.Fatal(err)
	}
	if err := root.Run(context.Background()); err != nil {
		if err == flag.ErrHelp {
			os.Exit(0)
		}
		log.Fatal(err)
	}
}

func buildCLI() *ffcli.Command {
	return &ffcli.Command{
		Name:       "scroll4me",
		ShortUsage: "scroll4me [command]",
		ShortHelp:  "AI-powered social media digest",
		LongHelp:   "Running with no command starts the system tray application.",
		Subcommands: []*ffcli.Command{
			openCmd(),
			stepCmd(),
			loginCmd(),
			logoutCmd(),
			clearCmd(),
			botTestCmd(),
		},
		Exec: func(ctx context.Context, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unknown command: %s\nRun 'scroll4me --help' for usage", args[0])
			}
			runTrayApp()
			return nil
		},
	}
}

// =============================================================================
// Step Commands
// =============================================================================

func stepCmd() *ffcli.Command {
	return &ffcli.Command{
		Name:       "step",
		ShortUsage: "scroll4me step <subcommand>",
		ShortHelp:  "Run pipeline steps individually",
		Subcommands: []*ffcli.Command{
			stepScrapeCmd(),
			stepAnalyzeCmd(),
			stepFilterCmd(),
			stepDigestCmd(),
			stepOpenCmd(),
			stepAllCmd(),
		},
		Exec: func(ctx context.Context, args []string) error {
			return flag.ErrHelp
		},
	}
}

func stepScrapeCmd() *ffcli.Command {
	return &ffcli.Command{
		Name:       "scrape",
		ShortUsage: "scroll4me step scrape",
		ShortHelp:  "Step 1: Scrape posts from X For You feed",
		Exec: func(ctx context.Context, args []string) error {
			a, err := initApp()
			if err != nil {
				return err
			}
			if !a.IsAuthenticated() {
				return fmt.Errorf("not authenticated - run 'scroll4me login' first")
			}
			_, err = a.ScrapeForYou(ctx)
			return err
		},
	}
}

func stepAnalyzeCmd() *ffcli.Command {
	fs := flag.NewFlagSet("analyze", flag.ExitOnError)
	file := fs.String("file", "", "posts JSON file (default: latest from cache)")

	return &ffcli.Command{
		Name:       "analyze",
		ShortUsage: "scroll4me step analyze [-file path]",
		ShortHelp:  "Step 2: Analyze posts with LLM",
		FlagSet:    fs,
		Exec: func(ctx context.Context, args []string) error {
			posts, _, err := loadPosts(*file)
			if err != nil {
				return err
			}
			if len(posts) == 0 {
				log.Println("No posts to analyze")
				return nil
			}
			a, err := initApp()
			if err != nil {
				return err
			}
			_, err = a.AnalyzePosts(ctx, posts)
			return err
		},
	}
}

func stepFilterCmd() *ffcli.Command {
	fs := flag.NewFlagSet("filter", flag.ExitOnError)
	postsFile := fs.String("posts-file", "", "posts JSON file (default: latest from cache)")
	analysesFile := fs.String("analyses-file", "", "analyses JSON file (default: latest from cache)")

	return &ffcli.Command{
		Name:       "filter",
		ShortUsage: "scroll4me step filter [-posts-file path] [-analyses-file path]",
		ShortHelp:  "Step 3: Filter posts by relevance threshold",
		FlagSet:    fs,
		Exec: func(ctx context.Context, args []string) error {
			posts, _, err := loadPosts(*postsFile)
			if err != nil {
				return fmt.Errorf("failed to load posts: %w", err)
			}
			analyses, err := loadAnalyses(*analysesFile)
			if err != nil {
				return fmt.Errorf("failed to load analyses: %w", err)
			}
			if len(posts) == 0 {
				log.Println("No posts to filter")
				return nil
			}
			a, err := initApp()
			if err != nil {
				return err
			}
			filtered := a.FilterByRelevance(posts, analyses)
			log.Printf("Filtered to %d relevant posts", len(filtered))
			return nil
		},
	}
}

func stepDigestCmd() *ffcli.Command {
	fs := flag.NewFlagSet("digest", flag.ExitOnError)
	file := fs.String("file", "", "filtered posts JSON file (default: latest from cache)")
	noOpen := fs.Bool("no-open", false, "don't open digest after generating")

	return &ffcli.Command{
		Name:       "digest",
		ShortUsage: "scroll4me step digest [-file path] [-no-open]",
		ShortHelp:  "Step 4: Build and save digest",
		FlagSet:    fs,
		Exec: func(ctx context.Context, args []string) error {
			filtered, totalScraped, err := loadFiltered(*file)
			if err != nil {
				return err
			}
			if len(filtered) == 0 {
				log.Println("No filtered posts - nothing to digest")
				return nil
			}
			a, err := initApp()
			if err != nil {
				return err
			}
			digestPath, err := a.BuildDigest(filtered, totalScraped)
			if err != nil {
				return err
			}
			if !*noOpen {
				if err := browser.OpenFile(digestPath); err != nil {
					log.Printf("Failed to open digest: %v", err)
				}
			}
			return nil
		},
	}
}

func stepOpenCmd() *ffcli.Command {
	return &ffcli.Command{
		Name:       "open",
		ShortUsage: "scroll4me step open",
		ShortHelp:  "Step 5: Open the latest digest",
		Exec: func(ctx context.Context, args []string) error {
			a, err := initApp()
			if err != nil {
				return err
			}
			return a.ViewLastDigest()
		},
	}
}

func stepAllCmd() *ffcli.Command {
	return &ffcli.Command{
		Name:       "all",
		ShortUsage: "scroll4me step all",
		ShortHelp:  "Run the full pipeline (scrape -> analyze -> filter -> digest -> open)",
		Exec: func(ctx context.Context, args []string) error {
			a, err := initApp()
			if err != nil {
				return err
			}
			return a.GenerateDigest()
		},
	}
}

// =============================================================================
// Utility Commands
// =============================================================================

func openCmd() *ffcli.Command {
	return &ffcli.Command{
		Name:       "open",
		ShortUsage: "scroll4me open <config|cache|digest>",
		ShortHelp:  "Open config file, cache directory, or latest digest",
		Exec: func(ctx context.Context, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("usage: scroll4me open <config|cache|digest>")
			}
			return runOpen(args[0])
		},
	}
}

func clearCmd() *ffcli.Command {
	return &ffcli.Command{
		Name:       "clear",
		ShortUsage: "scroll4me clear <cache|cookies>",
		ShortHelp:  "Clear cache or cookies",
		Exec: func(ctx context.Context, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("usage: scroll4me clear <cache|cookies>")
			}
			return runClear(args[0])
		},
	}
}

func loginCmd() *ffcli.Command {
	return &ffcli.Command{
		Name:       "login",
		ShortUsage: "scroll4me login",
		ShortHelp:  "Open browser to login to X.com",
		Exec: func(ctx context.Context, args []string) error {
			a, err := initApp()
			if err != nil {
				return err
			}
			return a.TriggerLogin()
		},
	}
}

func logoutCmd() *ffcli.Command {
	return &ffcli.Command{
		Name:       "logout",
		ShortUsage: "scroll4me logout",
		ShortHelp:  "Clear stored cookies (logout)",
		Exec: func(ctx context.Context, args []string) error {
			runClearCookies()
			return nil
		},
	}
}

func botTestCmd() *ffcli.Command {
	return &ffcli.Command{
		Name:       "bottest",
		ShortUsage: "scroll4me bottest",
		ShortHelp:  "Open bot.sannysoft.com to audit browser fingerprint",
		Exec: func(ctx context.Context, args []string) error {
			runBotTest()
			return nil
		},
	}
}

// =============================================================================
// Cache Loading Helpers
// =============================================================================

// loadPosts loads posts from file or latest cache.
// Returns posts, the path they were loaded from, and any error.
func loadPosts(file string) ([]types.Post, string, error) {
	if file != "" {
		log.Printf("Loading posts from: %s", file)
		posts, err := store.LoadStepOutput[[]types.Post](file)
		return posts, file, err
	}
	log.Println("Loading latest posts from cache...")
	return store.LoadLatestStepOutput[[]types.Post](store.Step1Posts)
}

// loadAnalyses loads analyses from file or latest cache.
func loadAnalyses(file string) ([]types.Analysis, error) {
	if file != "" {
		log.Printf("Loading analyses from: %s", file)
		return store.LoadStepOutput[[]types.Analysis](file)
	}
	log.Println("Loading latest analyses from cache...")
	analyses, _, err := store.LoadLatestStepOutput[[]types.Analysis](store.Step2Analyses)
	return analyses, err
}

// loadFiltered loads filtered posts from file or latest cache.
// Returns filtered posts, estimated total scraped count, and any error.
func loadFiltered(file string) ([]types.PostWithAnalysis, int, error) {
	if file != "" {
		log.Printf("Loading filtered posts from: %s", file)
		filtered, err := store.LoadStepOutput[[]types.PostWithAnalysis](file)
		// When loading from file, we don't know the original scraped count
		return filtered, len(filtered), err
	}
	log.Println("Loading latest filtered posts from cache...")
	filtered, _, err := store.LoadLatestStepOutput[[]types.PostWithAnalysis](store.Step3Filtered)
	if err != nil {
		return nil, 0, err
	}
	// Try to get original post count from step1 cache
	posts, _, postsErr := store.LoadLatestStepOutput[[]types.Post](store.Step1Posts)
	totalScraped := len(filtered)
	if postsErr == nil {
		totalScraped = len(posts)
	}
	return filtered, totalScraped, nil
}

// =============================================================================
// App Initialization
// =============================================================================

// initApp initializes the App with config and dependencies for CLI use.
func initApp() (*app.App, error) {
	cfg, err := config.Load()
	if err != nil {
		if os.IsNotExist(err) {
			cfg = config.Default()
		} else {
			return nil, fmt.Errorf("failed to load config: %w", err)
		}
	}

	cookieStorePath, err := auth.DefaultCookieStorePath()
	if err != nil {
		return nil, fmt.Errorf("failed to get cookie store path: %w", err)
	}
	cookieStore := auth.NewCookieStore(cookieStorePath)
	authManager := auth.NewManager(cookieStore)

	// Use headless for CLI
	postScraper := scraper.New(true, false)

	postAnalyzer, err := analyzer.New(cfg.Analysis, cfg.Interests)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize analyzer: %w", err)
	}

	return app.New(cfg, authManager, postScraper, postAnalyzer), nil
}

// =============================================================================
// Command Implementations
// =============================================================================

func runTrayApp() {
	cfg, err := config.Load()
	if err != nil {
		if os.IsNotExist(err) {
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

	cookieStorePath, err := auth.DefaultCookieStorePath()
	if err != nil {
		log.Fatalf("Failed to get cookie store path: %v", err)
	}
	cookieStore := auth.NewCookieStore(cookieStorePath)
	authManager := auth.NewManager(cookieStore)

	postScraper := scraper.New(cfg.Scraping.Headless, cfg.Scraping.DebugPauseAfterScrape)

	postAnalyzer, err := analyzer.New(cfg.Analysis, cfg.Interests)
	if err != nil {
		log.Fatalf("Failed to initialize analyzer: %v", err)
	}

	a := app.New(cfg, authManager, postScraper, postAnalyzer)

	log.Println("scroll4me starting...")

	systray.Run(tray.OnReady(a), tray.OnExit)
}

func runBotTest() {
	log.Println("Opening bot.sannysoft.com with stealth browser options...")

	opts := browseropts.Options(false)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	go func() {
		err := chromedp.Run(ctx,
			chromedp.Navigate("https://bot.sannysoft.com"),
		)
		if err != nil {
			log.Printf("Failed to navigate: %v", err)
		}
	}()

	fmt.Println("Press Enter to end program...")
	fmt.Scanln()

	log.Println("Done.")
}

func runOpen(target string) error {
	var path string
	var err error

	switch target {
	case "config":
		path, err = config.ConfigPath()
	case "cache":
		path, err = config.CacheDir()
	case "digest":
		a, initErr := initApp()
		if initErr != nil {
			return initErr
		}
		return a.ViewLastDigest()
	default:
		return fmt.Errorf("unknown target: %s (use 'config', 'cache', or 'digest')", target)
	}

	if err != nil {
		return fmt.Errorf("failed to get path: %w", err)
	}

	if err := browser.OpenFile(path); err != nil {
		return fmt.Errorf("failed to open: %w", err)
	}
	return nil
}

func runClear(target string) error {
	switch target {
	case "cache":
		runClearCache()
	case "cookies":
		runClearCookies()
	default:
		return fmt.Errorf("unknown target: %s (use 'cache' or 'cookies')", target)
	}
	return nil
}

func runClearCache() {
	cacheDir, err := config.CacheDir()
	if err != nil {
		log.Fatalf("Failed to get cache directory: %v", err)
	}

	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		log.Println("Cache directory doesn't exist - nothing to clear")
		return
	}

	log.Printf("Clearing cache at: %s", cacheDir)
	if err := os.RemoveAll(cacheDir); err != nil {
		log.Fatalf("Failed to clear cache: %v", err)
	}
	log.Println("Cache cleared successfully")
}

func runClearCookies() {
	cookiePath, err := auth.DefaultCookieStorePath()
	if err != nil {
		log.Fatalf("Failed to get cookie path: %v", err)
	}

	if _, err := os.Stat(cookiePath); os.IsNotExist(err) {
		log.Println("No cookies stored - nothing to clear")
		return
	}

	if err := os.Remove(cookiePath); err != nil {
		log.Fatalf("Failed to clear cookies: %v", err)
	}
	log.Println("Cookies cleared successfully (logged out)")
}
