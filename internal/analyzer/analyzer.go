package analyzer

import (
	"context"
	"fmt"

	"github.com/ibeckermayer/scroll4me/internal/config"
	"github.com/ibeckermayer/scroll4me/internal/store"
)

// Provider defines the interface for LLM providers
type Provider interface {
	Analyze(ctx context.Context, posts []store.Post, interests config.InterestsConfig) ([]store.Analysis, error)
}

// Analyzer handles LLM-based post analysis
type Analyzer struct {
	provider  Provider
	interests config.InterestsConfig
	batchSize int
}

// New creates a new analyzer with the given provider
func New(provider Provider, interests config.InterestsConfig, batchSize int) *Analyzer {
	return &Analyzer{
		provider:  provider,
		interests: interests,
		batchSize: batchSize,
	}
}

// AnalyzePosts processes posts through the LLM for relevance scoring
func (a *Analyzer) AnalyzePosts(ctx context.Context, posts []store.Post) ([]store.Analysis, error) {
	if len(posts) == 0 {
		return nil, nil
	}

	var allAnalyses []store.Analysis

	// Process in batches
	for i := 0; i < len(posts); i += a.batchSize {
		end := i + a.batchSize
		if end > len(posts) {
			end = len(posts)
		}

		batch := posts[i:end]
		analyses, err := a.provider.Analyze(ctx, batch, a.interests)
		if err != nil {
			return nil, fmt.Errorf("failed to analyze batch %d: %w", i/a.batchSize, err)
		}

		allAnalyses = append(allAnalyses, analyses...)
	}

	return allAnalyses, nil
}
