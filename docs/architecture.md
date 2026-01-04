# scroll4me - Architecture & Design

## Overview

scroll4me is a minimal system tray application that scrapes X (Twitter) on demand, filters content based on interests using LLM analysis, and generates a curated markdown digest.

**Goal**: Eliminate doomscrolling while staying informed.

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              scroll4me                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌────────────────────┐                                                     │
│  │   System Tray      │  ← getlantern/systray                               │
│  │   (Primary UI)     │                                                     │
│  │                    │                                                     │
│  │ - Login/Logout     │                                                     │
│  │ - Generate Digest  │                                                     │
│  │ - View Last Digest │                                                     │
│  │ - Edit Config      │                                                     │
│  └─────────┬──────────┘                                                     │
│            │                                                                 │
│            │ triggers                                                        │
│            ▼                                                                 │
│  ┌────────────────────┐         ┌────────────────────┐                     │
│  │  Auth Manager      │────────▶│  Cookie Store      │                     │
│  │  (chromedp headful)│         │  (JSON file)       │                     │
│  └────────────────────┘         └─────────┬──────────┘                     │
│                                           │                                  │
│            ┌──────────────────────────────┘                                 │
│            │                                                                 │
│            ▼                                                                 │
│  ┌────────────────────┐                                                     │
│  │     Scraper        │  ← chromedp headless                                │
│  │                    │                                                     │
│  │  - Load cookies    │                                                     │
│  │  - Navigate feed   │                                                     │
│  │  - Scroll & extract│                                                     │
│  └─────────┬──────────┘                                                     │
│            │                                                                 │
│            ▼                                                                 │
│  ┌────────────────────┐                                                     │
│  │     Analyzer       │  ← Claude API                                       │
│  │                    │                                                     │
│  │  - Score relevance │                                                     │
│  │  - Extract topics  │                                                     │
│  └─────────┬──────────┘                                                     │
│            │                                                                 │
│            ▼                                                                 │
│  ┌────────────────────┐         ┌────────────────────┐                     │
│  │   Digest Builder   │────────▶│   Markdown File    │                     │
│  │                    │         │   (local disk)     │                     │
│  │  - Filter by score │         │                    │                     │
│  │  - Format markdown │         │  ~/.config/        │                     │
│  │                    │         │  scroll4me/digests/│                     │
│  └────────────────────┘         └────────────────────┘                     │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## System Tray Menu

```
┌─────────────────────────────┐
│ ● Connected to X            │  ← Status display (disabled)
│ Logout                      │  ← Or "Login to X" if not connected
│ ─────────────────────────── │
│ Generate Digest             │  ← Main action: scrape + analyze + save
│ ─────────────────────────── │
│ View Last Digest            │  ← Opens most recent .md file
│ Edit Config                 │  ← Opens config.toml in default editor
│ Reload Config               │  ← Hot reload configuration
│ ─────────────────────────── │
│ Quit                        │
└─────────────────────────────┘
```

## Core Flow

When user clicks "Generate Digest":

1. **Scrape**: Fetch N posts from X.com For You feed
2. **Analyze**: Send posts to Claude API for relevance scoring
3. **Filter**: Keep posts above relevance threshold
4. **Build**: Generate markdown digest with all content
5. **Save**: Write to `~/.config/scroll4me/digests/YYYY-MM-DD-HHMMSS-digest.md`

## Components

### 1. System Tray (getlantern/systray)

Minimal cross-platform system tray with dropdown menu. No webview or settings window - configuration is done via TOML file.

### 2. Auth Manager

Handles X.com authentication via user-driven browser login.

**Flow**:

1. User clicks "Login to X"
2. Spawn chromedp in **headful** mode (visible Chrome window)
3. Navigate to `https://x.com/login`
4. User logs in manually (handles 2FA, CAPTCHAs, etc.)
5. Detect successful login (URL contains `/home`)
6. Extract all cookies via `network.GetAllCookies()`
7. Store cookies to JSON file
8. Close browser window

### 3. Scraper

Extracts posts from X.com using chromedp in headless mode.

**ScrapeForYou**: Scrolls the For You feed and extracts posts.

**Post structure**:

```go
type Post struct {
    ID           string
    AuthorHandle string
    AuthorName   string
    Content      string
    MediaURLs    []string
    Timestamp    time.Time
    Likes        int
    Retweets     int
    Replies      int
    IsRetweet    bool
    IsQuoteTweet bool
    IsReply      bool
    OriginalURL  string
    ScrapedAt    time.Time
}
```

### 4. Analyzer (Claude API)

Scores posts for relevance using Claude API.

**Input**: User's configured interests + batch of posts

**Output per post**:

- `relevance_score` (0.0 to 1.0)
- `topics` (up to 3 detected topics)
- `summary` (one sentence)

Posts are processed in configurable batch sizes to optimize API usage.

### 5. Digest Builder

Generates markdown files from analyzed posts.

**Features**:

- Sorts posts by relevance score
- Limits to configurable max posts
- Includes post content, summary, topics, engagement metrics
- Saves to configurable output directory

**Output format**: `YYYY-MM-DD-HHMMSS-digest.md`

---

## Configuration

Location: `~/.config/scroll4me/config.toml`

```toml
version = 1

[interests]
keywords = ["AI", "machine learning", "startups", "tech policy"]
priority_accounts = ["@elonmusk", "@sama"]
muted_accounts = ["@spambot123"]
muted_keywords = ["crypto pump", "NFT drop"]

[scraping]
posts_per_scrape = 100
headless = true

[analysis]
llm_provider = "claude"
api_key = "sk-ant-..."
model = "claude-sonnet-4-20250514"
relevance_threshold = 0.6
batch_size = 10

[digest]
output_dir = "~/.config/scroll4me/digests"
max_posts = 20
```

---

## Project Structure

```
scroll4me/
├── main.go                     # Application entrypoint + systray
├── internal/
│   ├── auth/
│   │   ├── manager.go          # Login flow orchestration
│   │   └── cookies.go          # Cookie extraction & storage
│   ├── scraper/
│   │   ├── scraper.go          # chromedp scraping logic
│   │   └── selectors.go        # X.com CSS selectors
│   ├── analyzer/
│   │   ├── analyzer.go         # LLM analysis orchestration
│   │   ├── prompt.go           # Prompt templates
│   │   └── providers/
│   │       └── claude.go       # Claude API implementation
│   ├── types/
│   │   └── types.go            # Shared data structures
│   ├── digest/
│   │   └── builder.go          # Markdown digest generation
│   └── config/
│       └── config.go           # Configuration loading/saving
├── assets/
│   └── icon.png                # System tray icon
├── docs/
│   ├── architecture.md         # This file
│   └── research/
│       └── x-auth-cookies.md   # Cookie research
├── go.mod
├── go.sum
├── Taskfile.yml
└── README.md
```

---

## Dependencies

| Package                         | Purpose                    |
| ------------------------------- | -------------------------- |
| `github.com/getlantern/systray` | Cross-platform system tray |
| `github.com/chromedp/chromedp`  | Browser automation         |
| `github.com/BurntSushi/toml`    | TOML configuration         |
| `github.com/pkg/browser`        | Open files in default app  |

---

## Future Considerations

1. **Bot detection**: X may detect chromedp. Mitigations:

   - Realistic scroll timing
   - Random delays
   - User-agent rotation

2. **API alternative**: X has a (paid) API. Could support both scraping and API.

3. **Multiple feeds**: Support Lists, Search, specific accounts beyond For You.

4. **Scheduling**: Add optional cron-like scheduling for automated digests.

5. **Email delivery**: Add optional email sending for digests.

6. **Self-hosted LLM**: Support local Ollama for privacy-conscious users.
