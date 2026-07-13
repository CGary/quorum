package core

import (
	"encoding/json"
	"os"
	"strings"
)

// Codex adapter (FLEET-006-b): the generic dispatch engine (FLEET-006-a,
// fleet_dispatch.go) already builds argv from the agents.yaml template and
// delivers the prompt over stdin -- both are transport-agnostic. This file
// adds the three pieces the engine cannot know about for codex specifically:
// JSONL usage parsing, the -o last-message file reader, and a generic
// result.json usage-field patch. None of it touches the engine.

// ParseCodexJSONLUsage scans raw `codex exec --json` JSONL output for a
// usage-shaped event and returns the LAST such event's counters as
// authoritative (later events supersede earlier partial/running totals).
//
// No repository fixture of real codex --json usage/quota-exhaustion output
// exists yet: ideas/fleet/resultados-fase-0a.md only validated
// model-unavailable and cwd-reliability, not usage reporting. This parser is
// therefore deliberately lenient about a few plausible field-name spellings
// (input_tokens/output_tokens, prompt_tokens/completion_tokens, either at
// the event's top level or nested under a "usage" key) but fails CLOSED:
// any line that is not valid JSON, or that carries none of the known
// numeric fields, contributes nothing, and if no event anywhere in the
// stream carries a usable counter the result is Source: "none" with every
// counter nil. Usage is never fabricated.
func ParseCodexJSONLUsage(raw string) DispatchUsage {
	usage := DispatchUsage{Source: "none"}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue // fail closed: malformed lines are ignored, never fatal
		}
		if found, ok := extractCodexUsage(event); ok {
			usage = found
		}
	}
	return usage
}

// extractCodexUsage looks for usage counters at the event's top level and,
// failing that, nested under a "usage" key, returning the first shape (of
// those two) that carries at least one known numeric field.
func extractCodexUsage(event map[string]any) (DispatchUsage, bool) {
	candidates := []map[string]any{event}
	if nested, ok := event["usage"].(map[string]any); ok {
		candidates = append(candidates, nested)
	}
	for _, c := range candidates {
		tokensIn := firstIntField(c, "input_tokens", "prompt_tokens")
		tokensOut := firstIntField(c, "output_tokens", "completion_tokens")
		requests := firstIntField(c, "requests", "request_count", "num_requests")
		if tokensIn == nil && tokensOut == nil && requests == nil {
			continue
		}
		return DispatchUsage{Source: "cli_reported", TokensIn: tokensIn, TokensOut: tokensOut, Requests: requests}, true
	}
	return DispatchUsage{}, false
}

func firstIntField(m map[string]any, keys ...string) *int {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if n, ok := numberToInt(v); ok {
				return &n
			}
		}
	}
	return nil
}

func numberToInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	default:
		return 0, false
	}
}

// ReadCodexLastMessage reads the file codex's own -o flag writes (the final
// assistant message, plain text, not the raw JSONL stream) and returns its
// trimmed contents. A missing or empty file degrades gracefully to ("", nil)
// so enrichment never blocks or fails a dispatch that otherwise succeeded.
func ReadCodexLastMessage(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

// ApplyDispatchResultUsage is a generic, agent-agnostic JSON read-modify-write
// over an already-written result.json: it loads the document as a plain map,
// replaces only the top-level "usage" key via a DispatchUsage round trip
// through encoding/json, and rewrites the file. It never touches
// schema_version, outcome, diff, applied, forensic_ref, or any other field,
// and is reusable by any future adapter that gains real usage reporting --
// nothing here is codex-specific by name.
func ApplyDispatchResultUsage(resultPath string, usage DispatchUsage) error {
	raw, err := os.ReadFile(resultPath)
	if err != nil {
		return err
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return err
	}
	usageBytes, err := json.Marshal(usage)
	if err != nil {
		return err
	}
	var usageMap map[string]any
	if err := json.Unmarshal(usageBytes, &usageMap); err != nil {
		return err
	}
	doc["usage"] = usageMap
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(resultPath, append(out, '\n'), 0o644)
}
