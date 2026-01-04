package providers

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ibeckermayer/scroll4me/internal/config"
	"github.com/ibeckermayer/scroll4me/internal/types"
)

// AnalysisResult represents the expected JSON structure from any LLM provider
type AnalysisResult struct {
	PostID         string   `json:"post_id"`
	RelevanceScore float64  `json:"relevance_score"`
	Topics         []string `json:"topics"`
	Summary        string   `json:"summary"`
	NeedsContext   bool     `json:"needs_context"`
}

// ParseAnalysisResponse parses raw JSON bytes from an LLM provider into Analysis objects.
// Each provider is responsible for assembling the complete JSON before calling this.
func ParseAnalysisResponse(jsonBytes []byte) ([]types.Analysis, error) {
	var results []AnalysisResult
	if err := json.Unmarshal(jsonBytes, &results); err != nil {
		return nil, fmt.Errorf("failed to parse analysis JSON: %w (response was: %.500s)", err, string(jsonBytes))
	}

	now := time.Now()
	analyses := make([]types.Analysis, len(results))
	for i, r := range results {
		analyses[i] = types.Analysis{
			PostID:         r.PostID,
			RelevanceScore: r.RelevanceScore,
			Topics:         r.Topics,
			Summary:        r.Summary,
			NeedsContext:   r.NeedsContext,
			AnalyzedAt:     now,
		}
	}

	return analyses, nil
}

// buildPrompt constructs the LLM prompt for analyzing posts
func buildPrompt(posts []types.Post, interests config.InterestsConfig) string {
	var sb strings.Builder

	sb.WriteString("You are analyzing social media posts for relevance to a user's interests.\n\n")

	// User interests
	sb.WriteString("## User Interests\n")
	hasInterests := len(interests.Keywords) > 0 || len(interests.PriorityAccounts) > 0 ||
		len(interests.MutedKeywords) > 0 || len(interests.MutedAccounts) > 0

	if !hasInterests {
		sb.WriteString("No specific interests configured. Score posts based on general quality, informativeness, and newsworthiness.\n")
	} else {
		if len(interests.Keywords) > 0 {
			sb.WriteString(fmt.Sprintf("Keywords: %s\n", strings.Join(interests.Keywords, ", ")))
		}
		if len(interests.PriorityAccounts) > 0 {
			sb.WriteString(fmt.Sprintf("Priority accounts: %s\n", strings.Join(interests.PriorityAccounts, ", ")))
		}
		if len(interests.MutedKeywords) > 0 {
			sb.WriteString(fmt.Sprintf("Muted keywords (score 0): %s\n", strings.Join(interests.MutedKeywords, ", ")))
		}
		if len(interests.MutedAccounts) > 0 {
			sb.WriteString(fmt.Sprintf("Muted accounts (score 0): %s\n", strings.Join(interests.MutedAccounts, ", ")))
		}
	}

	sb.WriteString("\n## Posts to Analyze\n\n")

	// Posts
	for i, p := range posts {
		sb.WriteString(fmt.Sprintf("### Post %d (ID: %s)\n", i+1, p.ID))
		sb.WriteString(fmt.Sprintf("Author: @%s (%s)\n", p.AuthorHandle, p.AuthorName))
		sb.WriteString(fmt.Sprintf("Content: %s\n", p.Content))
		sb.WriteString(fmt.Sprintf("Engagement: %d likes, %d retweets, %d replies\n", p.Likes, p.Retweets, p.Replies))
		if p.IsRetweet {
			sb.WriteString("Type: Retweet\n")
		}
		if p.IsQuoteTweet {
			sb.WriteString("Type: Quote Tweet\n")
		}
		sb.WriteString("\n")
	}

	// Instructions
	sb.WriteString("## Task\n\n")
	sb.WriteString("For each post, provide:\n")
	sb.WriteString("1. relevance_score (0.0 to 1.0): How relevant is this to the user's interests?\n")
	sb.WriteString("2. topics (array, max 3): Key topics detected\n")
	sb.WriteString("3. summary (string): One sentence summary\n")
	sb.WriteString("4. needs_context (boolean): Should we fetch replies for more context?\n\n")

	sb.WriteString("IMPORTANT: Respond with ONLY a valid JSON array. No markdown, no code blocks, no explanation - just the raw JSON starting with [ and ending with ].\n\n")
	sb.WriteString("Example structure:\n")
	sb.WriteString(`[{"post_id": "...", "relevance_score": 0.85, "topics": ["AI", "tech"], "summary": "Discussion about...", "needs_context": false}]`)
	sb.WriteString("\n")

	return sb.String()
}
