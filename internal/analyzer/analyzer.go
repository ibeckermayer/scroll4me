package analyzer

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"

	"github.com/ibeckermayer/scroll4me/internal/analyzer/providers"
	"github.com/ibeckermayer/scroll4me/internal/config"
	"github.com/ibeckermayer/scroll4me/internal/types"
)

// Provider defines the interface for LLM providers
type Provider interface {
	Analyze(ctx context.Context, posts []types.Post, interests config.InterestsConfig) ([]types.Analysis, error)
}

// Analyzer handles LLM-based post analysis
type Analyzer struct {
	provider  Provider
	interests config.InterestsConfig
	batchSize int
}

// New creates a new analyzer with the appropriate provider based on config
func New(analysisConfig config.AnalysisConfig, interests config.InterestsConfig) (*Analyzer, error) {
	var provider Provider

	switch analysisConfig.LLMProvider {
	case config.ProviderAnthropic:
		provider = providers.NewAnthropicProvider(analysisConfig.APIKey, analysisConfig.Model)
	// case config.ProviderOpenAI:
	// 	provider = providers.NewOpenAIProvider(analysisConfig.APIKey, analysisConfig.Model)
	default:
		return nil, fmt.Errorf("unknown LLM provider: %s", analysisConfig.LLMProvider)
	}

	return &Analyzer{
		provider:  provider,
		interests: interests,
		batchSize: analysisConfig.BatchSize,
	}, nil
}

// AnalyzePosts processes posts through the LLM for relevance scoring
func (a *Analyzer) AnalyzePosts(ctx context.Context, posts []types.Post) ([]types.Analysis, error) {
	if len(posts) == 0 {
		return nil, nil
	}

	// Calculate number of batches
	numBatches := (len(posts) + a.batchSize - 1) / a.batchSize

	// Pre-allocate results slice (one slice per batch)
	results := make([][]types.Analysis, numBatches)

	g, ctx := errgroup.WithContext(ctx)

	// Process batches concurrently
	for i := 0; i < len(posts); i += a.batchSize {
		batchIdx := i / a.batchSize
		start := i
		end := min(i+a.batchSize, len(posts))
		batch := posts[start:end]

		g.Go(func() error {
			analyses, err := a.provider.Analyze(ctx, batch, a.interests)
			if err != nil {
				return fmt.Errorf("failed to analyze batch %d: %w", batchIdx, err)
			}
			results[batchIdx] = analyses
			return nil
		})
	}

	// Wait for all goroutines and check for errors
	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Flatten results in order
	var allAnalyses []types.Analysis
	for _, batchResult := range results {
		allAnalyses = append(allAnalyses, batchResult...)
	}

	return allAnalyses, nil
}
