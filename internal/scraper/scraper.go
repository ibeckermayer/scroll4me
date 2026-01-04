package scraper

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"

	"github.com/ibeckermayer/scroll4me/internal/browser"
	"github.com/ibeckermayer/scroll4me/internal/types"
)

// Scraper handles extracting posts from X.com
type Scraper struct {
	headless bool
	// If true and not in headless mode, will pause after scraping and
	// wait for the browser to close before continuing. This is useful
	// for debugging the scrape process.
	debugPauseAfterScrape bool
}

// New creates a new scraper
func New(headless bool, debugPauseAfterScrape bool) *Scraper {
	return &Scraper{headless: headless, debugPauseAfterScrape: debugPauseAfterScrape}
}

// extractFunc is a function that extracts posts from the current view
type extractFunc func(ctx context.Context) ([]types.Post, error)

// scrollAndCollectParams configures the scroll-and-collect loop
type scrollAndCollectParams struct {
	maxCount          int
	maxScrollAttempts int
	extractor         extractFunc
	logPrefix         string
	baseDelayMs       int
	delayIncrementMs  int
}

// scrollAndCollect is the common scroll-collect-dedupe loop used by extractPosts and extractReplies
func (s *Scraper) scrollAndCollect(ctx context.Context, p scrollAndCollectParams) ([]types.Post, error) {
	var posts []types.Post
	seenIDs := make(map[string]bool)

	for scrollAttempts := 0; scrollAttempts < p.maxScrollAttempts; scrollAttempts++ {
		newPosts, err := p.extractor(ctx)
		if err != nil {
			return nil, err
		}

		// Add unique posts, break if we hit maxCount
		newUniqueCount := 0
		for _, post := range newPosts {
			if !seenIDs[post.ID] {
				seenIDs[post.ID] = true
				posts = append(posts, post)
				newUniqueCount++
				if len(posts) >= p.maxCount {
					break
				}
			}
		}

		log.Printf("%s %d/%d: found %d visible, %d new unique (total: %d/%d)",
			p.logPrefix, scrollAttempts+1, p.maxScrollAttempts,
			len(newPosts), newUniqueCount, len(posts), p.maxCount)

		if len(posts) >= p.maxCount {
			break
		}

		if err := s.scroll(ctx); err != nil {
			return nil, err
		}

		// Randomized wait for human-like timing
		jitter := rand.Intn(200)
		wait := p.baseDelayMs + jitter + scrollAttempts*p.delayIncrementMs
		time.Sleep(time.Duration(wait) * time.Millisecond)
	}

	return posts, nil
}

// ScrapeForYou fetches posts from the For You feed
func (s *Scraper) ScrapeForYou(ctx context.Context, cookies []*network.Cookie, count int) ([]types.Post, error) {
	log.Printf("Starting scrape for %d posts (headless=%v, debugPauseAfterScrape=%v)", count, s.headless, s.debugPauseAfterScrape)

	// Create browser context with anti-bot-detection options
	opts := browser.Options(s.headless)

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()

	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()

	// Set timeout for the entire scrape operation
	timedBrowserCtx, timeoutCancel := context.WithTimeout(browserCtx, 5*time.Minute)
	defer timeoutCancel()

	// Inject cookies before navigation
	log.Printf("Injecting %d cookies...", len(cookies))
	if err := s.injectCookies(timedBrowserCtx, cookies); err != nil {
		return nil, fmt.Errorf("failed to inject cookies: %w", err)
	}

	// Navigate to home feed
	log.Printf("Navigating to x.com/home...")
	if err := chromedp.Run(timedBrowserCtx,
		chromedp.Navigate("https://x.com/home"),
		chromedp.WaitVisible(WaitForTweets, chromedp.ByQuery),
	); err != nil {
		return nil, fmt.Errorf("failed to load feed: %w", err)
	}
	log.Printf("Feed loaded, beginning extraction...")

	// Scrape posts with scrolling
	posts, err := s.extractPosts(timedBrowserCtx, count)
	if s.debugPauseAfterScrape {
		if s.headless {
			log.Println("Skipping debug pause after scrape in headless mode")
		} else {
			log.Printf("Pausing for debug after scraping. extractPosts returned with error: %v", err)
			fmt.Print("Press Enter to continue...")
			fmt.Scanln()
			log.Println("Continuing...")
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to extract posts: %w", err)
	}

	return posts, nil
}

// injectCookies sets cookies in the browser context
func (s *Scraper) injectCookies(ctx context.Context, cookies []*network.Cookie) error {
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			for _, c := range cookies {
				err := network.SetCookie(c.Name, c.Value).
					WithDomain(c.Domain).
					WithPath(c.Path).
					WithSecure(c.Secure).
					WithHTTPOnly(c.HTTPOnly).
					WithSameSite(c.SameSite).
					Do(ctx)

				if err != nil {
					return err
				}
			}
			return nil
		}),
	)
}

// extractPosts scrolls and extracts posts from the feed
func (s *Scraper) extractPosts(ctx context.Context, count int) ([]types.Post, error) {
	posts, err := s.scrollAndCollect(ctx, scrollAndCollectParams{
		maxCount:          count,
		maxScrollAttempts: count / 5, // Rough estimate: ~5 posts per scroll
		extractor:         s.extractVisiblePosts,
		logPrefix:         "Scroll",
		baseDelayMs:       500,
		delayIncrementMs:  100,
	})
	if err != nil {
		return nil, err
	}

	log.Printf("Extraction complete: %d posts collected", len(posts))
	return posts, nil
}

// rawPost represents the raw data extracted from the DOM via JavaScript
type rawPost struct {
	ID           string   `json:"id"`
	AuthorHandle string   `json:"authorHandle"`
	AuthorName   string   `json:"authorName"`
	Content      string   `json:"content"`
	MediaURLs    []string `json:"mediaUrls"`
	Timestamp    string   `json:"timestamp"`
	Likes        string   `json:"likes"`
	Retweets     string   `json:"retweets"`
	Replies      string   `json:"replies"`
	IsRetweet    bool     `json:"isRetweet"`
	IsQuoteTweet bool     `json:"isQuoteTweet"`
	IsReply      bool     `json:"isReply"`
	OriginalURL  string   `json:"originalUrl"`
}

// expandTruncatedTweets clicks "Show more" buttons on visible tweets to reveal full content.
// Uses variable delays (250ms-1000ms) between clicks to appear more human-like.
func (s *Scraper) expandTruncatedTweets(ctx context.Context) error {
	// Find all "Show more" buttons currently visible
	var buttonCount int
	countJS := fmt.Sprintf(`document.querySelectorAll('%s').length`, TweetShowMore)

	if err := chromedp.Run(ctx, chromedp.Evaluate(countJS, &buttonCount)); err != nil {
		return fmt.Errorf("failed to count show more buttons: %w", err)
	}

	if buttonCount == 0 {
		return nil
	}

	log.Printf("Expanding %d truncated tweets...", buttonCount)

	// Click each button with a variable delay
	for i := 0; i < buttonCount; i++ {
		// Click the first visible "Show more" button (they disappear after clicking)
		clickJS := fmt.Sprintf(`
			(function() {
				const btn = document.querySelector('%s');
				if (btn) {
					btn.click();
					return true;
				}
				return false;
			})()
		`, TweetShowMore)

		var clicked bool
		if err := chromedp.Run(ctx, chromedp.Evaluate(clickJS, &clicked)); err != nil {
			log.Printf("Failed to click show more button %d: %v", i, err)
			continue
		}

		if !clicked {
			break // No more buttons found
		}

		// Variable delay: 250ms to 500ms
		delay := time.Duration(250+rand.Intn(250)) * time.Millisecond
		time.Sleep(delay)
	}

	return nil
}

// extractVisiblePosts parses currently visible tweets
func (s *Scraper) extractVisiblePosts(ctx context.Context) ([]types.Post, error) {
	// First, expand any truncated tweets to get full content
	if err := s.expandTruncatedTweets(ctx); err != nil {
		log.Printf("Warning: failed to expand truncated tweets: %v", err)
		// Continue anyway - we'll get partial content
	}

	var rawPosts []rawPost

	// JavaScript to extract tweet data from the DOM
	extractJS := `
		(function() {
			const tweets = document.querySelectorAll('article[data-testid="tweet"]');
			const results = [];

			tweets.forEach(el => {
				try {
					// Extract tweet ID from status link
					const statusLink = el.querySelector('a[href*="/status/"]');
					const id = statusLink?.href?.match(/status\/(\d+)/)?.[1];
					if (!id) return; // Skip if no ID found

					// Extract author info from User-Name element
					const userNameEl = el.querySelector('[data-testid="User-Name"]');
					let authorHandle = '';
					let authorName = '';
					if (userNameEl) {
						// The handle is in a link, display name is usually the first text
						const handleLink = userNameEl.querySelector('a[href^="/"]');
						if (handleLink) {
							authorHandle = handleLink.getAttribute('href')?.replace('/', '') || '';
						}
						// Get display name from the first span with text
						const nameSpan = userNameEl.querySelector('span');
						authorName = nameSpan?.textContent || '';
					}

					// Extract tweet text
					const tweetTextEl = el.querySelector('[data-testid="tweetText"]');
					const content = tweetTextEl?.textContent || '';

					// Extract media URLs
					const mediaUrls = [];
					const mediaEls = el.querySelectorAll('[data-testid="tweetPhoto"] img, [data-testid="videoPlayer"] video');
					mediaEls.forEach(m => {
						const src = m.src || m.poster;
						if (src) mediaUrls.push(src);
					});

					// Extract timestamp
					const timeEl = el.querySelector('time');
					const timestamp = timeEl?.getAttribute('datetime') || '';

					// Extract engagement metrics (these are displayed as aria-label or text)
					const getMetric = (testId) => {
						const metricEl = el.querySelector('[data-testid="' + testId + '"]');
						if (!metricEl) return '0';
						// Try aria-label first (e.g., "123 Replies")
						const ariaLabel = metricEl.getAttribute('aria-label');
						if (ariaLabel) {
							const match = ariaLabel.match(/^([\d,.]+[KkMm]?)/);
							return match ? match[1] : '0';
						}
						// Fallback to text content
						const text = metricEl.textContent?.trim();
						return text || '0';
					};

					const replies = getMetric('reply');
					const retweets = getMetric('retweet');
					const likes = getMetric('like');

					// Check if it's a retweet (has social context indicating repost)
					const socialContext = el.querySelector('[data-testid="socialContext"]');
					const isRetweet = socialContext?.textContent?.toLowerCase().includes('repost') ||
					                  socialContext?.textContent?.toLowerCase().includes('retweeted') || false;

					// Check if it's a quote tweet
					const isQuoteTweet = el.querySelector('[data-testid="quoteTweet"]') !== null;

					// Check if it's a reply (has "Replying to" text)
					const isReply = el.textContent?.includes('Replying to') || false;

					// Build the original URL
					const originalUrl = statusLink?.href || '';

					results.push({
						id,
						authorHandle,
						authorName,
						content,
						mediaUrls,
						timestamp,
						likes,
						retweets,
						replies,
						isRetweet,
						isQuoteTweet,
						isReply,
						originalUrl
					});
				} catch (e) {
					console.error('Error extracting tweet:', e);
				}
			});

			return results;
		})()
	`

	err := chromedp.Run(ctx,
		chromedp.Evaluate(extractJS, &rawPosts),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to extract posts from DOM: %w", err)
	}

	// Convert raw posts to types.Post
	posts := make([]types.Post, 0, len(rawPosts))
	now := time.Now()

	for _, rp := range rawPosts {
		if rp.ID == "" {
			continue
		}

		// Parse timestamp
		var timestamp time.Time
		if rp.Timestamp != "" {
			if parsed, err := time.Parse(time.RFC3339, rp.Timestamp); err == nil {
				timestamp = parsed
			}
		}

		post := types.Post{
			ID:           rp.ID,
			AuthorHandle: rp.AuthorHandle,
			AuthorName:   rp.AuthorName,
			Content:      rp.Content,
			MediaURLs:    rp.MediaURLs,
			Timestamp:    timestamp,
			Likes:        parseMetric(rp.Likes),
			Retweets:     parseMetric(rp.Retweets),
			Replies:      parseMetric(rp.Replies),
			QuoteTweets:  0, // Not easily available from the DOM
			IsRetweet:    rp.IsRetweet,
			IsQuoteTweet: rp.IsQuoteTweet,
			IsReply:      rp.IsReply,
			OriginalURL:  rp.OriginalURL,
			ScrapedAt:    now,
		}
		posts = append(posts, post)
	}

	return posts, nil
}

// scroll scrolls the page down
func (s *Scraper) scroll(ctx context.Context) error {
	return chromedp.Run(ctx,
		chromedp.Evaluate(`window.scrollBy(0, window.innerHeight)`, nil),
	)
}

// ScrapeThread fetches replies for a specific post (for context enrichment)
func (s *Scraper) ScrapeThread(ctx context.Context, cookies []*network.Cookie, postURL string, replyCount int) ([]types.Post, error) {
	log.Printf("Starting thread scrape for %d replies: %s", replyCount, postURL)

	// Create browser context with anti-bot-detection options
	opts := browser.Options(s.headless)

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()

	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()

	// Set timeout for the thread scrape operation
	browserCtx, timeoutCancel := context.WithTimeout(browserCtx, 2*time.Minute)
	defer timeoutCancel()

	// Inject cookies before navigation
	log.Printf("Injecting %d cookies...", len(cookies))
	if err := s.injectCookies(browserCtx, cookies); err != nil {
		return nil, fmt.Errorf("failed to inject cookies: %w", err)
	}

	// Navigate to the post URL
	log.Printf("Navigating to thread...")
	if err := chromedp.Run(browserCtx,
		chromedp.Navigate(postURL),
		chromedp.WaitVisible(WaitForTweets, chromedp.ByQuery),
	); err != nil {
		return nil, fmt.Errorf("failed to load post: %w", err)
	}
	log.Printf("Thread loaded, waiting for replies...")

	// Wait a bit for replies to load
	// TODO: Find a more reliable way to wait for replies to load
	time.Sleep(2 * time.Second)

	// Extract replies (skip the first tweet which is the main post)
	replies, err := s.extractReplies(browserCtx, replyCount)
	if err != nil {
		return nil, fmt.Errorf("failed to extract replies: %w", err)
	}

	log.Printf("Thread scrape complete: %d replies collected", len(replies))
	return replies, nil
}

// extractReplies extracts reply tweets from a thread page
func (s *Scraper) extractReplies(ctx context.Context, count int) ([]types.Post, error) {
	return s.scrollAndCollect(ctx, scrollAndCollectParams{
		maxCount:          count,
		maxScrollAttempts: count/3 + 5, // Replies are often fewer per scroll
		extractor:         s.extractVisibleReplies,
		logPrefix:         "Reply scroll",
		baseDelayMs:       800,
		delayIncrementMs:  150,
	})
}

// extractVisibleReplies extracts reply tweets from the current view
// This skips the main tweet (first article) and gets the replies
func (s *Scraper) extractVisibleReplies(ctx context.Context) ([]types.Post, error) {
	var rawPosts []rawPost

	// JavaScript to extract reply tweets (skipping the main tweet)
	extractJS := `
		(function() {
			const tweets = document.querySelectorAll('article[data-testid="tweet"]');
			const results = [];
			let skippedFirst = false;

			tweets.forEach(el => {
				// Skip the first tweet (main post)
				if (!skippedFirst) {
					skippedFirst = true;
					return;
				}

				try {
					// Extract tweet ID from status link
					const statusLink = el.querySelector('a[href*="/status/"]');
					const id = statusLink?.href?.match(/status\/(\d+)/)?.[1];
					if (!id) return;

					// Extract author info
					const userNameEl = el.querySelector('[data-testid="User-Name"]');
					let authorHandle = '';
					let authorName = '';
					if (userNameEl) {
						const handleLink = userNameEl.querySelector('a[href^="/"]');
						if (handleLink) {
							authorHandle = handleLink.getAttribute('href')?.replace('/', '') || '';
						}
						const nameSpan = userNameEl.querySelector('span');
						authorName = nameSpan?.textContent || '';
					}

					// Extract tweet text
					const tweetTextEl = el.querySelector('[data-testid="tweetText"]');
					const content = tweetTextEl?.textContent || '';

					// Extract media URLs
					const mediaUrls = [];
					const mediaEls = el.querySelectorAll('[data-testid="tweetPhoto"] img, [data-testid="videoPlayer"] video');
					mediaEls.forEach(m => {
						const src = m.src || m.poster;
						if (src) mediaUrls.push(src);
					});

					// Extract timestamp
					const timeEl = el.querySelector('time');
					const timestamp = timeEl?.getAttribute('datetime') || '';

					// Extract engagement metrics
					const getMetric = (testId) => {
						const metricEl = el.querySelector('[data-testid="' + testId + '"]');
						if (!metricEl) return '0';
						const ariaLabel = metricEl.getAttribute('aria-label');
						if (ariaLabel) {
							const match = ariaLabel.match(/^([\d,.]+[KkMm]?)/);
							return match ? match[1] : '0';
						}
						return metricEl.textContent?.trim() || '0';
					};

					const replies = getMetric('reply');
					const retweets = getMetric('retweet');
					const likes = getMetric('like');

					const originalUrl = statusLink?.href || '';

					results.push({
						id,
						authorHandle,
						authorName,
						content,
						mediaUrls,
						timestamp,
						likes,
						retweets,
						replies,
						isRetweet: false,
						isQuoteTweet: false,
						isReply: true,
						originalUrl
					});
				} catch (e) {
					console.error('Error extracting reply:', e);
				}
			});

			return results;
		})()
	`

	err := chromedp.Run(ctx,
		chromedp.Evaluate(extractJS, &rawPosts),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to extract replies from DOM: %w", err)
	}

	// Convert raw posts to types.Post
	posts := make([]types.Post, 0, len(rawPosts))
	now := time.Now()

	for _, rp := range rawPosts {
		if rp.ID == "" {
			continue
		}

		var timestamp time.Time
		if rp.Timestamp != "" {
			if parsed, err := time.Parse(time.RFC3339, rp.Timestamp); err == nil {
				timestamp = parsed
			}
		}

		post := types.Post{
			ID:           rp.ID,
			AuthorHandle: rp.AuthorHandle,
			AuthorName:   rp.AuthorName,
			Content:      rp.Content,
			MediaURLs:    rp.MediaURLs,
			Timestamp:    timestamp,
			Likes:        parseMetric(rp.Likes),
			Retweets:     parseMetric(rp.Retweets),
			Replies:      parseMetric(rp.Replies),
			QuoteTweets:  0,
			IsRetweet:    false,
			IsQuoteTweet: false,
			IsReply:      true,
			OriginalURL:  rp.OriginalURL,
			ScrapedAt:    now,
		}
		posts = append(posts, post)
	}

	return posts, nil
}

// parseMetric converts abbreviated metric strings like "1.2K", "5.7M", or "423" to integers
func parseMetric(s string) int {
	if s == "" {
		return 0
	}

	// Clean up the string
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "") // Remove commas (e.g., "1,234")

	// Handle abbreviated formats (K for thousands, M for millions)
	multiplier := 1.0
	if strings.HasSuffix(strings.ToUpper(s), "K") {
		multiplier = 1000
		s = s[:len(s)-1]
	} else if strings.HasSuffix(strings.ToUpper(s), "M") {
		multiplier = 1000000
		s = s[:len(s)-1]
	}

	// Parse the numeric part
	value, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}

	return int(value * multiplier)
}
