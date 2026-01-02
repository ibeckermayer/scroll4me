package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/ibeckermayer/scroll4me/internal/analyzer"
	"github.com/ibeckermayer/scroll4me/internal/config"
	"github.com/ibeckermayer/scroll4me/internal/types"
)

const (
	claudeAPIURL     = "https://api.anthropic.com/v1/messages"
	anthropicVersion = "2023-06-01"
)

// ClaudeProvider implements the Provider interface using Claude API
type ClaudeProvider struct {
	apiKey string
	model  string
	client *http.Client
}

// NewClaudeProvider creates a new Claude provider
func NewClaudeProvider(apiKey, model string) *ClaudeProvider {
	return &ClaudeProvider{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{
			Timeout: 120 * time.Second, // LLM calls can be slow
		},
	}
}

// claudeRequest represents the request body for Claude API
type claudeRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	Messages  []claudeMessage `json:"messages"`
}

// claudeMessage represents a message in the Claude conversation
type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// claudeResponse represents the response from Claude API
type claudeResponse struct {
	Content []claudeContent `json:"content"`
	Error   *claudeError    `json:"error,omitempty"`
}

type claudeContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type claudeError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
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

	// Build request
	reqBody := claudeRequest{
		Model:     c.model,
		MaxTokens: 4096,
		Messages: []claudeMessage{
			{Role: "user", Content: prompt},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", claudeAPIURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)

	// Execute request
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call Claude API: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check for HTTP errors
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Claude API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var claudeResp claudeResponse
	if err := json.Unmarshal(body, &claudeResp); err != nil {
		return nil, fmt.Errorf("failed to parse Claude response: %w", err)
	}

	if claudeResp.Error != nil {
		return nil, fmt.Errorf("Claude API error: %s - %s", claudeResp.Error.Type, claudeResp.Error.Message)
	}

	if len(claudeResp.Content) == 0 {
		return nil, fmt.Errorf("Claude returned empty response")
	}

	// Extract the text content
	responseText := claudeResp.Content[0].Text

	// Parse the JSON from Claude's response
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
