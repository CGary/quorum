package core

import (
	"reflect"
	"testing"
)

// verbatim agy rejection fixture observed live 2026-09-03 (blueprint risk note).
// The header line has no leading spaces; each model line is prefixed with two
// spaces. ParseAvailableModels must handle any leading whitespace.
const agyRejectionFixture = `Error: unknown model "quorum-catalog-probe-nonexistent"

Available models:
  Gemini 3.8 Flash (High)
  Gemini 3.8 Flash (Medium)
  Gemini 3.8 Flash (Low)
  Gemini 3.7 Flash (High)
  Gemini 3.7 Flash (Medium)
  Gemini 3.7 Flash (Low)
  Gemini 3.6 Flash (High)
  Gemini 3.6 Flash (Medium)
  Gemini 3.6 Flash (Low)
  Gemini 3.1 Pro (High)
  Gemini 3.1 Pro (Low)
  Claude Sonnet 4.6 (Thinking)
  Claude Opus 4.6 (Thinking)
  GPT-OSS 120B (Medium)
`

// TestParseAvailableModels covers AC-1.
func TestParseAvailableModels(t *testing.T) {
	t.Run("extracts 14 display names from verbatim agy rejection text", func(t *testing.T) {
		want := []string{
			"Gemini 3.8 Flash (High)",
			"Gemini 3.8 Flash (Medium)",
			"Gemini 3.8 Flash (Low)",
			"Gemini 3.7 Flash (High)",
			"Gemini 3.7 Flash (Medium)",
			"Gemini 3.7 Flash (Low)",
			"Gemini 3.6 Flash (High)",
			"Gemini 3.6 Flash (Medium)",
			"Gemini 3.6 Flash (Low)",
			"Gemini 3.1 Pro (High)",
			"Gemini 3.1 Pro (Low)",
			"Claude Sonnet 4.6 (Thinking)",
			"Claude Opus 4.6 (Thinking)",
			"GPT-OSS 120B (Medium)",
		}
		got, ok := ParseAvailableModels(agyRejectionFixture)
		if !ok {
			t.Fatal("expected ok=true, got ok=false")
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v\nwant %v", got, want)
		}
	})

	t.Run("returns ok=false when Available models header is absent", func(t *testing.T) {
		_, ok := ParseAvailableModels("Error: some other error\nno model list here\n")
		if ok {
			t.Fatal("expected ok=false when header is absent, got ok=true")
		}
	})

	t.Run("empty string returns ok=false", func(t *testing.T) {
		_, ok := ParseAvailableModels("")
		if ok {
			t.Fatal("expected ok=false for empty input")
		}
	})

	t.Run("stops collecting at blank line after header", func(t *testing.T) {
		input := "Available models:\n  Model A\n  Model B\n\n  Model C\n"
		got, ok := ParseAvailableModels(input)
		if !ok {
			t.Fatal("expected ok=true")
		}
		want := []string{"Model A", "Model B"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
}

// TestDiffCatalog covers AC-2.
func TestDiffCatalog(t *testing.T) {
	t.Run("computes declared_dead and live_undeclared with deterministic sorted ordering", func(t *testing.T) {
		// Spec-specified worked example:
		// declared {a -> "Gemini 3.5 Flash (High)", b -> "Gemini 3.6 Flash (High)"}
		// live ["Gemini 3.6 Flash (High)", "Gemini 3.8 Flash (High)"]
		// => declared_dead ["a"], live_undeclared ["Gemini 3.8 Flash (High)"]
		declared := map[string]string{
			"a": "Gemini 3.5 Flash (High)",
			"b": "Gemini 3.6 Flash (High)",
		}
		live := []string{"Gemini 3.6 Flash (High)", "Gemini 3.8 Flash (High)"}

		delta := DiffCatalog(declared, live)

		wantDead := []string{"a"}
		if !reflect.DeepEqual(delta.DeclaredDead, wantDead) {
			t.Errorf("DeclaredDead: got %v, want %v", delta.DeclaredDead, wantDead)
		}

		wantUndeclared := []string{"Gemini 3.8 Flash (High)"}
		if !reflect.DeepEqual(delta.LiveUndeclared, wantUndeclared) {
			t.Errorf("LiveUndeclared: got %v, want %v", delta.LiveUndeclared, wantUndeclared)
		}
	})

	t.Run("both slices are empty when declared and live are identical", func(t *testing.T) {
		declared := map[string]string{
			"x": "Model X",
			"y": "Model Y",
		}
		live := []string{"Model X", "Model Y"}
		delta := DiffCatalog(declared, live)
		if len(delta.DeclaredDead) != 0 {
			t.Errorf("expected no declared_dead, got %v", delta.DeclaredDead)
		}
		if len(delta.LiveUndeclared) != 0 {
			t.Errorf("expected no live_undeclared, got %v", delta.LiveUndeclared)
		}
	})

	t.Run("multiple dead keys are returned sorted", func(t *testing.T) {
		declared := map[string]string{
			"z-key": "Old Z",
			"a-key": "Old A",
			"m-key": "Old M",
		}
		live := []string{"New Model"}
		delta := DiffCatalog(declared, live)
		wantDead := []string{"a-key", "m-key", "z-key"}
		if !reflect.DeepEqual(delta.DeclaredDead, wantDead) {
			t.Errorf("DeclaredDead not sorted: got %v, want %v", delta.DeclaredDead, wantDead)
		}
	})

	t.Run("multiple undeclared live names are returned sorted", func(t *testing.T) {
		declared := map[string]string{}
		live := []string{"Zeta Model", "Alpha Model", "Mid Model"}
		delta := DiffCatalog(declared, live)
		wantUndeclared := []string{"Alpha Model", "Mid Model", "Zeta Model"}
		if !reflect.DeepEqual(delta.LiveUndeclared, wantUndeclared) {
			t.Errorf("LiveUndeclared not sorted: got %v, want %v", delta.LiveUndeclared, wantUndeclared)
		}
	})
}
