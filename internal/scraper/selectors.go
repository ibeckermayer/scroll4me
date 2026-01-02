package scraper

// X.com DOM selectors
// These are isolated here because X changes their DOM frequently
// Update these when scraping breaks

const (
	// Feed selectors
	FeedContainer = `[data-testid="primaryColumn"]`
	TweetArticle  = `article[data-testid="tweet"]`

	// Tweet content selectors
	TweetText       = `[data-testid="tweetText"]`
	TweetAuthor     = `[data-testid="User-Name"]`
	TweetTimestamp  = `time`
	TweetLink       = `a[href*="/status/"]`
	TweetMedia      = `[data-testid="tweetPhoto"], [data-testid="videoPlayer"]`

	// Engagement selectors
	ReplyCount    = `[data-testid="reply"]`
	RetweetCount  = `[data-testid="retweet"]`
	LikeCount     = `[data-testid="like"]`

	// Tweet type indicators
	RetweetIndicator  = `[data-testid="socialContext"]`
	QuoteIndicator    = `[data-testid="quoteTweet"]`
	ReplyIndicator    = `[data-testid="tweet"] a[href*="/status/"][dir="ltr"]`

	// Login page indicators (for detecting auth state)
	HomeIndicator = `[data-testid="SideNav_NewTweet_Button"]`
	LoginForm     = `[data-testid="loginButton"]`
)

// Common wait conditions
const (
	WaitForFeed   = FeedContainer
	WaitForTweets = TweetArticle
)
