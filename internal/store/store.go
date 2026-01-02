package store

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Store handles all database operations
type Store struct {
	db *sql.DB
}

// New creates a new Store with SQLite backend
func New(dbPath string) (*Store, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}

	return s, nil
}

// Close closes the database connection
func (s *Store) Close() error {
	return s.db.Close()
}

// migrate creates the database schema
func (s *Store) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS posts (
		id TEXT PRIMARY KEY,
		author_handle TEXT NOT NULL,
		author_name TEXT,
		content TEXT NOT NULL,
		media_urls TEXT,
		timestamp DATETIME,
		likes INTEGER,
		retweets INTEGER,
		replies INTEGER,
		quote_tweets INTEGER,
		is_retweet BOOLEAN,
		is_quote_tweet BOOLEAN,
		is_reply BOOLEAN,
		original_url TEXT,
		scraped_at DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS analysis (
		post_id TEXT PRIMARY KEY REFERENCES posts(id),
		relevance_score REAL,
		topics TEXT,
		summary TEXT,
		needs_context BOOLEAN,
		analyzed_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS digest_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		post_id TEXT REFERENCES posts(id),
		digest_sent_at DATETIME NOT NULL,
		digest_type TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_posts_scraped_at ON posts(scraped_at);
	CREATE INDEX IF NOT EXISTS idx_posts_timestamp ON posts(timestamp);
	CREATE INDEX IF NOT EXISTS idx_analysis_score ON analysis(relevance_score);
	`

	_, err := s.db.Exec(schema)
	return err
}

// SavePost inserts or updates a post
func (s *Store) SavePost(p *Post) error {
	mediaJSON, _ := json.Marshal(p.MediaURLs)

	_, err := s.db.Exec(`
		INSERT INTO posts (id, author_handle, author_name, content, media_urls,
			timestamp, likes, retweets, replies, quote_tweets,
			is_retweet, is_quote_tweet, is_reply, original_url, scraped_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			likes = excluded.likes,
			retweets = excluded.retweets,
			replies = excluded.replies,
			quote_tweets = excluded.quote_tweets
	`, p.ID, p.AuthorHandle, p.AuthorName, p.Content, string(mediaJSON),
		p.Timestamp, p.Likes, p.Retweets, p.Replies, p.QuoteTweets,
		p.IsRetweet, p.IsQuoteTweet, p.IsReply, p.OriginalURL, p.ScrapedAt)

	return err
}

// SaveAnalysis inserts or updates analysis for a post
func (s *Store) SaveAnalysis(a *Analysis) error {
	topicsJSON, _ := json.Marshal(a.Topics)

	_, err := s.db.Exec(`
		INSERT INTO analysis (post_id, relevance_score, topics, summary, needs_context, analyzed_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(post_id) DO UPDATE SET
			relevance_score = excluded.relevance_score,
			topics = excluded.topics,
			summary = excluded.summary,
			needs_context = excluded.needs_context,
			analyzed_at = excluded.analyzed_at
	`, a.PostID, a.RelevanceScore, string(topicsJSON), a.Summary, a.NeedsContext, a.AnalyzedAt)

	return err
}

// GetUnanalyzedPosts returns posts that haven't been analyzed yet
func (s *Store) GetUnanalyzedPosts(limit int) ([]Post, error) {
	rows, err := s.db.Query(`
		SELECT p.id, p.author_handle, p.author_name, p.content, p.media_urls,
			p.timestamp, p.likes, p.retweets, p.replies, p.quote_tweets,
			p.is_retweet, p.is_quote_tweet, p.is_reply, p.original_url, p.scraped_at
		FROM posts p
		LEFT JOIN analysis a ON p.id = a.post_id
		WHERE a.post_id IS NULL
		ORDER BY p.scraped_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanPosts(rows)
}

// GetPostsForDigest returns analyzed posts above threshold not yet in a digest
func (s *Store) GetPostsForDigest(threshold float64, digestType string, limit int) ([]PostWithAnalysis, error) {
	rows, err := s.db.Query(`
		SELECT p.id, p.author_handle, p.author_name, p.content, p.media_urls,
			p.timestamp, p.likes, p.retweets, p.replies, p.quote_tweets,
			p.is_retweet, p.is_quote_tweet, p.is_reply, p.original_url, p.scraped_at,
			a.relevance_score, a.topics, a.summary, a.needs_context, a.analyzed_at
		FROM posts p
		JOIN analysis a ON p.id = a.post_id
		LEFT JOIN digest_history dh ON p.id = dh.post_id AND dh.digest_type = ?
		WHERE a.relevance_score >= ? AND dh.id IS NULL
		ORDER BY a.relevance_score DESC
		LIMIT ?
	`, digestType, threshold, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []PostWithAnalysis
	for rows.Next() {
		var p Post
		var a Analysis
		var mediaJSON, topicsJSON string

		err := rows.Scan(
			&p.ID, &p.AuthorHandle, &p.AuthorName, &p.Content, &mediaJSON,
			&p.Timestamp, &p.Likes, &p.Retweets, &p.Replies, &p.QuoteTweets,
			&p.IsRetweet, &p.IsQuoteTweet, &p.IsReply, &p.OriginalURL, &p.ScrapedAt,
			&a.RelevanceScore, &topicsJSON, &a.Summary, &a.NeedsContext, &a.AnalyzedAt,
		)
		if err != nil {
			return nil, err
		}

		json.Unmarshal([]byte(mediaJSON), &p.MediaURLs)
		json.Unmarshal([]byte(topicsJSON), &a.Topics)
		a.PostID = p.ID

		results = append(results, PostWithAnalysis{Post: p, Analysis: &a})
	}

	return results, rows.Err()
}

// MarkDigested records that posts were included in a digest
func (s *Store) MarkDigested(postIDs []string, digestType string) error {
	now := time.Now()
	for _, id := range postIDs {
		_, err := s.db.Exec(`
			INSERT INTO digest_history (post_id, digest_sent_at, digest_type)
			VALUES (?, ?, ?)
		`, id, now, digestType)
		if err != nil {
			return err
		}
	}
	return nil
}

// PostExists checks if a post ID already exists
func (s *Store) PostExists(id string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM posts WHERE id = ?)`, id).Scan(&exists)
	return exists, err
}

func scanPosts(rows *sql.Rows) ([]Post, error) {
	var posts []Post
	for rows.Next() {
		var p Post
		var mediaJSON string

		err := rows.Scan(
			&p.ID, &p.AuthorHandle, &p.AuthorName, &p.Content, &mediaJSON,
			&p.Timestamp, &p.Likes, &p.Retweets, &p.Replies, &p.QuoteTweets,
			&p.IsRetweet, &p.IsQuoteTweet, &p.IsReply, &p.OriginalURL, &p.ScrapedAt,
		)
		if err != nil {
			return nil, err
		}

		json.Unmarshal([]byte(mediaJSON), &p.MediaURLs)
		posts = append(posts, p)
	}
	return posts, rows.Err()
}
