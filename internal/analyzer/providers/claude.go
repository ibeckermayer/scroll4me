package providers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ibeckermayer/scroll4me/internal/analyzer"
	"github.com/ibeckermayer/scroll4me/internal/config"
	"github.com/ibeckermayer/scroll4me/internal/store"
)

// ClaudeProvider implements the Provider interface using Claude API
type ClaudeProvider struct {
	apiKey string
	model  string
}

// NewClaudeProvider creates a new Claude provider
func NewClaudeProvider(apiKey, model string) *ClaudeProvider {
	return &ClaudeProvider{
		apiKey: apiKey,
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
func (c *ClaudeProvider) Analyze(ctx context.Context, posts []store.Post, interests config.InterestsConfig) ([]store.Analysis, error) {
	prompt := analyzer.BuildPrompt(posts, interests)

	// TODO: Implement actual Claude API call
	// For now, return a placeholder
	//
	// Implementation will use:
	// - POST to https://api.anthropic.com/v1/messages
	// - Headers: x-api-key, anthropic-version, content-type
	// - Body: model, max_tokens, messages[{role: "user", content: prompt}]

	_ = prompt // Use prompt when implementing

	return nil, fmt.Errorf("Claude provider not yet implemented")
}

// parseResponse extracts analyses from Claude's JSON response
func parseResponse(responseJSON string, posts []store.Post) ([]store.Analysis, error) {
	var responses []analysisResponse
	if err := json.Unmarshal([]byte(responseJSON), &responses); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	analyses := make([]store.Analysis, len(responses))
	for i, r := range responses {
		analyses[i] = store.Analysis{
			PostID:         r.PostID,
			RelevanceScore: r.RelevanceScore,
			Topics:         r.Topics,
			Summary:        r.Summary,
			NeedsContext:   r.NeedsContext,
		}
	}

	return analyses, nil
}
