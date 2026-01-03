package providers

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/ibeckermayer/scroll4me/internal/config"
	"github.com/ibeckermayer/scroll4me/internal/store"
	"github.com/ibeckermayer/scroll4me/internal/types"
)

// AnthropicProvider implements the Provider interface using Anthropic's Claude API
type AnthropicProvider struct {
	client   *anthropic.Client
	provider string // e.g. "anthropic"
	model    string
}

// NewAnthropicProvider creates a new Anthropic provider
func NewAnthropicProvider(apiKey, model string) *AnthropicProvider {
	client := anthropic.NewClient(
		option.WithAPIKey(apiKey),
	)
	return &AnthropicProvider{
		client:   &client,
		provider: config.ProviderAnthropic,
		model:    model,
	}
}

// Analyze sends posts to Claude for relevance analysis
func (c *AnthropicProvider) Analyze(ctx context.Context, posts []types.Post, interests config.InterestsConfig) ([]types.Analysis, error) {
	prompt := buildPrompt(posts, interests)

	// Use prefilling to ensure Claude continues with valid JSON (starting after the "[")
	message, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 4096,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
			anthropic.NewAssistantMessage(anthropic.NewTextBlock("[")),
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

	// Cache the prompt/response for debugging
	if cachePath, err := store.SaveLLMExchange(store.LLMExchange{
		Timestamp: time.Now(),
		Provider:  c.provider,
		Model:     c.model,
		Prompt:    prompt,
		Response:  responseText,
	}); err != nil {
		log.Printf("Failed to cache LLM exchange: %v", err)
	} else {
		log.Printf("Cached LLM exchange to: %s", cachePath)
	}

	if responseText == "" {
		return nil, fmt.Errorf("Claude returned empty response")
	}

	// Prepend "[" since we used prefilling - the response continues from after the "["
	fullJSON := "[" + responseText
	return ParseAnalysisResponse([]byte(fullJSON))
}
