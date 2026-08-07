package openrouter

import (
	"context"

	"github.com/hsme/core/src/core/worker"
)

type FallbackExtractor struct {
	primary  worker.GraphExtractor
	fallback worker.GraphExtractor
}

func NewFallbackExtractor(primary, fallback worker.GraphExtractor) *FallbackExtractor {
	return &FallbackExtractor{
		primary:  primary,
		fallback: fallback,
	}
}

func (f *FallbackExtractor) ExtractEntities(ctx context.Context, text string) (worker.KnowledgeGraph, error) {
	kg, err := f.primary.ExtractEntities(ctx, text)
	if err != nil {
		return f.fallback.ExtractEntities(ctx, text)
	}
	return kg, nil
}
