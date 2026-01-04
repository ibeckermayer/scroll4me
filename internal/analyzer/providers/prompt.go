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
			AnalyzedAt:     now,
		}
	}

	return analyses, nil
}

// buildPrompt constructs the LLM prompt for analyzing posts
func buildPrompt(posts []types.Post, interests config.InterestsConfig) string {
	var sb strings.Builder

	sb.WriteString("You are analyzing social media posts for relevance to a user's interests.\n\n")

	// Analysis guidelines
	sb.WriteString("## Analysis Guidelines\n")

	// Custom instructions (or fallback if empty)
	if interests.CustomInstructions != "" {
		sb.WriteString(interests.CustomInstructions + "\n")
	} else {
		sb.WriteString("User specified no particular interests.\n")
	}

	// Specific interests if configured
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
	sb.WriteString("3. summary (string): One sentence summary\n\n")

	sb.WriteString("IMPORTANT: Respond with ONLY a valid JSON array. No markdown, no code blocks, no explanation - just the raw JSON starting with [ and ending with ].\n\n")
	sb.WriteString("Example structure:\n")
	sb.WriteString(`[{"post_id": "...", "relevance_score": 0.85, "topics": ["AI", "tech"], "summary": "Discussion about..."}]`)
	sb.WriteString("\n")

	return sb.String()
}
