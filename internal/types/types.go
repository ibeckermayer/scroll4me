package types

import "time"

// Post represents a scraped X post
type Post struct {
	ID           string    `json:"id"`
	AuthorHandle string    `json:"author_handle"`
	AuthorName   string    `json:"author_name"`
	Content      string    `json:"content"`
	MediaURLs    []string  `json:"media_urls"`
	Timestamp    time.Time `json:"timestamp"`
	Likes        int       `json:"likes"`
	Retweets     int       `json:"retweets"`
	Replies      int       `json:"replies"`
	QuoteTweets  int       `json:"quote_tweets"`
	IsRetweet    bool      `json:"is_retweet"`
	IsQuoteTweet bool      `json:"is_quote_tweet"`
	IsReply      bool      `json:"is_reply"`
	OriginalURL  string    `json:"original_url"`
	ScrapedAt    time.Time `json:"scraped_at"`
}

// Analysis represents LLM analysis results for a post
type Analysis struct {
	PostID         string    `json:"post_id"`
	RelevanceScore float64   `json:"relevance_score"`
	Topics         []string  `json:"topics"`
	Summary        string    `json:"summary"`
	NeedsContext   bool      `json:"needs_context"`
	AnalyzedAt     time.Time `json:"analyzed_at"`
}

// PostWithAnalysis combines a post with its analysis and optional context
type PostWithAnalysis struct {
	Post     Post
	Analysis *Analysis
	Context  []Post // Replies/thread context if fetched
}
