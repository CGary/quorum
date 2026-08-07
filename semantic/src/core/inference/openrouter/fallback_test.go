package openrouter

import (
	"context"
	"errors"
	"testing"

	"github.com/hsme/core/src/core/worker"
)

type stubExtractor struct {
	kg    worker.KnowledgeGraph
	err   error
	calls int
}

func (s *stubExtractor) ExtractEntities(ctx context.Context, text string) (worker.KnowledgeGraph, error) {
	s.calls++
	return s.kg, s.err
}

func TestFallbackExtractor_PrimarySuccess(t *testing.T) {
	primary := &stubExtractor{
		kg: worker.KnowledgeGraph{
			Nodes: []worker.Node{{Type: "TECH", Name: "Go"}},
		},
	}
	fallback := &stubExtractor{
		kg: worker.KnowledgeGraph{
			Nodes: []worker.Node{{Type: "TECH", Name: "Python"}},
		},
	}

	fe := NewFallbackExtractor(primary, fallback)
	kg, err := fe.ExtractEntities(context.Background(), "text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(kg.Nodes) != 1 || kg.Nodes[0].Name != "Go" {
		t.Errorf("expected primary result (Go), got %+v", kg.Nodes)
	}
	if primary.calls != 1 {
		t.Errorf("expected 1 primary call, got %d", primary.calls)
	}
	if fallback.calls != 0 {
		t.Errorf("expected 0 fallback calls, got %d", fallback.calls)
	}
}

func TestFallbackExtractor_PrimaryFails_FallbackSucceeds(t *testing.T) {
	primary := &stubExtractor{
		err: errors.New("openrouter connection failed"),
	}
	fallback := &stubExtractor{
		kg: worker.KnowledgeGraph{
			Nodes: []worker.Node{{Type: "TECH", Name: "Ollama"}},
		},
	}

	fe := NewFallbackExtractor(primary, fallback)
	kg, err := fe.ExtractEntities(context.Background(), "text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(kg.Nodes) != 1 || kg.Nodes[0].Name != "Ollama" {
		t.Errorf("expected fallback result (Ollama), got %+v", kg.Nodes)
	}
	if primary.calls != 1 {
		t.Errorf("expected 1 primary call, got %d", primary.calls)
	}
	if fallback.calls != 1 {
		t.Errorf("expected 1 fallback call, got %d", fallback.calls)
	}
}
