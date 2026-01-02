package digest

import (
	"bytes"
	"fmt"
	"html/template"
	"sort"
	"time"

	"github.com/ibeckermayer/scroll4me/internal/store"
)

// Builder creates digest emails from analyzed posts
type Builder struct {
	maxPosts int
	template *template.Template
}

// New creates a new digest builder
func New(maxPosts int) (*Builder, error) {
	tmpl, err := template.New("digest").Parse(defaultTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	return &Builder{
		maxPosts: maxPosts,
		template: tmpl,
	}, nil
}

// Digest represents a compiled digest ready for sending
type Digest struct {
	Subject   string
	HTMLBody  string
	PlainBody string
	PostIDs   []string
	CreatedAt time.Time
}

// DigestData is the template data structure
type DigestData struct {
	Title string
	Date  string
	Posts []PostData
	Stats StatsData
}

// PostData represents a post in the digest template
type PostData struct {
	AuthorHandle string
	AuthorName   string
	Content      string
	Summary      string
	Topics       []string
	Likes        int
	Retweets     int
	Replies      int
	URL          string
	Score        float64
}

// StatsData contains digest statistics
type StatsData struct {
	TotalAnalyzed int
	TotalIncluded int
}

// Build creates a digest from analyzed posts
func (b *Builder) Build(posts []store.PostWithAnalysis, digestType string) (*Digest, error) {
	if len(posts) == 0 {
		return nil, fmt.Errorf("no posts to include in digest")
	}

	// Sort by relevance score descending
	sort.Slice(posts, func(i, j int) bool {
		return posts[i].Analysis.RelevanceScore > posts[j].Analysis.RelevanceScore
	})

	// Limit to max posts
	if len(posts) > b.maxPosts {
		posts = posts[:b.maxPosts]
	}

	// Build template data
	now := time.Now()
	data := DigestData{
		Title: fmt.Sprintf("Your X Digest - %s", capitalize(digestType)),
		Date:  now.Format("Monday, January 2"),
		Posts: make([]PostData, len(posts)),
		Stats: StatsData{
			TotalIncluded: len(posts),
		},
	}

	postIDs := make([]string, len(posts))
	for i, p := range posts {
		data.Posts[i] = PostData{
			AuthorHandle: p.Post.AuthorHandle,
			AuthorName:   p.Post.AuthorName,
			Content:      truncate(p.Post.Content, 280),
			Summary:      p.Analysis.Summary,
			Topics:       p.Analysis.Topics,
			Likes:        p.Post.Likes,
			Retweets:     p.Post.Retweets,
			Replies:      p.Post.Replies,
			URL:          p.Post.OriginalURL,
			Score:        p.Analysis.RelevanceScore,
		}
		postIDs[i] = p.Post.ID
	}

	// Render HTML
	var htmlBuf bytes.Buffer
	if err := b.template.Execute(&htmlBuf, data); err != nil {
		return nil, fmt.Errorf("failed to render template: %w", err)
	}

	return &Digest{
		Subject:   fmt.Sprintf("X Digest - %s, %s", capitalize(digestType), now.Format("Jan 2")),
		HTMLBody:  htmlBuf.String(),
		PlainBody: buildPlainText(data),
		PostIDs:   postIDs,
		CreatedAt: now,
	}, nil
}

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return string(s[0]-32) + s[1:]
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func buildPlainText(data DigestData) string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("%s\n%s\n\n", data.Title, data.Date))

	for i, p := range data.Posts {
		buf.WriteString(fmt.Sprintf("%d. @%s: %s\n", i+1, p.AuthorHandle, p.Summary))
		buf.WriteString(fmt.Sprintf("   %s\n\n", p.URL))
	}

	return buf.String()
}

const defaultTemplate = `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>{{.Title}}</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px; background: #f5f5f5; }
        .container { background: white; border-radius: 8px; padding: 20px; }
        h1 { color: #1da1f2; margin-bottom: 5px; }
        .date { color: #666; margin-bottom: 20px; }
        .post { border-bottom: 1px solid #eee; padding: 15px 0; }
        .post:last-child { border-bottom: none; }
        .author { font-weight: bold; color: #333; }
        .handle { color: #666; }
        .content { margin: 10px 0; line-height: 1.4; }
        .summary { color: #1da1f2; font-style: italic; margin: 8px 0; }
        .topics { margin: 5px 0; }
        .topic { background: #e8f5fd; color: #1da1f2; padding: 2px 8px; border-radius: 12px; font-size: 12px; margin-right: 5px; }
        .metrics { color: #666; font-size: 13px; }
        .link { color: #1da1f2; text-decoration: none; }
        .footer { margin-top: 20px; padding-top: 15px; border-top: 1px solid #eee; color: #999; font-size: 12px; text-align: center; }
    </style>
</head>
<body>
    <div class="container">
        <h1>{{.Title}}</h1>
        <div class="date">{{.Date}}</div>

        {{range .Posts}}
        <div class="post">
            <div class="author">{{.AuthorName}} <span class="handle">@{{.AuthorHandle}}</span></div>
            <div class="content">{{.Content}}</div>
            <div class="summary">{{.Summary}}</div>
            <div class="topics">
                {{range .Topics}}<span class="topic">{{.}}</span>{{end}}
            </div>
            <div class="metrics">{{.Likes}} likes · {{.Retweets}} retweets · {{.Replies}} replies</div>
            <a href="{{.URL}}" class="link">View on X →</a>
        </div>
        {{end}}

        <div class="footer">
            Included {{.Stats.TotalIncluded}} posts · Generated by scroll4me
        </div>
    </div>
</body>
</html>`
