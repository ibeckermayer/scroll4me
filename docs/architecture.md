# scroll4me - Architecture & Design

## Overview

scroll4me is a desktop application that automatically scrolls X (Twitter) on behalf of the user, filters content based on interests using LLM analysis, and delivers a curated digest via email at scheduled intervals.

**Goal**: Eliminate doomscrolling while staying informed.

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              scroll4me                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌────────────────────┐   (on demand)   ┌────────────────────┐             │
│  │ Settings Window    │◄────────────────│   System Tray      │             │
│  │ (Wails attached)   │                 │   (Primary UI)     │             │
│  │                    │                 │                    │             │
│  │ - X login trigger  │                 │ Runs in background │             │
│  │ - Interests        │                 │ Shows status icon  │             │
│  │ - Email config     │                 │ Dropdown menu      │             │
│  │ - Schedule times   │                 │                    │             │
│  │ - LLM API key      │                 └────────────────────┘             │
│  └─────────┬──────────┘                                                     │
│            │                                                                 │
│            │ triggers                                                        │
│            ▼                                                                 │
│  ┌────────────────────┐         ┌────────────────────┐                     │
│  │  Auth Manager      │────────▶│  Cookie Store      │                     │
│  │  (chromedp headful)│         │  (encrypted file)  │                     │
│  └────────────────────┘         └─────────┬──────────┘                     │
│                                           │                                  │
│            ┌──────────────────────────────┘                                 │
│            │                                                                 │
│            ▼                                                                 │
│  ┌────────────────────┐         ┌────────────────────┐                     │
│  │     Scheduler      │────────▶│     Scraper        │                     │
│  │  (cron-like jobs)  │         │ (chromedp headless)│                     │
│  │                    │         │                    │                     │
│  │  - Scrape job      │         │  - Load cookies    │                     │
│  │  - Digest job      │         │  - Navigate feed   │                     │
│  └────────────────────┘         │  - Scroll & extract│                     │
│            │                    └─────────┬──────────┘                     │
│            │                              │                                  │
│            │                              ▼                                  │
│            │                    ┌────────────────────┐                     │
│            │                    │   Content Store    │                     │
│            │                    │     (SQLite)       │                     │
│            │                    │                    │                     │
│            │                    │  - Raw posts       │                     │
│            │                    │  - Analysis cache  │                     │
│            │                    │  - Digest history  │                     │
│            │                    └─────────┬──────────┘                     │
│            │                              │                                  │
│            │ triggers                     │                                  │
│            ▼                              ▼                                  │
│  ┌────────────────────┐         ┌────────────────────┐                     │
│  │     Analyzer       │◀────────│  Pending Posts     │                     │
│  │   (LLM API call)   │         │                    │                     │
│  │                    │         └────────────────────┘                     │
│  │  - Score relevance │                                                     │
│  │  - Extract topics  │                                                     │
│  │  - Flag for context│                                                     │
│  └─────────┬──────────┘                                                     │
│            │                                                                 │
│            ▼                                                                 │
│  ┌────────────────────┐         ┌────────────────────┐                     │
│  │  Context Enricher  │────────▶│     Scraper        │  (optional fetch    │
│  │    (optional)      │         │  (thread replies)  │   of replies)       │
│  └─────────┬──────────┘         └────────────────────┘                     │
│            │                                                                 │
│            ▼                                                                 │
│  ┌────────────────────┐         ┌────────────────────┐                     │
│  │   Digest Builder   │────────▶│     Notifier       │                     │
│  │                    │         │     (email)        │                     │
│  │  - Filter by score │         │                    │                     │
│  │  - Group by topic  │         │  - SMTP / SendGrid │                     │
│  │  - Format HTML     │         │  - Send digest     │                     │
│  └────────────────────┘         └────────────────────┘                     │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## System Tray UX

The app runs as a menu bar widget (macOS) / system tray app (Windows/Linux).

```
┌─────────────────────────────┐
│ 🔄 scroll4me          [icon]│  ← Menu bar / System tray
└─────────────────────────────┘
         │
         ▼ (click)
┌─────────────────────────────┐
│ ✓ Running                   │
│ ─────────────────────────── │
│ Last scrape: 10 min ago     │
│ Next digest: 6:00 PM        │
│ ─────────────────────────── │
│ ▶ Scrape Now                │
│ ▶ Send Digest Now           │
│ ─────────────────────────── │
│ ⚙ Settings...               │  ← Opens Wails window
│ 📋 View Last Digest         │  ← Opens in browser
│ ─────────────────────────── │
│ ⏸ Pause / ▶ Resume          │
│ ✕ Quit                      │
└─────────────────────────────┘
```

## App Behavior

- **Starts minimized to system tray** (no window on launch)
- **Tray icon** shows status (green = running, yellow = paused, red = error)
- **Left-click** opens dropdown menu with status and actions
- **Settings window** attached to tray, appears below/above icon, hides on focus loss
- **Scheduler runs in background** regardless of window state
- **Quit** from tray menu fully exits the app

## Components

### 1. System Tray + Settings Window (Wails v3)

**Purpose**: Provide a menu bar widget with attached settings window.

**System Tray Responsibilities**:

- Show status icon (running/paused/error)
- Display dropdown menu with status info and actions
- Trigger manual scrape/digest
- Pause/resume scheduler
- Open settings window on demand

**Settings Window Responsibilities**:

- Display settings form (attached to tray, hides on focus loss)
- Trigger X login flow (spawns chromedp)
- Save/load configuration
- Show login status (authenticated or not)

**Tech**: Wails v3 with native system tray support + HTML/CSS/JS frontend

**NOT responsible for**: Viewing digests (sent via email)

---

### 2. Auth Manager

**Purpose**: Handle X.com authentication via user-driven browser login.

**Flow**:

1. User clicks "Login to X" in Wails UI
2. Spawn chromedp in **headful** mode (visible Chrome window)
3. Navigate to `https://x.com/login`
4. User logs in manually (handles 2FA, CAPTCHAs, etc.)
5. Detect successful login (check for redirect to `/home` or presence of auth elements)
6. Extract all cookies via `network.GetAllCookies()`
7. Store cookies securely
8. Close browser window

**Detection of successful login**:

- URL contains `/home`
- OR presence of specific DOM element (e.g., compose tweet button)
- OR `auth_token` cookie exists

---

### 3. Cookie Store

**Purpose**: Securely persist X.com session cookies.

**Options** (in order of preference):

1. **OS Keychain** - macOS Keychain, Windows Credential Manager, Linux Secret Service
2. **Encrypted file** - AES-256 encrypted JSON file with key derived from machine-specific data

**Stored data**:

```json
{
  "cookies": [
    {"name": "auth_token", "value": "...", "domain": ".x.com", ...},
    {"name": "ct0", "value": "...", ...}
  ],
  "captured_at": "2026-01-15T10:30:00Z",
  "expires_at": "2026-03-01T10:30:00Z"
}
```

---

### 4. Scheduler

**Purpose**: Run scrape and digest jobs on a schedule.

**Implementation**:

- Use `robfig/cron` for Go-native cron scheduling
- Run as part of the main application (or as a separate background service)

**Jobs**:
| Job | Default Schedule | Description |
|-----|------------------|-------------|
| Scrape | Every 2 hours | Fetch new posts from For You feed |
| Digest (morning) | 7:00 AM | Compile and send morning digest |
| Digest (evening) | 6:00 PM | Compile and send evening digest |

**Considerations**:

- On macOS: Could use launchd for background execution
- On Windows: Could use Task Scheduler
- Cross-platform: Just run the app and let it schedule internally

---

### 5. Scraper

**Purpose**: Extract posts from X.com For You feed.

**Implementation**: chromedp in headless mode

**Flow**:

1. Load cookies from Cookie Store
2. Create new browser context with cookies
3. Navigate to `https://x.com/home`
4. Wait for feed to load
5. Scroll and extract posts (configurable count, e.g., 50-100 posts)
6. Parse each post into structured data
7. Store in Content Store
8. Handle pagination/infinite scroll

**Extracted data per post**:

```go
type Post struct {
    ID            string    // Tweet ID
    AuthorHandle  string    // @username
    AuthorName    string    // Display name
    Content       string    // Tweet text
    MediaURLs     []string  // Images, videos
    Timestamp     time.Time
    Likes         int
    Retweets      int
    Replies       int
    QuoteTweets   int
    IsRetweet     bool
    IsQuoteTweet  bool
    IsReply       bool
    OriginalURL   string    // Link to tweet
    ScrapedAt     time.Time
}
```

**Challenges**:

- X's DOM structure changes frequently
- Need resilient selectors
- Rate limiting / bot detection
- Handling "Show more" or truncated tweets

---

### 6. Content Store (SQLite)

**Purpose**: Persist scraped posts and analysis results.

**Schema**:

```sql
-- Raw scraped posts
CREATE TABLE posts (
    id TEXT PRIMARY KEY,           -- Tweet ID
    author_handle TEXT NOT NULL,
    author_name TEXT,
    content TEXT NOT NULL,
    media_urls TEXT,               -- JSON array
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

-- LLM analysis results
CREATE TABLE analysis (
    post_id TEXT PRIMARY KEY REFERENCES posts(id),
    relevance_score REAL,          -- 0.0 to 1.0
    topics TEXT,                   -- JSON array of detected topics
    summary TEXT,                  -- LLM-generated summary
    needs_context BOOLEAN,         -- Flag for thread enrichment
    analyzed_at DATETIME NOT NULL
);

-- Digest history (for deduplication)
CREATE TABLE digest_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    post_id TEXT REFERENCES posts(id),
    digest_sent_at DATETIME NOT NULL,
    digest_type TEXT               -- 'morning' or 'evening'
);

-- Indexes
CREATE INDEX idx_posts_scraped_at ON posts(scraped_at);
CREATE INDEX idx_posts_timestamp ON posts(timestamp);
CREATE INDEX idx_analysis_score ON analysis(relevance_score);
```

---

### 7. Analyzer (LLM)

**Purpose**: Score posts for relevance and extract topics.

**Input**: User's configured interests + post content

**Prompt structure**:

```
You are analyzing a social media post for relevance to the user's interests.

User interests: {interests}

Post:
Author: {author}
Content: {content}
Engagement: {likes} likes, {retweets} retweets, {replies} replies

Tasks:
1. Score relevance from 0.0 to 1.0
2. List detected topics (max 3)
3. Write a 1-sentence summary
4. Should we fetch replies for more context? (true/false)

Respond in JSON format:
{
  "relevance_score": 0.85,
  "topics": ["AI", "tech policy"],
  "summary": "Discussion of new AI regulation proposals",
  "needs_context": false
}
```

**API options**:

- Claude API (preferred)
- OpenAI API
- Local Ollama (for privacy-conscious users)

**Batching**: Process multiple posts in a single API call to reduce costs/latency.

---

### 8. Context Enricher (Optional)

**Purpose**: Fetch top replies for posts flagged as needing context.

**Trigger**: `analysis.needs_context = true`

**Flow**:

1. Navigate to post's original URL
2. Scroll to load top replies
3. Extract reply content
4. Append to post's context for digest

**Criteria for needing context**:

- High reply count relative to likes (controversial)
- Post is a question
- Post references external event
- LLM flags it

---

### 9. Digest Builder

**Purpose**: Compile analyzed posts into a formatted digest.

**Flow**:

1. Query posts with `relevance_score >= threshold` (configurable, default 0.6)
2. Exclude posts already in `digest_history`
3. Group by topic or keep chronological
4. Format as HTML email
5. Pass to Notifier

**Digest format**:

```
📰 Your X Digest - Morning, Jan 15

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

🔥 TOP STORIES

[Topic: AI]
@elonmusk: "Exciting announcement about..."
Summary: Tesla reveals new AI partnership
🔗 View on X

---

[Topic: Politics]
@breaking_news: "Breaking: Congress passes..."
Summary: New legislation affecting tech companies
💬 Notable reply: "@user: This means..."
🔗 View on X

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

📊 Stats: Analyzed 87 posts, included 12

Settings: scroll4me.app/settings
```

---

### 10. Notifier (Email)

**Purpose**: Send digest emails.

**Options**:

1. **SMTP** - User provides SMTP server credentials
2. **SendGrid** - API-based, simpler setup
3. **Resend** - Modern email API
4. **Self-hosted** - For advanced users

**Configuration**:

```json
{
  "email": {
    "provider": "smtp",
    "smtp_host": "smtp.gmail.com",
    "smtp_port": 587,
    "smtp_user": "user@gmail.com",
    "smtp_pass": "app-password",
    "from_address": "scroll4me@example.com",
    "to_address": "user@example.com"
  }
}
```

---

## Configuration File

Location: `~/.config/scroll4me/config.json` (or platform-appropriate path)

```json
{
  "version": 1,

  "interests": {
    "keywords": ["AI", "machine learning", "startups", "tech policy"],
    "priority_accounts": ["@elonmusk", "@sama", "@kaboringblog"],
    "muted_accounts": ["@spambot123"],
    "muted_keywords": ["crypto pump", "NFT drop"]
  },

  "scraping": {
    "posts_per_scrape": 100,
    "scrape_interval_hours": 2,
    "headless": true
  },

  "analysis": {
    "llm_provider": "claude",
    "api_key": "sk-...",
    "model": "claude-sonnet-4-20250514",
    "relevance_threshold": 0.6,
    "batch_size": 10
  },

  "digest": {
    "morning_time": "07:00",
    "evening_time": "18:00",
    "timezone": "America/New_York",
    "max_posts_per_digest": 20,
    "include_context": true
  },

  "email": {
    "provider": "smtp",
    "smtp_host": "smtp.gmail.com",
    "smtp_port": 587,
    "smtp_user": "user@gmail.com",
    "smtp_pass": "encrypted:...",
    "to_address": "user@example.com"
  }
}
```

---

## Project Structure

```
scroll4me/
├── cmd/
│   └── scroll4me/
│       └── main.go              # Application entrypoint
├── internal/
│   ├── auth/
│   │   ├── manager.go           # Login flow orchestration
│   │   └── cookies.go           # Cookie extraction & storage
│   ├── scraper/
│   │   ├── scraper.go           # chromedp scraping logic
│   │   ├── parser.go            # DOM parsing, post extraction
│   │   └── selectors.go         # X.com CSS selectors (isolated for updates)
│   ├── analyzer/
│   │   ├── analyzer.go          # LLM analysis orchestration
│   │   ├── prompt.go            # Prompt templates
│   │   └── providers/
│   │       ├── claude.go
│   │       ├── openai.go
│   │       └── ollama.go
│   ├── store/
│   │   ├── store.go             # SQLite operations
│   │   ├── migrations.go        # Schema migrations
│   │   └── models.go            # Data structures
│   ├── digest/
│   │   ├── builder.go           # Digest compilation
│   │   └── templates/
│   │       └── email.html       # Email template
│   ├── notifier/
│   │   ├── notifier.go          # Email sending
│   │   └── providers/
│   │       ├── smtp.go
│   │       └── sendgrid.go
│   ├── scheduler/
│   │   └── scheduler.go         # Cron job management
│   └── config/
│       ├── config.go            # Configuration loading/saving
│       └── defaults.go          # Default values
├── frontend/                    # Wails frontend
│   ├── src/
│   │   ├── App.svelte           # (or React/Vue/vanilla)
│   │   ├── Settings.svelte
│   │   └── ...
│   ├── index.html
│   └── package.json
├── docs/
│   ├── architecture.md          # This file
│   └── research/
│       └── x-auth-cookies.md    # Cookie research
├── build/                       # Wails build output
├── go.mod
├── go.sum
├── wails.json                   # Wails configuration
└── README.md
```

---

## Development Phases

### Phase 1: Foundation

- [ ] Initialize Go module and Wails project
- [ ] Set up project structure
- [ ] Implement config loading/saving
- [ ] Create SQLite store with schema

### Phase 2: Authentication

- [ ] Implement chromedp headful login flow
- [ ] Cookie extraction and secure storage
- [ ] Login status detection
- [ ] Basic Wails UI with "Login to X" button

### Phase 3: Scraping

- [ ] Implement chromedp headless scraper
- [ ] Parse For You feed posts
- [ ] Store posts in SQLite
- [ ] Handle scrolling and pagination

### Phase 4: Analysis

- [ ] Implement LLM analyzer (Claude API first)
- [ ] Prompt engineering for relevance scoring
- [ ] Batch processing for efficiency
- [ ] Store analysis results

### Phase 5: Digest & Notification

- [ ] Implement digest builder
- [ ] HTML email template
- [ ] SMTP notifier
- [ ] Deduplication logic

### Phase 6: Scheduling & Polish

- [ ] Implement cron scheduler
- [ ] Background execution
- [ ] Settings UI completion
- [ ] Error handling and logging

### Phase 7: Context Enrichment (Optional)

- [ ] Thread reply fetching
- [ ] Integration with digest

---

## Open Questions & Future Considerations

1. **Bot detection**: X may detect chromedp. Mitigations:

   - Realistic scroll timing
   - Random delays
   - User-agent rotation
   - Consider using logged-in user's actual browser profile

2. **API alternative**: X has a (paid) API. Could support both scraping and API for users with access.

3. **Multiple feeds**: Support Lists, Search, specific accounts beyond For You.

4. **Feedback loop**: "Thumbs up/down" on digest items to improve LLM scoring over time.

5. **Mobile**: Could send push notifications instead of email.

6. **Self-hosted LLM**: Full privacy with local Ollama/llama.cpp.
