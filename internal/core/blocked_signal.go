package core

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// blockedMarkerRe locates the standardized BLOCKED sentinel on its own line.
// The rich question payload (a JSON object) follows the marker. The legacy
// single-line form (BLOCKED: missing_file=...) also carries this marker, which
// is exactly why it is DETECTED as an attempted-but-malformed question and
// classified as attempt/malformed_question rather than silently ignored.
var blockedMarkerRe = regexp.MustCompile(`(?m)^\s*BLOCKED:`)

// BlockedOption is one decidable option offered by a blocked question. An
// option without a Consequence is not a valid option: the whole point of the
// rich protocol is that every choice states its cost/benefit.
type BlockedOption struct {
	Label       string `json:"label"`
	Consequence string `json:"consequence"`
}

// BlockedQuestion is the schema-validated rich question a delegate emits when
// it cannot proceed. It replaces the legacy single-line BlockedSignal
// (missing_file/reason/severity): an incomplete question is not a question, so
// a payload that fails Validate is never classified as blocked.
type BlockedQuestion struct {
	Question       string          `json:"question"`
	Attempted      []string        `json:"attempted,omitempty"`
	Discarded      []string        `json:"discarded,omitempty"`
	Evidence       []string        `json:"evidence"`
	Options        []BlockedOption `json:"options"`
	Recommendation string          `json:"recommendation,omitempty"`
	OpenOption     string          `json:"open_option"`
}

// hasBlockedMarker reports whether the payload carries the BLOCKED sentinel,
// i.e. the delegate attempted to signal a block (validly or not). Pure and
// IO-free.
func hasBlockedMarker(payload string) bool {
	return blockedMarkerRe.MatchString(payload)
}

// ParseBlockedSignal parses and validates a rich BLOCKED question payload. It
// is a pure, IO-free Application Service: it locates the BLOCKED marker, parses
// the JSON object that follows it, then runs the rich-schema Validator. The
// legacy single-line format (BLOCKED: missing_file=...) is rejected here: the
// text after the marker is not a JSON object, so JSON decoding fails. There is
// no dual-format coexistence.
func ParseBlockedSignal(message string) (*BlockedQuestion, error) {
	loc := blockedMarkerRe.FindStringIndex(message)
	if loc == nil {
		return nil, fmt.Errorf("blocked question must carry a BLOCKED: marker followed by a JSON payload")
	}
	candidate := strings.TrimSpace(message[loc[1]:])
	if candidate == "" {
		return nil, fmt.Errorf("blocked question payload is empty after the BLOCKED: marker")
	}

	var q BlockedQuestion
	// Decode a single JSON value and ignore any trailing text (e.g. a NOTES
	// block after the payload). This is why a legacy `missing_file=...` line
	// fails: it is not a JSON object.
	dec := json.NewDecoder(strings.NewReader(candidate))
	if err := dec.Decode(&q); err != nil {
		return nil, fmt.Errorf("blocked question payload is not valid JSON: %w", err)
	}
	if err := q.Validate(); err != nil {
		return nil, err
	}
	return &q, nil
}

// Validate enforces the rich-question invariants (pure, IO-free): a decidable
// question, at least one concrete evidence entry, at least two options each
// carrying a non-empty consequence, and an always-present non-empty open
// option. Recommendation is optional.
func (q *BlockedQuestion) Validate() error {
	if strings.TrimSpace(q.Question) == "" {
		return fmt.Errorf("blocked question requires a non-empty question")
	}
	if countNonEmpty(q.Evidence) < 1 {
		return fmt.Errorf("blocked question requires at least one non-empty evidence entry")
	}
	if len(q.Options) < 2 {
		return fmt.Errorf("blocked question requires at least two options, got %d", len(q.Options))
	}
	for i, opt := range q.Options {
		if strings.TrimSpace(opt.Label) == "" {
			return fmt.Errorf("blocked question option %d requires a non-empty label", i+1)
		}
		if strings.TrimSpace(opt.Consequence) == "" {
			return fmt.Errorf("blocked question option %d (%q) requires a non-empty consequence", i+1, opt.Label)
		}
	}
	if strings.TrimSpace(q.OpenOption) == "" {
		return fmt.Errorf("blocked question requires a non-empty open_option")
	}
	return nil
}

func countNonEmpty(values []string) int {
	n := 0
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			n++
		}
	}
	return n
}

// AppendBlockedAnswer appends exactly one blocked_answer event (a reserved
// ADR 0011 event type; clarification only, never a contract mutation) to the
// task's 07-trace.json via the same append-only, schema-validated SaveArtifact
// path appendDispatchTrace uses. Unlike the parser above, this function is
// intentionally IO-bound: it persists to disk. It never rewrites or shortens
// existing attempts[]/events[]; SaveArtifact's EnsureTraceAppendOnly enforces
// that.
func AppendBlockedAnswer(taskDir, dispatchID, answer, answeredBy string) error {
	if strings.TrimSpace(answer) == "" {
		return fmt.Errorf("blocked answer requires a non-empty answer")
	}
	tracePath := filepath.Join(taskDir, "07-trace.json")
	payload, err := LoadArtifactPayload(tracePath)
	if err != nil {
		return fmt.Errorf("cannot load trace for blocked answer: %w", err)
	}
	trace, ok := payload.(map[string]any)
	if !ok {
		return fmt.Errorf("trace payload is not an object")
	}
	events, _ := asSlice(trace["events"])
	ev := map[string]any{
		"type":        "blocked_answer",
		"ts":          time.Now().UTC().Format(time.RFC3339),
		"dispatch_id": dispatchID,
		"answer":      answer,
	}
	if strings.TrimSpace(answeredBy) != "" {
		ev["answered_by"] = answeredBy
	}
	trace["events"] = append(events, ev)
	_, err = TaskStore{}.SaveArtifact(&TaskDirMatch{Path: taskDir, Location: "active"}, "07-trace.json", trace)
	return err
}
