package openrouter

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hsme/core/src/core/worker"
)

// Tier represents a single extraction tier in the chain.
type Tier struct {
	Name      string
	Extractor worker.GraphExtractor
	Timeout   time.Duration
}

// ChainExtractor evaluates extraction tiers sequentially until one succeeds.
type ChainExtractor struct {
	tiers []Tier
}

// NewChainExtractor creates a new multi-tier extraction chain.
func NewChainExtractor(tiers ...Tier) *ChainExtractor {
	return &ChainExtractor{
		tiers: tiers,
	}
}

// Tiers returns a copy of the configured extraction tiers.
func (c *ChainExtractor) Tiers() []Tier {
	result := make([]Tier, len(c.tiers))
	copy(result, c.tiers)
	return result
}

// FallbackExtractor is retained for backward compatibility with 2-tier callers.
type FallbackExtractor = ChainExtractor

// NewFallbackExtractor creates a 2-tier ChainExtractor for backward compatibility.
func NewFallbackExtractor(primary, fallback worker.GraphExtractor) *ChainExtractor {
	return NewChainExtractor(
		Tier{Name: "primary", Extractor: primary},
		Tier{Name: "fallback", Extractor: fallback},
	)
}

// ExtractEntities executes tiers in order until one succeeds or all fail.
func (c *ChainExtractor) ExtractEntities(ctx context.Context, text string) (worker.KnowledgeGraph, error) {
	var lastErr error
	for i, tier := range c.tiers {
		tierCtx := ctx
		var cancel context.CancelFunc
		if tier.Timeout > 0 {
			tierCtx, cancel = context.WithTimeout(ctx, tier.Timeout)
		}

		kg, err := tier.Extractor.ExtractEntities(tierCtx, text)
		if cancel != nil {
			cancel()
		}

		if err == nil {
			return kg, nil
		}

		name := tier.Name
		if name == "" {
			name = fmt.Sprintf("tier-%d", i+1)
		}
		log.Printf("openrouter extraction chain: %s failed: %v", name, err)
		lastErr = err
	}

	if lastErr == nil {
		return worker.KnowledgeGraph{}, fmt.Errorf("extraction chain: no tiers configured")
	}
	return worker.KnowledgeGraph{}, fmt.Errorf("extraction chain: all %d tiers failed, last error: %w", len(c.tiers), lastErr)
}
