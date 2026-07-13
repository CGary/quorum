package core

import (
	"fmt"
	"os/exec"
	"strings"
)

// maxNotesLineLen bounds each individual notes[] line. The schema imposes no
// maxLength on notes strings, so this is a defensive, deterministic limit
// chosen by this fabricator (not by the schema).
const maxNotesLineLen = 500

// maxNotesLines bounds how many notes[] lines a single entry may carry, so a
// pathological delegate notes blob cannot grow the artifact unboundedly.
const maxNotesLines = 50

// summaryMaxLen matches the schema's summary maxLength:200.
const summaryMaxLen = 200

// fabricateChangedFiles derives the list of files touched in worktree relative
// to baseline using only git plumbing: `git diff --name-only <baseline>` for
// tracked changes, unioned with `git ls-files --others --exclude-standard` for
// new untracked files. It never consults delegate-claimed file lists. Both
// git invocations are read-only against the repository (no staging changes).
func fabricateChangedFiles(worktree string, baseline string) ([]string, error) {
	seen := map[string]bool{}
	var ordered []string

	add := func(raw string) {
		f := strings.TrimSpace(raw)
		if f == "" || seen[f] {
			return
		}
		seen[f] = true
		ordered = append(ordered, f)
	}

	if baseline != "" {
		diffOut, err := runGitFabCmd(worktree, "diff", "--name-only", baseline)
		if err != nil {
			return nil, fmt.Errorf("git diff --name-only failed: %w", err)
		}
		for _, line := range strings.Split(diffOut, "\n") {
			add(line)
		}
	}

	untrackedOut, err := runGitFabCmd(worktree, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, fmt.Errorf("git ls-files --others failed: %w", err)
	}
	for _, line := range strings.Split(untrackedOut, "\n") {
		add(line)
	}

	return ordered, nil
}

func runGitFabCmd(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// truncateDeterministic truncates s to at most n bytes, deterministically
// (no locale/collation dependence). It never panics on malformed/binary-ish
// input since it operates on raw bytes.
func truncateDeterministic(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// fabricateNotes builds the notes[] array from raw delegate notes text.
// Every line is prefixed with the literal "[delegate notes] " marker (no
// translation/reformatting), truncated deterministically per-line and capped
// in total line count. Empty notesText yields a single deterministic
// placeholder line so the schema's notes minItems:1 is always satisfied.
func fabricateNotes(notesText string) []string {
	trimmed := strings.TrimSpace(notesText)
	if trimmed == "" {
		return []string{"[delegate notes] (no notes provided by delegate)"}
	}

	rawLines := strings.Split(trimmed, "\n")
	notes := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		prefixed := "[delegate notes] " + truncateDeterministic(line, maxNotesLineLen)
		notes = append(notes, prefixed)
		if len(notes) >= maxNotesLines {
			break
		}
	}

	if len(notes) == 0 {
		// Notes text was non-empty but contained only blank lines (or was
		// otherwise unusable after trimming); fall back to the placeholder
		// so minItems:1 still holds.
		return []string{"[delegate notes] (no notes provided by delegate)"}
	}
	return notes
}

// fabricateSummary derives the entries[].. summary line: the first non-empty
// trimmed line of notesText when present, otherwise a deterministic synthetic
// fallback describing the dispatch. Always truncated to the schema's
// summary maxLength (200), defensively even for the synthetic fallback.
func fabricateSummary(result DispatchResult, notesText string, fileCount int) string {
	trimmed := strings.TrimSpace(notesText)
	if trimmed != "" {
		for _, line := range strings.Split(trimmed, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				return truncateDeterministic(line, summaryMaxLen)
			}
		}
	}

	fallback := fmt.Sprintf(
		"Delegated implementation via %s/%s; %d files changed. See trace events %s.",
		result.Agent, result.Model, fileCount, result.DispatchID,
	)
	return truncateDeterministic(fallback, summaryMaxLen)
}

// toAnySlice converts a []string into []any so payloads built by this
// fabricator match the plain-JSON-value shape (map[string]any / []any /
// string / bool / float64 / nil) that the schema validation engine
// (santhosh-tekuri/jsonschema) requires; a native []string fails its type
// switch even though it is semantically a JSON array of strings.
func toAnySlice(items []string) []any {
	out := make([]any, len(items))
	for i, s := range items {
		out[i] = s
	}
	return out
}

// FabricateImplementationLog builds a schema-valid single-attempt fragment of
// 04-implementation-log.yaml from git facts (changed_files, derived only from
// the worktree's live git state) and delegate notes (narrative only, never a
// source of truth for changed_files). It performs no filesystem writes and no
// LLM/network calls; the only I/O is read-only git subprocess invocation
// against worktree.
//
// Per decision D12, an attempt whose worktree diff is empty (no tracked
// changes relative to result.BaselineCommit, no new untracked files) returns
// (nil, nil): no entry is fabricated and no schema is touched. Persisting the
// returned payload is exclusively the caller's responsibility, via
// SaveArtifact / `quorum task artifact-save`.
func FabricateImplementationLog(result DispatchResult, worktree string, notesText string) (map[string]any, error) {
	changedFiles, err := fabricateChangedFiles(worktree, result.BaselineCommit)
	if err != nil {
		return nil, err
	}

	if len(changedFiles) == 0 {
		// D12: zero net diff. Never invent a changed_files entry to satisfy
		// the schema's minItems:1 constraint; this attempt belongs only in
		// 07-trace.json, produced elsewhere by the dispatcher.
		return nil, nil
	}

	notes := fabricateNotes(notesText)
	summary := fabricateSummary(result, notesText, len(changedFiles))

	entry := map[string]any{
		"changed_files":  toAnySlice(changedFiles),
		"notes":          toAnySlice(notes),
		"verify_pending": true,
	}

	payload := map[string]any{
		"task_id": result.TaskID,
		"summary": summary,
		"entries": []any{entry},
	}

	return payload, nil
}
