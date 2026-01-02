package analyzer

import (
	"fmt"
	"strings"

	"github.com/ibeckermayer/scroll4me/internal/config"
	"github.com/ibeckermayer/scroll4me/internal/types"
)

// BuildPrompt constructs the LLM prompt for analyzing posts
func BuildPrompt(posts []types.Post, interests config.InterestsConfig) string {
	var sb strings.Builder

	sb.WriteString("You are analyzing social media posts for relevance to a user's interests.\n\n")

	// User interests
	sb.WriteString("## User Interests\n")
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
	sb.WriteString("3. summary (string): One sentence summary\n")
	sb.WriteString("4. needs_context (boolean): Should we fetch replies for more context?\n\n")

	sb.WriteString("Respond with a JSON array in this exact format:\n")
	sb.WriteString("```json\n")
	sb.WriteString("[\n")
	sb.WriteString("  {\n")
	sb.WriteString("    \"post_id\": \"...\",\n")
	sb.WriteString("    \"relevance_score\": 0.85,\n")
	sb.WriteString("    \"topics\": [\"AI\", \"tech\"],\n")
	sb.WriteString("    \"summary\": \"Discussion about...\",\n")
	sb.WriteString("    \"needs_context\": false\n")
	sb.WriteString("  }\n")
	sb.WriteString("]\n")
	sb.WriteString("```\n")

	return sb.String()
}
