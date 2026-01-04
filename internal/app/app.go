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

// Config returns the current configuration.
func (a *App) Config() *config.Config {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.config
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

// =============================================================================
// Pipeline Step Methods
// =============================================================================

// ScrapeForYou performs Step 1: Scrape posts from the X "For You" feed.
// Logs progress and caches output to step1_posts.
func (a *App) ScrapeForYou(ctx context.Context) ([]types.Post, error) {
	cookies, err := a.authManager.GetCookies()
	if err != nil {
		return nil, err
	}

	s := a.getSnapshot()

	log.Printf("Scraping %d posts from For You feed...", s.config.Scraping.PostsPerScrape)
	posts, err := s.scraper.ScrapeForYou(ctx, cookies, s.config.Scraping.PostsPerScrape)
	if err != nil {
		return nil, err
	}
	log.Printf("Scraped %d posts", len(posts))

	// Cache output
	if cachePath, err := store.SaveStepOutput(store.Step1Posts, posts); err != nil {
		log.Printf("Failed to cache posts: %v", err)
	} else {
		log.Printf("Cached posts to: %s", cachePath)
	}

	return posts, nil
}

// AnalyzePosts performs Step 2: Analyze posts with LLM for relevance scoring.
// Logs progress and caches output to step2_analyses.
func (a *App) AnalyzePosts(ctx context.Context, posts []types.Post) ([]types.Analysis, error) {
	log.Println("Analyzing posts with LLM...")

	s := a.getSnapshot()
	analyses, err := s.analyzer.AnalyzePosts(ctx, posts)
	if err != nil {
		return nil, err
	}
	log.Printf("Analyzed %d posts", len(analyses))

	// Cache output
	if cachePath, err := store.SaveStepOutput(store.Step2Analyses, analyses); err != nil {
		log.Printf("Failed to cache analyses: %v", err)
	} else {
		log.Printf("Cached analyses to: %s", cachePath)
	}

	return analyses, nil
}

// FilterByRelevance performs Step 3: Filter posts by relevance threshold.
// Logs progress and caches output to step3_filtered.
func (a *App) FilterByRelevance(posts []types.Post, analyses []types.Analysis) []types.PostWithAnalysis {
	s := a.getSnapshot()

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

	// Cache output
	if cachePath, err := store.SaveStepOutput(store.Step3Filtered, relevantPosts); err != nil {
		log.Printf("Failed to cache filtered posts: %v", err)
	} else {
		log.Printf("Cached filtered posts to: %s", cachePath)
	}

	return relevantPosts
}

// FetchContext performs Step 4: Fetch replies for posts that need context.
// Logs progress and caches output to step4_context.
// Returns a new slice with context populated (does not modify the input).
func (a *App) FetchContext(ctx context.Context, posts []types.PostWithAnalysis) ([]types.PostWithAnalysis, error) {
	cookies, err := a.authManager.GetCookies()
	if err != nil {
		return nil, err
	}

	s := a.getSnapshot()

	// Create a copy to avoid mutating the input
	result := make([]types.PostWithAnalysis, len(posts))
	copy(result, posts)

	if s.config.Digest.IncludeContext {
		log.Println("Fetching context for relevant posts...")
		for i := range result {
			if result[i].Analysis != nil && result[i].Analysis.NeedsContext {
				log.Printf("Fetching replies for post %s...", result[i].Post.ID)
				replies, err := s.scraper.ScrapeThread(ctx, cookies, result[i].Post.OriginalURL, 3)
				if err != nil {
					log.Printf("Failed to fetch replies for %s: %v", result[i].Post.ID, err)
					continue
				}
				result[i].Context = replies
				log.Printf("Got %d replies for post %s", len(replies), result[i].Post.ID)
			}
		}
	}

	// Cache output
	if cachePath, err := store.SaveStepOutput(store.Step4Context, result); err != nil {
		log.Printf("Failed to cache posts with context: %v", err)
	} else {
		log.Printf("Cached posts with context to: %s", cachePath)
	}

	return result, nil
}

// BuildDigest performs Step 5: Build and save the digest.
// Caches the markdown to step5_digests and saves to user output directory.
// Returns the path to the saved digest file.
func (a *App) BuildDigest(posts []types.PostWithAnalysis, totalScraped int) (string, error) {
	log.Println("Building digest...")

	s := a.getSnapshot()
	builder := digest.New(s.config.Digest.OutputDir, s.config.Digest.MaxPosts)

	content, err := builder.Render(posts, totalScraped)
	if err != nil {
		return "", err
	}

	// Cache markdown
	if cachePath, err := store.SaveTextOutput(store.Step5Digests, content.Markdown, ".md"); err != nil {
		log.Printf("Failed to cache digest: %v", err)
	} else {
		log.Printf("Cached digest to: %s", cachePath)
	}

	// Save to user output directory
	d, err := builder.Save(content)
	if err != nil {
		return "", err
	}

	log.Printf("Digest saved to: %s (%d posts)", d.FilePath, d.PostCount)
	return d.FilePath, nil
}

// =============================================================================
// Orchestration Methods
// =============================================================================

// GenerateDigest performs the full scrape -> analyze -> build digest flow.
func (a *App) GenerateDigest() error {
	log.Println("Generate Digest triggered...")

	if !a.authManager.IsAuthenticated() {
		log.Println("Not authenticated - please login to X first")
		return nil
	}

	ctx := context.Background()

	// Step 1: Scrape posts
	posts, err := a.ScrapeForYou(ctx)
	if err != nil {
		log.Printf("Scrape failed: %v", err)
		return err
	}
	if len(posts) == 0 {
		log.Println("No posts scraped - nothing to analyze")
		return nil
	}

	// Step 2: Analyze posts with LLM
	analyses, err := a.AnalyzePosts(ctx, posts)
	if err != nil {
		log.Printf("Analysis failed: %v", err)
		return err
	}

	// Step 3: Filter by relevance threshold
	relevantPosts := a.FilterByRelevance(posts, analyses)
	if len(relevantPosts) == 0 {
		log.Println("No posts above relevance threshold - no digest generated")
		return nil
	}

	// // Step 4: Fetch context for posts that need it
	// postsWithContext, err := a.FetchContext(ctx, relevantPosts)
	// if err != nil {
	// 	log.Printf("Failed to fetch context: %v", err)
	// 	return err
	// }

	// Step 5: Build and save digest
	_, err = a.BuildDigest(relevantPosts, len(posts))
	if err != nil {
		log.Printf("Failed to build digest: %v", err)
		return err
	}

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
