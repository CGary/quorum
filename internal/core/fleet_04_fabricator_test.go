package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runGitFab(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return string(out)
}

// newFabTestRepo creates a real git repo with one committed baseline file,
// returning the worktree dir and the baseline commit sha.
func newFabTestRepo(t *testing.T) (dir string, baseline string) {
	t.Helper()
	dir = t.TempDir()
	runGitFab(t, dir, "init", "-q")
	runGitFab(t, dir, "config", "user.email", "test@example.com")
	runGitFab(t, dir, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitFab(t, dir, "add", ".")
	runGitFab(t, dir, "commit", "-q", "-m", "baseline")
	baseline = strings.TrimSpace(runGitFab(t, dir, "rev-parse", "HEAD"))
	return dir, baseline
}

func baseFabResult(baseline string) DispatchResult {
	return DispatchResult{
		SchemaVersion:  FleetDispatchResultSchemaVersion,
		DispatchID:     "disp-001",
		TaskID:         "FLEET-008",
		Agent:          "ejecutor",
		Model:          "sonnet",
		Phase:          "execute",
		BaselineCommit: baseline,
		Applied:        true,
		Diff:           DispatchDiffStat{Empty: false, FilesChanged: 1},
	}
}

func TestFabricateImplementationLog_CleanSingleFileDiff(t *testing.T) {
	dir, baseline := newFabTestRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := baseFabResult(baseline)
	payload, err := FabricateImplementationLog(result, dir, "Implemented the feature cleanly.\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if payload == nil {
		t.Fatal("expected non-nil payload for non-empty diff")
	}
	if payload["task_id"] != "FLEET-008" {
		t.Fatalf("task_id = %v", payload["task_id"])
	}
	entries, ok := payload["entries"].([]any)
	if !ok || len(entries) != 1 {
		t.Fatalf("expected exactly 1 entry, got %#v", payload["entries"])
	}
	entry0 := entries[0].(map[string]any)
	files := asStrings(entry0["changed_files"])
	if len(files) != 1 || files[0] != "main.go" {
		t.Fatalf("expected changed_files=[main.go], got %#v", entry0["changed_files"])
	}

	if err := validatePayloadAsImplementationLog(t, payload); err != nil {
		t.Fatalf("schema validation failed: %v", err)
	}
}

func TestFabricateImplementationLog_DirtyMultiLanguageLongNotes(t *testing.T) {
	dir, baseline := newFabTestRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "script.py"), []byte("print('hi')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new_untracked.rb"), []byte("puts 'hi'\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	longLine := strings.Repeat("это очень длинная строка заметок разработчика ", 200)
	notes := longLine + "\n" + strings.Repeat("another very long line of narrative text ", 200)

	result := baseFabResult(baseline)
	payload, err := FabricateImplementationLog(result, dir, notes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if payload == nil {
		t.Fatal("expected non-nil payload")
	}
	entries := payload["entries"].([]any)
	entry0 := entries[0].(map[string]any)
	files := asStrings(entry0["changed_files"])
	if len(files) != 3 {
		t.Fatalf("expected 3 changed files (2 tracked + 1 untracked), got %#v", files)
	}
	notesArr := asStrings(entry0["notes"])
	for _, line := range notesArr {
		if len(line) > 2000 {
			t.Fatalf("expected each notes line to be truncated, got length %d", len(line))
		}
		if !strings.HasPrefix(line, "[delegate notes] ") {
			t.Fatalf("expected [delegate notes] prefix, got %q", line[:min(30, len(line))])
		}
	}

	if err := validatePayloadAsImplementationLog(t, payload); err != nil {
		t.Fatalf("schema validation failed: %v", err)
	}
}

func TestFabricateImplementationLog_EmptyNotesFallback(t *testing.T) {
	dir, baseline := newFabTestRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := baseFabResult(baseline)
	payload, err := FabricateImplementationLog(result, dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if payload == nil {
		t.Fatal("expected non-nil payload")
	}
	summary, _ := payload["summary"].(string)
	if !strings.Contains(summary, "ejecutor/sonnet") || !strings.Contains(summary, "disp-001") {
		t.Fatalf("expected synthetic summary fallback mentioning agent/model/dispatch id, got %q", summary)
	}
	entries := payload["entries"].([]any)
	entry0 := entries[0].(map[string]any)
	notesArr := asStrings(entry0["notes"])
	if len(notesArr) == 0 {
		t.Fatal("expected at least a placeholder notes line")
	}

	if err := validatePayloadAsImplementationLog(t, payload); err != nil {
		t.Fatalf("schema validation failed: %v", err)
	}
}

func TestFabricateImplementationLog_FiftyFileDiff(t *testing.T) {
	dir, baseline := newFabTestRepo(t)
	for i := 0; i < 50; i++ {
		name := filepath.Join(dir, fmt.Sprintf("file%03d.txt", i))
		if err := os.WriteFile(name, []byte("content"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	result := baseFabResult(baseline)
	payload, err := FabricateImplementationLog(result, dir, "bulk change")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	entries := payload["entries"].([]any)
	entry0 := entries[0].(map[string]any)
	files := asStrings(entry0["changed_files"])
	if len(files) != 50 {
		t.Fatalf("expected 50 changed files, got %d", len(files))
	}

	if err := validatePayloadAsImplementationLog(t, payload); err != nil {
		t.Fatalf("schema validation failed: %v", err)
	}
}

func TestFabricateImplementationLog_IgnoresDelegateClaimedFiles(t *testing.T) {
	dir, baseline := newFabTestRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main // A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.go"), []byte("package b\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := baseFabResult(baseline)
	notes := "I changed c.go and d.go to fix the bug."
	payload, err := FabricateImplementationLog(result, dir, notes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	entries := payload["entries"].([]any)
	entry0 := entries[0].(map[string]any)
	files := asStrings(entry0["changed_files"])
	want := map[string]bool{"a.go": true, "b.go": true}
	if len(files) != 2 {
		t.Fatalf("expected exactly 2 changed files, got %#v", files)
	}
	for _, f := range files {
		if !want[f] {
			t.Fatalf("unexpected file %q in changed_files, delegate-claimed files must never appear", f)
		}
	}
}

func TestFabricateImplementationLog_PersistsOnlyThroughSaveArtifact(t *testing.T) {
	dir, baseline := newFabTestRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := baseFabResult(baseline)
	payload, err := FabricateImplementationLog(result, dir, "did the thing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if payload == nil {
		t.Fatal("expected non-nil payload")
	}

	taskDir := t.TempDir()
	artifactPath := filepath.Join(taskDir, "04-implementation-log.yaml")
	if _, err := os.Stat(artifactPath); err == nil {
		t.Fatal("artifact must not exist before SaveArtifact is called")
	}
	saved, err := SaveArtifact(artifactPath, payload)
	if err != nil {
		t.Fatalf("SaveArtifact failed: %v", err)
	}
	if saved != artifactPath {
		t.Fatalf("expected saved path %q, got %q", artifactPath, saved)
	}
	if _, err := os.Stat(artifactPath); err != nil {
		t.Fatalf("expected artifact to exist on disk after SaveArtifact: %v", err)
	}
}

func TestFabricateImplementationLog_MalformedBinaryishNotesNoPanic(t *testing.T) {
	dir, baseline := newFabTestRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := baseFabResult(baseline)
	binaryish := string([]byte{0x00, 0xff, 0xfe, 0x01, 0x02}) + "\x00\x00garbled\x01\x02notes\x00"
	payload, err := FabricateImplementationLog(result, dir, binaryish)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if payload == nil {
		t.Fatal("expected non-nil payload even for malformed notes")
	}

	if err := validatePayloadAsImplementationLog(t, payload); err != nil {
		t.Fatalf("schema validation failed: %v", err)
	}
}

func TestFabricateImplementationLog_ZeroDiffReturnsNilNil(t *testing.T) {
	dir, baseline := newFabTestRepo(t)
	// No mutation: worktree is clean, matches baseline exactly.

	result := baseFabResult(baseline)
	result.Diff = DispatchDiffStat{Empty: true}
	payload, err := FabricateImplementationLog(result, dir, "delegate claims it did something")
	if err != nil {
		t.Fatalf("expected no error for zero diff, got %v", err)
	}
	if payload != nil {
		t.Fatalf("expected nil payload for zero-diff worktree, got %#v", payload)
	}
}

func TestFabricateImplementationLog_NeverReadsResultDiffOrClaimedFiles(t *testing.T) {
	dir, baseline := newFabTestRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "real.go"), []byte("package real\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := baseFabResult(baseline)
	// Deliberately mismatched/misleading Diff stat: claims empty, but the
	// worktree actually has a real change. Function must trust git, not this field.
	result.Diff = DispatchDiffStat{Empty: true, FilesChanged: 0}
	payload, err := FabricateImplementationLog(result, dir, "claims files x.go and y.go changed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if payload == nil {
		t.Fatal("expected non-nil payload: function must trust live git state over result.Diff")
	}
	entries := payload["entries"].([]any)
	entry0 := entries[0].(map[string]any)
	files := asStrings(entry0["changed_files"])
	if len(files) != 1 || files[0] != "real.go" {
		t.Fatalf("expected changed_files=[real.go] derived from git, got %#v", files)
	}
}

// asStrings converts a []any (as produced by json/yaml-shaped payloads) back
// to []string for test assertions.
func asStrings(v any) []string {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, _ := item.(string)
		out = append(out, s)
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// validatePayloadAsImplementationLog exercises the real schema validation path
// (ValidateArtifact) that SaveArtifact relies on, using a throwaway path name
// so schema resolution keys off "04-implementation-log.yaml".
func validatePayloadAsImplementationLog(t *testing.T, payload map[string]any) error {
	t.Helper()
	return ValidateArtifact("04-implementation-log.yaml", payload)
}
