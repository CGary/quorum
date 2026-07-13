package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestParseCodexJSONLUsageFound covers AC-7 "JSONL usage parse (found)": the
// LAST usage-shaped event's counters are returned as cli_reported, never
// fabricated beyond what the fixture actually contains.
func TestParseCodexJSONLUsageFound(t *testing.T) {
	raw := "{\"type\":\"reasoning\",\"text\":\"thinking\"}\n" +
		"{\"type\":\"agent_message\",\"input_tokens\":100,\"output_tokens\":50}\n" +
		"{\"type\":\"token_count\",\"usage\":{\"input_tokens\":140,\"output_tokens\":60}}\n"
	got := ParseCodexJSONLUsage(raw)
	if got.Source != "cli_reported" {
		t.Fatalf("want source=cli_reported, got %q", got.Source)
	}
	if got.TokensIn == nil || *got.TokensIn != 140 {
		t.Fatalf("want tokens_in=140 (last event wins), got %+v", got.TokensIn)
	}
	if got.TokensOut == nil || *got.TokensOut != 60 {
		t.Fatalf("want tokens_out=60 (last event wins), got %+v", got.TokensOut)
	}
}

// TestParseCodexJSONLUsageAbsent covers AC-7 "JSONL usage parse (absent)":
// no usage-shaped event anywhere in the stream must yield Source: none with
// every counter nil -- never a fabricated zero.
func TestParseCodexJSONLUsageAbsent(t *testing.T) {
	raw := "{\"type\":\"reasoning\",\"text\":\"thinking\"}\n{\"type\":\"agent_message\",\"text\":\"done\"}\n"
	got := ParseCodexJSONLUsage(raw)
	if got.Source != "none" {
		t.Fatalf("want source=none, got %q", got.Source)
	}
	if got.TokensIn != nil || got.TokensOut != nil || got.Requests != nil {
		t.Fatalf("want all counters nil, got %+v", got)
	}
}

// TestParseCodexJSONLUsageMalformed covers AC-7 "JSONL usage parse
// (malformed)": garbage/non-JSON lines never panic and never surface as
// cli_reported.
func TestParseCodexJSONLUsageMalformed(t *testing.T) {
	raw := "<<< this is not valid json at all >>>\n{not json either\n\n"
	got := ParseCodexJSONLUsage(raw)
	if got.Source != "none" {
		t.Fatalf("want source=none for malformed input, got %q", got.Source)
	}
	if got.TokensIn != nil || got.TokensOut != nil {
		t.Fatalf("want nil counters for malformed input, got %+v", got)
	}
}

// TestReadCodexLastMessageFound covers AC-7 "last-message file (found)": the
// adapter reads the plain trimmed text codex wrote via -o, not the raw JSONL
// stream.
func TestReadCodexLastMessageFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "last-message.txt")
	if err := os.WriteFile(path, []byte("  final answer from codex  \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadCodexLastMessage(path)
	if err != nil {
		t.Fatalf("ReadCodexLastMessage: %v", err)
	}
	if got != "final answer from codex" {
		t.Fatalf("want trimmed last message, got %q", got)
	}
}

// TestReadCodexLastMessageMissing covers AC-7 "last-message file
// (missing/empty)": an absent file degrades to empty string, no error --
// dispatch enrichment must never block on this.
func TestReadCodexLastMessageMissing(t *testing.T) {
	dir := t.TempDir()
	got, err := ReadCodexLastMessage(filepath.Join(dir, "does-not-exist.txt"))
	if err != nil {
		t.Fatalf("want no error for missing file, got %v", err)
	}
	if got != "" {
		t.Fatalf("want empty string for missing file, got %q", got)
	}
}

// TestApplyDispatchResultUsage covers the generic JSON patch: only the
// top-level "usage" key changes, every other result.json field is untouched.
func TestApplyDispatchResultUsage(t *testing.T) {
	dir := t.TempDir()
	resultPath := filepath.Join(dir, "result.json")
	original := DispatchResult{
		SchemaVersion: FleetDispatchResultSchemaVersion,
		DispatchID:    "d1",
		TaskID:        "FLEET-900",
		Agent:         "codex",
		Model:         "openai/gpt-5.5-medium",
		Phase:         "execute",
		Outcome:       DispatchOutcome{Class: "attempt"},
		Applied:       true,
		Usage:         DispatchUsage{Source: "none"},
		TraceEvents:   []string{"dispatch_started", "dispatch_finished"},
	}
	raw, err := json.MarshalIndent(original, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resultPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	tokensIn, tokensOut := 120, 34
	usage := DispatchUsage{Source: "cli_reported", TokensIn: &tokensIn, TokensOut: &tokensOut}
	if err := ApplyDispatchResultUsage(resultPath, usage); err != nil {
		t.Fatalf("ApplyDispatchResultUsage: %v", err)
	}
	patched, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	var got DispatchResult
	if err := json.Unmarshal(patched, &got); err != nil {
		t.Fatalf("unmarshal patched result.json: %v", err)
	}
	if got.Usage.Source != "cli_reported" || got.Usage.TokensIn == nil || *got.Usage.TokensIn != 120 || got.Usage.TokensOut == nil || *got.Usage.TokensOut != 34 {
		t.Fatalf("want patched usage, got %+v", got.Usage)
	}
	if got.SchemaVersion != original.SchemaVersion || got.DispatchID != original.DispatchID ||
		got.TaskID != original.TaskID || got.Agent != original.Agent || got.Model != original.Model ||
		got.Outcome.Class != original.Outcome.Class || got.Applied != original.Applied {
		t.Fatalf("want every other field unchanged, got %+v want-from %+v", got, original)
	}
}
