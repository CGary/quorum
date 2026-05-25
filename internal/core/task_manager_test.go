package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mkRoot(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}
func mkSpec(t *testing.T, root, loc, dir, id string) {
	t.Helper()
	p := filepath.Join(root, ".ai", "tasks", loc, dir)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "00-spec.yaml"), []byte("task_id: "+id+"\nsummary: test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
func useSchemas(t *testing.T) {
	t.Helper()
	t.Setenv("QUORUM_SCHEMAS_DIR", filepath.Join(os.Getenv("PWD"), "..", "..", ".agents", "schemas"))
}

func TestFindTaskDirParity(t *testing.T) {
	root := mkRoot(t)
	mkSpec(t, root, "active", "LEGACY-999-slug", "FEAT-001")
	mkSpec(t, root, "active", "FEAT-001", "OTHER-001")
	m, err := FindTaskDirIn(root, "FEAT-001", []string{"active"})
	if err != nil || m == nil || filepath.Base(m.Path) != "LEGACY-999-slug" || m.Location != "active" {
		t.Fatalf("yaml tier = %#v, %v", m, err)
	}

	mkSpec(t, root, "active", "FEAT-002", "OTHER-002")
	m, err = FindTaskDirIn(root, "FEAT-002", []string{"active"})
	if err != nil || m == nil || filepath.Base(m.Path) != "FEAT-002" {
		t.Fatalf("exact tier = %#v, %v", m, err)
	}

	mkSpec(t, root, "inbox", "FEAT-003-a-new-spec", "FEAT-003-a")
	m, err = FindTaskDirIn(root, "FEAT-003", []string{"inbox"})
	if err != nil || m != nil {
		t.Fatalf("parent resolved to child = %#v, %v", m, err)
	}
	m, err = FindTaskDirIn(root, "FEAT-003-a", []string{"inbox"})
	if err != nil || m == nil || filepath.Base(m.Path) != "FEAT-003-a-new-spec" {
		t.Fatalf("child tier = %#v, %v", m, err)
	}

	mkSpec(t, root, "done", "FEAT-004-alpha", "OTHER-004")
	mkSpec(t, root, "done", "FEAT-004-beta", "OTHER-005")
	_, err = FindTaskDirIn(root, "FEAT-004", []string{"done"})
	if err == nil || !strings.Contains(err.Error(), "AMBIGUITY ERROR") || !strings.Contains(err.Error(), "done/FEAT-004-alpha") {
		t.Fatalf("ambiguity = %v", err)
	}
}

func TestValidateArtifactPythonErrorFormat(t *testing.T) {
	useSchemas(t)
	cases := []struct {
		path    string
		payload map[string]any
		want    string
	}{
		{"01-blueprint.yaml", map[string]any{"task_id": "FEAT-001", "summary": "valid blueprint", "affected_files": []any{"src/a.py"}, "symbols": []any{}, "dependencies": []any{}}, "artifact=01-blueprint.yaml; field=$; reason='test_scenarios' is a required property"},
		{"05-validation.json", map[string]any{"task_id": "FEAT-001", "summary": "invalid", "executed_at": "2026-05-03T00:00:00Z", "commands": []any{}, "overall_result": "passed"}, "artifact=05-validation.json; field=$.commands; reason=[] should be non-empty"},
		{"05-validation.json", map[string]any{"task_id": "FEAT-001", "summary": "invalid", "executed_at": "2026-05-03T00:00:00Z", "commands": []any{map[string]any{"command": "pytest", "exit_code": 0, "duration_s": 1.0, "output_excerpt": "", "extra": true}}, "overall_result": "passed"}, "artifact=05-validation.json; field=$.commands[0]; reason=Additional properties are not allowed ('extra' was unexpected)"},
	}
	for _, tc := range cases {
		if err := ValidateArtifact(tc.path, tc.payload); err == nil || err.Error() != tc.want {
			t.Fatalf("%s error = %v, want %s", tc.path, err, tc.want)
		}
	}
}

func TestEnsureTraceAppendOnly(t *testing.T) {
	existing := map[string]any{"attempts": []any{map[string]any{"phase": "blueprint", "result": "passed", "duration_s": 1.0}}}
	appended := map[string]any{"attempts": []any{map[string]any{"phase": "blueprint", "result": "passed", "duration_s": 1.0}, map[string]any{"phase": "execute", "result": "passed", "duration_s": 2.0}}}
	if err := EnsureTraceAppendOnly("07-trace.json", existing, appended); err != nil {
		t.Fatal(err)
	}
	want := "artifact=07-trace.json; field=$.attempts; reason=append-only trace cannot remove existing attempts"
	if err := EnsureTraceAppendOnly("07-trace.json", existing, map[string]any{"attempts": []any{}}); err == nil || err.Error() != want {
		t.Fatalf("shorten = %v", err)
	}
	want = "artifact=07-trace.json; field=$.attempts; reason=append-only trace cannot reorder or mutate existing attempts"
	mutated := map[string]any{"attempts": []any{map[string]any{"phase": "blueprint", "result": "failed", "duration_s": 1.0}}}
	if err := EnsureTraceAppendOnly("07-trace.json", existing, mutated); err == nil || err.Error() != want {
		t.Fatalf("mutate = %v", err)
	}
}
