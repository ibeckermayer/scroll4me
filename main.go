package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/chromedp/chromedp"
	"github.com/getlantern/systray"
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

	// No args = run the main tray app
	if len(os.Args) < 2 {
		runApp()
		return
	}

	// Subcommands for dev/utility
	switch os.Args[1] {
	case "bot-test":
		runBotTest()
	case "open":
		if len(os.Args) < 3 {
			fmt.Println("Usage: scroll4me open <config|cache>")
			os.Exit(1)
		}
		runOpen(os.Args[2])
	case "analyze":
		runAnalyze(os.Args[2:])
	case "clear-cache":
		runClearCache()
	case "-h", "--help", "help":
		printUsage()
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("scroll4me - AI-powered social media digest")
	fmt.Println()
	fmt.Println("Usage: scroll4me [command]")
	fmt.Println()
	fmt.Println("Running with no command starts the system tray application.")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  bot-test        Open bot.sannysoft.com to audit browser fingerprint")
	fmt.Println("  open config     Open config file in default editor")
	fmt.Println("  open cache      Open cache directory in file explorer")
	fmt.Println("  analyze         Run analysis pipeline on cached posts")
	fmt.Println("  clear-cache     Clear all cached data")
	fmt.Println("  help            Show this help message")
}

func runApp() {
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
	postScraper := scraper.New(cfg.Scraping.Headless, cfg.Scraping.DebugPauseAfterScrape)

	// Initialize analyzer
	postAnalyzer, err := analyzer.New(cfg.Analysis, cfg.Interests)
	if err != nil {
		log.Fatalf("Failed to initialize analyzer: %v", err)
	}

	// Create app
	a := app.New(cfg, authManager, postScraper, postAnalyzer)

	log.Println("scroll4me starting...")

	// Run systray (blocks until Quit)
	systray.Run(tray.OnReady(a), tray.OnExit)
}

func runBotTest() {
	log.Println("Opening bot.sannysoft.com with stealth browser options...")

	opts := browseropts.Options(false) // non-headless so you can see it

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

func runOpen(target string) {
	var path string
	var err error

	switch target {
	case "config":
		path, err = config.ConfigPath()
	case "cache":
		path, err = config.CacheDir()
	default:
		fmt.Printf("Unknown target: %s\n", target)
		os.Exit(1)
	}

	if err != nil {
		log.Fatalf("Failed to get path: %v", err)
	}

	if err := browser.OpenFile(path); err != nil {
		log.Fatalf("Failed to open: %v", err)
	}
}

func runAnalyze(args []string) {
	fs := flag.NewFlagSet("analyze", flag.ExitOnError)
	filePath := fs.String("file", "", "Path to posts JSON file (default: latest from cache)")
	noOpen := fs.Bool("no-open", false, "Don't open the digest after generating")

	fs.Usage = func() {
		fmt.Println("Usage: scroll4me analyze [options]")
		fmt.Println()
		fmt.Println("Run steps 2-4 on cached or specified posts:")
		fmt.Println("  2. Analyze posts with LLM")
		fmt.Println("  3. Filter by relevance threshold")
		fmt.Println("  4. Build and save digest")
		fmt.Println()
		fmt.Println("Options:")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	// Load posts from file or cache
	var posts []types.Post
	var postsPath string

	if *filePath != "" {
		log.Printf("Loading posts from: %s", *filePath)
		var err error
		posts, err = store.LoadStepOutput[[]types.Post](*filePath)
		if err != nil {
			log.Fatalf("Failed to load posts from %s: %v", *filePath, err)
		}
		postsPath = *filePath
	} else {
		log.Println("Loading latest posts from cache...")
		var err error
		posts, postsPath, err = store.LoadLatestStepOutput[[]types.Post](store.Step1Posts)
		if err != nil {
			log.Fatalf("Failed to load latest posts: %v", err)
		}
	}
	log.Printf("Loaded %d posts from: %s", len(posts), postsPath)

	if len(posts) == 0 {
		log.Println("No posts to analyze")
		return
	}

	// Initialize app
	a, err := initApp()
	if err != nil {
		log.Fatalf("Failed to initialize app: %v", err)
	}

	ctx := context.Background()

	// Step 2: Analyze posts with LLM
	analyses, err := a.AnalyzePosts(ctx, posts)
	if err != nil {
		log.Fatalf("Analysis failed: %v", err)
	}

	// Step 3: Filter by relevance threshold
	relevantPosts := a.FilterByRelevance(posts, analyses)
	if len(relevantPosts) == 0 {
		log.Println("No posts above relevance threshold - no digest generated")
		return
	}

	// Step 4: Build and save digest
	digestPath, err := a.BuildDigest(relevantPosts, len(posts))
	if err != nil {
		log.Fatalf("Failed to build digest: %v", err)
	}

	// Open digest unless --no-open
	if !*noOpen {
		if err := browser.OpenFile(digestPath); err != nil {
			log.Printf("Failed to open digest: %v", err)
		}
	}
}

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

	// Initialize cookie store and auth manager
	cookieStorePath, err := auth.DefaultCookieStorePath()
	if err != nil {
		return nil, fmt.Errorf("failed to get cookie store path: %w", err)
	}
	cookieStore := auth.NewCookieStore(cookieStorePath)
	authManager := auth.NewManager(cookieStore)

	// Initialize scraper (headless for CLI)
	postScraper := scraper.New(true, false)

	// Initialize analyzer
	postAnalyzer, err := analyzer.New(cfg.Analysis, cfg.Interests)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize analyzer: %w", err)
	}

	return app.New(cfg, authManager, postScraper, postAnalyzer), nil
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
