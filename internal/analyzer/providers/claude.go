package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/ibeckermayer/scroll4me/internal/analyzer"
	"github.com/ibeckermayer/scroll4me/internal/config"
	"github.com/ibeckermayer/scroll4me/internal/types"
)

// ClaudeProvider implements the Provider interface using Claude API
type ClaudeProvider struct {
	client *anthropic.Client
	model  string
}

// NewClaudeProvider creates a new Claude provider
func NewClaudeProvider(apiKey, model string) *ClaudeProvider {
	client := anthropic.NewClient(
		option.WithAPIKey(apiKey),
	)
	return &ClaudeProvider{
		client: &client,
		model:  model,
	}
}

// analysisResponse represents the expected JSON response from Claude
type analysisResponse struct {
	PostID         string   `json:"post_id"`
	RelevanceScore float64  `json:"relevance_score"`
	Topics         []string `json:"topics"`
	Summary        string   `json:"summary"`
	NeedsContext   bool     `json:"needs_context"`
}

// Analyze sends posts to Claude for relevance analysis
func (c *ClaudeProvider) Analyze(ctx context.Context, posts []types.Post, interests config.InterestsConfig) ([]types.Analysis, error) {
	prompt := analyzer.BuildPrompt(posts, interests)

	message, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 4096,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to call Claude API: %w", err)
	}

	// Extract text from response
	var responseText string
	for _, block := range message.Content {
		if block.Type == "text" {
			responseText = block.Text
			break
		}
	}

	if responseText == "" {
		return nil, fmt.Errorf("Claude returned empty response")
	}

	return parseResponse(responseText)
}

// parseResponse extracts analyses from Claude's JSON response
func parseResponse(responseText string) ([]types.Analysis, error) {
	// Claude may wrap the JSON in markdown code blocks, so we need to extract it
	jsonText := extractJSON(responseText)

	var responses []analysisResponse
	if err := json.Unmarshal([]byte(jsonText), &responses); err != nil {
		return nil, fmt.Errorf("failed to parse response JSON: %w (response was: %s)", err, responseText)
	}

	now := time.Now()
	analyses := make([]types.Analysis, len(responses))
	for i, r := range responses {
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

// extractJSON attempts to extract JSON from Claude's response, handling markdown code blocks
func extractJSON(text string) string {
	// Try to extract JSON from markdown code block
	re := regexp.MustCompile(`(?s)` + "```" + `(?:json)?\s*\n?(\[.*?\])\s*\n?` + "```")
	matches := re.FindStringSubmatch(text)
	if len(matches) > 1 {
		return matches[1]
	}

	// If no code block, try to find raw JSON array
	re = regexp.MustCompile(`(?s)(\[.*\])`)
	matches = re.FindStringSubmatch(text)
	if len(matches) > 1 {
		return matches[1]
	}

	// Return original text as fallback
	return text
}
