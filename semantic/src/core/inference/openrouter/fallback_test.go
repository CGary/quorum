package openrouter

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hsme/core/src/core/worker"
)

type recordExtractor struct {
	model       string
	kg          worker.KnowledgeGraph
	err         error
	calls       int
	deadlines   []time.Time
	hasDeadline []bool
}

func (r *recordExtractor) ExtractEntities(ctx context.Context, text string) (worker.KnowledgeGraph, error) {
	r.calls++
	if d, ok := ctx.Deadline(); ok {
		r.deadlines = append(r.deadlines, d)
		r.hasDeadline = append(r.hasDeadline, true)
	} else {
		r.deadlines = append(r.deadlines, time.Time{})
		r.hasDeadline = append(r.hasDeadline, false)
	}
	return r.kg, r.err
}

// AC-1 & AC-2: Strictly sequential execution in order (short -> long -> ollama)
// with tier 2 reusing the exact same cloud model, only timeout differing.
func TestChainExtractor_ThreeTiersOrderAndDeadlines(t *testing.T) {
	cloudModel := "nvidia/nemotron-nano-9b-v2:free"
	cloudExtractor := &recordExtractor{
		model: cloudModel,
		err:   errors.New("openrouter gateway timeout"),
	}

	ollamaModel := "phi3.5"
	ollamaExtractor := &recordExtractor{
		model: ollamaModel,
		kg: worker.KnowledgeGraph{
			Nodes: []worker.Node{{Type: "TECH", Name: "Ollama"}},
		},
	}

	shortTimeout := 100 * time.Millisecond
	longTimeout := 500 * time.Millisecond

	chain := NewChainExtractor(
		Tier{
			Name:      "openrouter-short",
			Extractor: cloudExtractor,
			Timeout:   shortTimeout,
		},
		Tier{
			Name:      "openrouter-long",
			Extractor: cloudExtractor,
			Timeout:   longTimeout,
		},
		Tier{
			Name:      "ollama-fallback",
			Extractor: ollamaExtractor,
			Timeout:   0,
		},
	)

	before := time.Now()
	kg, err := chain.ExtractEntities(context.Background(), "sample text")
	if err != nil {
		t.Fatalf("expected success on tier 3, got error: %v", err)
	}

	if len(kg.Nodes) != 1 || kg.Nodes[0].Name != "Ollama" {
		t.Errorf("expected ollama result, got %+v", kg.Nodes)
	}

	// AC-1: OpenRouter called twice (short then long), then Ollama once.
	if cloudExtractor.calls != 2 {
		t.Errorf("expected 2 cloud extractor calls, got %d", cloudExtractor.calls)
	}
	if ollamaExtractor.calls != 1 {
		t.Errorf("expected 1 ollama extractor call, got %d", ollamaExtractor.calls)
	}

	// AC-2: Tier 2 reuses the same cloud model.
	if cloudExtractor.model != cloudModel {
		t.Errorf("expected cloud model %s, got %s", cloudModel, cloudExtractor.model)
	}

	// Deadlines check: tier 1 had short timeout, tier 2 had long timeout, tier 3 had no extra deadline.
	if len(cloudExtractor.deadlines) != 2 {
		t.Fatalf("expected 2 cloud deadlines recorded, got %d", len(cloudExtractor.deadlines))
	}
	d1 := cloudExtractor.deadlines[0].Sub(before)
	d2 := cloudExtractor.deadlines[1].Sub(before)
	if d1 < 50*time.Millisecond || d1 > 300*time.Millisecond {
		t.Errorf("unexpected short deadline delta: %v", d1)
	}
	if d2 < 400*time.Millisecond || d2 > 800*time.Millisecond {
		t.Errorf("unexpected long deadline delta: %v", d2)
	}
	if !cloudExtractor.deadlines[1].After(cloudExtractor.deadlines[0]) {
		t.Errorf("expected tier 2 deadline (%v) to be after tier 1 deadline (%v)", cloudExtractor.deadlines[1], cloudExtractor.deadlines[0])
	}
	if ollamaExtractor.hasDeadline[0] {
		t.Errorf("expected ollama tier to have no extra context deadline, got %v", ollamaExtractor.deadlines[0])
	}
}

// AC-3: Tier 3 regression test - Ollama receives its own model name, never OpenRouter model name.
func TestChainExtractor_Tier3ReceivesOllamaModelName(t *testing.T) {
	cloudModel := "nvidia/nemotron-nano-9b-v2:free"
	cloudExtractor := &recordExtractor{
		model: cloudModel,
		err:   errors.New("openrouter 504"),
	}

	ollamaModel := "phi3.5"
	ollamaExtractor := &recordExtractor{
		model: ollamaModel,
		kg: worker.KnowledgeGraph{
			Nodes: []worker.Node{{Type: "TECH", Name: "OllamaCorrectModel"}},
		},
	}

	chain := NewChainExtractor(
		Tier{Name: "openrouter-short", Extractor: cloudExtractor, Timeout: 50 * time.Millisecond},
		Tier{Name: "openrouter-long", Extractor: cloudExtractor, Timeout: 100 * time.Millisecond},
		Tier{Name: "ollama-fallback", Extractor: ollamaExtractor, Timeout: 0},
	)

	kg, err := chain.ExtractEntities(context.Background(), "text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(kg.Nodes) != 1 || kg.Nodes[0].Name != "OllamaCorrectModel" {
		t.Errorf("unexpected kg: %+v", kg)
	}

	if ollamaExtractor.model != "phi3.5" {
		t.Errorf("expected ollama model to be phi3.5, got %s", ollamaExtractor.model)
	}
	if ollamaExtractor.model == cloudModel {
		t.Errorf("regression: ollama extractor was configured with cloud model name %s", cloudModel)
	}
}

// AC-4: Short-circuiting - when tier 1 succeeds, tiers 2 and 3 are never called.
func TestChainExtractor_ShortCircuit_Tier1Success(t *testing.T) {
	tier1 := &recordExtractor{
		model: "tier1-model",
		kg: worker.KnowledgeGraph{
			Nodes: []worker.Node{{Type: "TECH", Name: "Tier1"}},
		},
	}
	tier2 := &recordExtractor{model: "tier2-model", err: errors.New("should not be called")}
	tier3 := &recordExtractor{model: "tier3-model", err: errors.New("should not be called")}

	chain := NewChainExtractor(
		Tier{Name: "tier-1", Extractor: tier1, Timeout: 100 * time.Millisecond},
		Tier{Name: "tier-2", Extractor: tier2, Timeout: 200 * time.Millisecond},
		Tier{Name: "tier-3", Extractor: tier3, Timeout: 0},
	)

	kg, err := chain.ExtractEntities(context.Background(), "text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(kg.Nodes) != 1 || kg.Nodes[0].Name != "Tier1" {
		t.Errorf("expected tier 1 result, got %+v", kg.Nodes)
	}
	if tier1.calls != 1 {
		t.Errorf("expected 1 tier1 call, got %d", tier1.calls)
	}
	if tier2.calls != 0 {
		t.Errorf("expected 0 tier2 calls, got %d", tier2.calls)
	}
	if tier3.calls != 0 {
		t.Errorf("expected 0 tier3 calls, got %d", tier3.calls)
	}
}

// AC-4: Short-circuiting - when tier 1 fails and tier 2 succeeds, tier 3 is never called.
func TestChainExtractor_ShortCircuit_Tier2Success(t *testing.T) {
	tier1 := &recordExtractor{model: "tier1-model", err: errors.New("tier1 timeout")}
	tier2 := &recordExtractor{
		model: "tier2-model",
		kg: worker.KnowledgeGraph{
			Nodes: []worker.Node{{Type: "TECH", Name: "Tier2"}},
		},
	}
	tier3 := &recordExtractor{model: "tier3-model", err: errors.New("should not be called")}

	chain := NewChainExtractor(
		Tier{Name: "tier-1", Extractor: tier1, Timeout: 100 * time.Millisecond},
		Tier{Name: "tier-2", Extractor: tier2, Timeout: 200 * time.Millisecond},
		Tier{Name: "tier-3", Extractor: tier3, Timeout: 0},
	)

	kg, err := chain.ExtractEntities(context.Background(), "text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(kg.Nodes) != 1 || kg.Nodes[0].Name != "Tier2" {
		t.Errorf("expected tier 2 result, got %+v", kg.Nodes)
	}
	if tier1.calls != 1 {
		t.Errorf("expected 1 tier1 call, got %d", tier1.calls)
	}
	if tier2.calls != 1 {
		t.Errorf("expected 1 tier2 call, got %d", tier2.calls)
	}
	if tier3.calls != 0 {
		t.Errorf("expected 0 tier3 calls, got %d", tier3.calls)
	}
}

// AC-5: Total failure produces a terminal error preserving worker re-queue contract.
func TestChainExtractor_AllTiersFailTerminalError(t *testing.T) {
	tier1 := &recordExtractor{model: "tier1", err: errors.New("err1")}
	tier2 := &recordExtractor{model: "tier2", err: errors.New("err2")}
	tier3 := &recordExtractor{model: "tier3", err: errors.New("err3")}

	chain := NewChainExtractor(
		Tier{Name: "tier-1", Extractor: tier1},
		Tier{Name: "tier-2", Extractor: tier2},
		Tier{Name: "tier-3", Extractor: tier3},
	)

	_, err := chain.ExtractEntities(context.Background(), "text")
	if err == nil {
		t.Fatal("expected terminal error when all tiers fail, got nil")
	}

	if !strings.Contains(err.Error(), "all 3 tiers failed") {
		t.Errorf("expected error message mentioning all 3 tiers failed, got %v", err)
	}
	if !strings.Contains(err.Error(), "err3") {
		t.Errorf("expected error message to wrap/include last error, got %v", err)
	}

	if tier1.calls != 1 || tier2.calls != 1 || tier3.calls != 1 {
		t.Errorf("expected each tier called once: t1=%d, t2=%d, t3=%d", tier1.calls, tier2.calls, tier3.calls)
	}
}

func TestChainExtractor_NoTiers(t *testing.T) {
	chain := NewChainExtractor()
	_, err := chain.ExtractEntities(context.Background(), "text")
	if err == nil {
		t.Fatal("expected error on empty chain, got nil")
	}
}

func TestFallbackExtractor_LegacyCompat(t *testing.T) {
	primary := &recordExtractor{
		kg: worker.KnowledgeGraph{
			Nodes: []worker.Node{{Type: "TECH", Name: "Go"}},
		},
	}
	fallback := &recordExtractor{
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
