package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const validRichQuestion = `BLOCKED:
{
  "question": "Should cmd/new_helper.go be added to touch?",
  "attempted": ["searched for an existing helper"],
  "discarded": ["inlining the helper: duplicates logic"],
  "evidence": ["blueprint references cmd/new_helper.go but it is absent from touch"],
  "options": [
    {"label": "add to touch", "consequence": "expands contract scope; needs human approval"},
    {"label": "inline helper", "consequence": "keeps scope but duplicates logic"}
  ],
  "recommendation": "add to touch",
  "open_option": "propose another path if neither fits"
}`

// AC-1: a well-formed rich payload parses and fully populates the question.
func TestParseBlockedSignal_ValidRichQuestion(t *testing.T) {
	q, err := ParseBlockedSignal(validRichQuestion)
	if err != nil {
		t.Fatalf("expected a valid rich question, got error: %v", err)
	}
	if q.Question == "" {
		t.Error("question must be populated")
	}
	if len(q.Evidence) < 1 {
		t.Error("evidence must have at least one entry")
	}
	if len(q.Options) < 2 {
		t.Errorf("expected >=2 options, got %d", len(q.Options))
	}
	for i, opt := range q.Options {
		if opt.Consequence == "" {
			t.Errorf("option %d is missing a consequence", i)
		}
	}
	if q.OpenOption == "" {
		t.Error("open_option must be non-empty")
	}
	if q.Recommendation == "" {
		t.Error("recommendation should round-trip when present")
	}
}

// AC-2: incomplete rich payloads are rejected (never classified as blocked).
func TestParseBlockedSignal_RejectsIncomplete(t *testing.T) {
	cases := map[string]string{
		"missing evidence": `BLOCKED:
{"question":"q?","options":[{"label":"a","consequence":"x"},{"label":"b","consequence":"y"}],"open_option":"o"}`,
		"fewer than two options": `BLOCKED:
{"question":"q?","evidence":["e"],"options":[{"label":"a","consequence":"x"}],"open_option":"o"}`,
		"option missing consequence": `BLOCKED:
{"question":"q?","evidence":["e"],"options":[{"label":"a","consequence":"x"},{"label":"b"}],"open_option":"o"}`,
		"missing open_option": `BLOCKED:
{"question":"q?","evidence":["e"],"options":[{"label":"a","consequence":"x"},{"label":"b","consequence":"y"}]}`,
		"empty question": `BLOCKED:
{"question":"  ","evidence":["e"],"options":[{"label":"a","consequence":"x"},{"label":"b","consequence":"y"}],"open_option":"o"}`,
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseBlockedSignal(payload); err == nil {
				t.Fatalf("expected rejection for %s, got nil error", name)
			}
			// The marker is present, so the dispatcher must treat it as a
			// malformed question (attempt/malformed_question), not silence.
			if !hasBlockedMarker(payload) {
				t.Fatalf("%s: expected the BLOCKED marker to be detectable", name)
			}
		})
	}
}

// AC-3: the removed legacy single-line format and non-JSON payloads are
// rejected. The legacy line still carries the BLOCKED marker, so it is a
// malformed question rather than an ignored line.
func TestParseBlockedSignal_RejectsLegacyAndNonJSON(t *testing.T) {
	legacy := "BLOCKED: missing_file=src/foo.go; reason=Need to add to touch; severity=critical"
	if _, err := ParseBlockedSignal(legacy); err == nil {
		t.Fatal("expected the legacy single-line format to be rejected")
	}
	if !hasBlockedMarker(legacy) {
		t.Fatal("legacy line must still be detected as an attempted (malformed) question")
	}

	// No marker at all: not a blocked question and not a malformed one.
	plain := "just an error, could not complete the task"
	if _, err := ParseBlockedSignal(plain); err == nil {
		t.Fatal("expected a payload without a BLOCKED marker to be rejected")
	}
	if hasBlockedMarker(plain) {
		t.Fatal("a plain error line must not be detected as a blocked question")
	}
}

// AC-5: AppendBlockedAnswer appends one blocked_answer event and leaves the
// prior attempts[]/events[] unchanged, and the result validates against
// trace.schema.json (SaveArtifact validates before writing).
func TestAppendBlockedAnswer_AppendsWithoutRewritingPriorHistory(t *testing.T) {
	useSchemas(t)
	taskDir := t.TempDir()
	tracePath := filepath.Join(taskDir, "07-trace.json")
	seed := `{
  "task_id": "FLEET-777",
  "summary": "append blocked answer fixture",
  "started_at": "2026-07-24T00:00:00Z",
  "attempts": [{"phase":"execute","result":"failed","duration_s":1.5,"model":"m/x"}],
  "events": [
    {"type":"dispatch_started","ts":"2026-07-24T00:00:01Z","dispatch_id":"d1","bundle_hash":"abc"},
    {"type":"blocked_question","ts":"2026-07-24T00:00:02Z","dispatch_id":"d1","question":"add to touch?","open_option":"or else"},
    {"type":"dispatch_finished","ts":"2026-07-24T00:00:02Z","dispatch_id":"d1","outcome_class":"blocked"}
  ],
  "total_cost_usd": 0,
  "violations": [],
  "context_overflows": []
}`
	if err := os.WriteFile(tracePath, []byte(seed), 0o644); err != nil {
		t.Fatalf("write seed trace: %v", err)
	}

	beforeEvents := traceSlice(t, tracePath, "events")
	beforeAttempts := traceSlice(t, tracePath, "attempts")

	if err := AppendBlockedAnswer(taskDir, "d1", "add cmd/new_helper.go to touch", "human"); err != nil {
		t.Fatalf("AppendBlockedAnswer: %v", err)
	}

	afterEvents := traceSlice(t, tracePath, "events")
	afterAttempts := traceSlice(t, tracePath, "attempts")

	// attempts[] is untouched entirely.
	if !reflect.DeepEqual(beforeAttempts, afterAttempts) {
		t.Fatalf("attempts[] must be unchanged; before=%v after=%v", beforeAttempts, afterAttempts)
	}
	// events[] grows by exactly one, and every prior event is unchanged.
	if len(afterEvents) != len(beforeEvents)+1 {
		t.Fatalf("expected exactly one new event, before=%d after=%d", len(beforeEvents), len(afterEvents))
	}
	for i := range beforeEvents {
		if !reflect.DeepEqual(beforeEvents[i], afterEvents[i]) {
			t.Fatalf("prior event %d was rewritten:\n before=%v\n after=%v", i, beforeEvents[i], afterEvents[i])
		}
	}
	last, ok := afterEvents[len(afterEvents)-1].(map[string]any)
	if !ok {
		t.Fatalf("appended event is not an object: %v", afterEvents[len(afterEvents)-1])
	}
	if last["type"] != "blocked_answer" {
		t.Fatalf("appended event type = %v, want blocked_answer", last["type"])
	}
	if last["answer"] != "add cmd/new_helper.go to touch" || last["answered_by"] != "human" {
		t.Fatalf("appended blocked_answer missing answer/answered_by: %v", last)
	}

	// AppendBlockedAnswer must reject an empty answer.
	if err := AppendBlockedAnswer(taskDir, "d1", "   ", "human"); err == nil {
		t.Fatal("expected an empty answer to be rejected")
	}
}

func traceSlice(t *testing.T, path, key string) []any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal trace: %v", err)
	}
	s, _ := m[key].([]any)
	return s
}
