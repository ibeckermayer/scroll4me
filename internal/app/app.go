package app

import (
	"context"
	"log"
	"sync"

	"github.com/pkg/browser"

	"github.com/ibeckermayer/scroll4me/internal/analyzer"
	"github.com/ibeckermayer/scroll4me/internal/auth"
	"github.com/ibeckermayer/scroll4me/internal/config"
	"github.com/ibeckermayer/scroll4me/internal/digest"
	"github.com/ibeckermayer/scroll4me/internal/scraper"
	"github.com/ibeckermayer/scroll4me/internal/store"
	"github.com/ibeckermayer/scroll4me/internal/types"
)

// App holds the application state.
type App struct {
	mu          sync.RWMutex
	authManager *auth.Manager // immutable after creation

	// Mutable fields - use getSnapshot() for concurrent access.
	config   *config.Config
	scraper  *scraper.Scraper
	analyzer *analyzer.Analyzer
}

// snapshot holds fields that may be replaced by ReloadConfig.
// Use getSnapshot() to obtain a consistent, point-in-time copy.
type snapshot struct {
	config   *config.Config
	scraper  *scraper.Scraper
	analyzer *analyzer.Analyzer
}

// getSnapshot returns a snapshot of mutable fields under read lock.
func (a *App) getSnapshot() snapshot {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return snapshot{
		config:   a.config,
		scraper:  a.scraper,
		analyzer: a.analyzer,
	}
}

// New creates a new App instance.
func New(cfg *config.Config, authManager *auth.Manager, sc *scraper.Scraper, an *analyzer.Analyzer) *App {
	return &App{
		config:      cfg,
		authManager: authManager,
		scraper:     sc,
		analyzer:    an,
	}
}

// IsAuthenticated checks if X.com credentials are stored.
func (a *App) IsAuthenticated() bool {
	return a.authManager.IsAuthenticated()
}

// TriggerLogin starts the X.com login flow.
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

// TriggerLogout clears stored X.com credentials.
func (a *App) TriggerLogout() error {
	log.Println("Logout triggered - clearing stored cookies")
	if err := a.authManager.Logout(); err != nil {
		log.Printf("Logout failed: %v", err)
		return err
	}
	log.Println("Logout successful - cookies cleared")
	return nil
}

// GenerateDigest performs the full scrape -> analyze -> build digest flow.
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

	s := a.getSnapshot()
	ctx := context.Background()

	// Step 1: Scrape posts
	log.Printf("Scraping %d posts from For You feed...", s.config.Scraping.PostsPerScrape)
	posts, err := s.scraper.ScrapeForYou(ctx, cookies, s.config.Scraping.PostsPerScrape)
	if err != nil {
		log.Printf("Scrape failed: %v", err)
		return err
	}
	log.Printf("Scraped %d posts", len(posts))

	if len(posts) == 0 {
		log.Println("No posts scraped - nothing to analyze")
		return nil
	}

	// Save posts to cache for debugging
	if cachePath, err := store.SavePosts(posts); err != nil {
		log.Printf("Failed to cache posts: %v", err)
	} else {
		log.Printf("Cached posts to: %s", cachePath)
	}

	// Step 2: Analyze posts with LLM
	log.Println("Analyzing posts with LLM...")
	analyses, err := s.analyzer.AnalyzePosts(ctx, posts)
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
		if analysis.RelevanceScore >= s.config.Analysis.RelevanceThreshold {
			relevantPosts = append(relevantPosts, types.PostWithAnalysis{
				Post:     post,
				Analysis: analysis,
			})
		}
	}

	log.Printf("Found %d posts above relevance threshold (%.0f%%)",
		len(relevantPosts), s.config.Analysis.RelevanceThreshold*100)

	if len(relevantPosts) == 0 {
		log.Println("No posts above relevance threshold - no digest generated")
		return nil
	}

	// Step 4: Fetch context (replies) for posts that need it
	if s.config.Digest.IncludeContext {
		log.Println("Fetching context for relevant posts...")
		for i := range relevantPosts {
			if relevantPosts[i].Analysis.NeedsContext {
				log.Printf("Fetching replies for post %s...", relevantPosts[i].Post.ID)
				replies, err := s.scraper.ScrapeThread(ctx, cookies, relevantPosts[i].Post.OriginalURL, 3)
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
	builder := digest.New(s.config.Digest.OutputDir, s.config.Digest.MaxPosts)
	d, err := builder.Build(relevantPosts, len(posts))
	if err != nil {
		log.Printf("Failed to build digest: %v", err)
		return err
	}

	log.Printf("Digest saved to: %s (%d posts)", d.FilePath, d.PostCount)
	return nil
}

// ViewLastDigest opens the most recent digest file.
func (a *App) ViewLastDigest() error {
	s := a.getSnapshot()

	path, err := digest.GetLatestDigest(s.config.Digest.OutputDir)
	if err != nil {
		log.Printf("No digest found: %v", err)
		return err
	}

	log.Printf("Opening digest: %s", path)
	return browser.OpenFile(path)
}

// ReloadConfig reloads the configuration from disk.
func (a *App) ReloadConfig() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Recreate analyzer with new config
	newAnalyzer, err := analyzer.New(cfg.Analysis, cfg.Interests)
	if err != nil {
		return err
	}

	a.mu.Lock()
	a.config = cfg
	a.analyzer = newAnalyzer
	a.scraper = scraper.New(cfg.Scraping.Headless, cfg.Scraping.DebugPauseAfterScrape)
	a.mu.Unlock()

	log.Println("Configuration reloaded")
	return nil
}
