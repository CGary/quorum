package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTaskArtifactPath(t *testing.T) {
	store := NewTaskStore("/tmp/proj")
	task := &TaskDirMatch{Path: "/tmp/proj/.ai/tasks/active/TEST-1", Location: "active"}

	validNames := []string{"00-spec.yaml", "feedback.json", "custom.yaml"}
	for _, name := range validNames {
		path, err := store.TaskArtifactPath(task, name)
		if err != nil {
			t.Errorf("expected %q to be valid, got err: %v", name, err)
		}
		if path != filepath.Join(task.Path, name) {
			t.Errorf("expected %q, got %q", filepath.Join(task.Path, name), path)
		}
	}

	invalidNames := []string{
		"", ".", "..", "../00-spec.yaml", "subdir/file.yaml", "subdir\\file.yaml", "/tmp/file.yaml",
	}
	for _, name := range invalidNames {
		_, err := store.TaskArtifactPath(task, name)
		if err == nil {
			t.Errorf("expected %q to be invalid, got nil err", name)
		}
	}
}

func TestParseArtifactPayload(t *testing.T) {
	jsonRaw := []byte(`{"hello":"world"}`)
	pJSON, err := ParseArtifactPayload("file.json", jsonRaw)
	if err != nil {
		t.Errorf("ParseArtifactPayload JSON failed: %v", err)
	}
	mJSON, ok := pJSON.(map[string]any)
	if !ok || mJSON["hello"] != "world" {
		t.Errorf("ParseArtifactPayload JSON result wrong: %v", pJSON)
	}

	yamlRaw := []byte("hello: world\n")
	pYAML, err := ParseArtifactPayload("file.yaml", yamlRaw)
	if err != nil {
		t.Errorf("ParseArtifactPayload YAML failed: %v", err)
	}
	mYAML, ok := pYAML.(map[string]any)
	if !ok || mYAML["hello"] != "world" {
		t.Errorf("ParseArtifactPayload YAML result wrong: %v", pYAML)
	}

	if _, err := ParseArtifactPayload("file.json", []byte(`{"broken"`)); err == nil {
		t.Errorf("expected invalid JSON to fail")
	}
	if _, err := ParseArtifactPayload("file.yaml", []byte(":\n")); err == nil {
		t.Errorf("expected invalid YAML to fail")
	}

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "payload.yaml")
	if err := os.WriteFile(path, yamlRaw, 0644); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadArtifactPayload(path)
	if err != nil {
		t.Fatalf("LoadArtifactPayload wrapper failed: %v", err)
	}
	mLoaded, ok := loaded.(map[string]any)
	if !ok || mLoaded["hello"] != "world" {
		t.Fatalf("LoadArtifactPayload wrapper result wrong: %#v", loaded)
	}
}

func TestTaskStoreLoadArtifactValidates(t *testing.T) {
	useSchemas(t)
	tmpDir := t.TempDir()
	store := NewTaskStore(tmpDir)
	taskPath := filepath.Join(tmpDir, ".ai", "tasks", "active", "FEAT-001")
	if err := os.MkdirAll(taskPath, 0755); err != nil {
		t.Fatal(err)
	}
	task := &TaskDirMatch{Path: taskPath, Location: "active"}

	validSpec := []byte("task_id: FEAT-001\nsummary: Valid spec\ngoal: Exercise TaskStore load validation.\ninvariants:\n  - Preserve invariants.\nacceptance:\n  - Tests cover behavior.\nrisk: medium\n")
	if err := os.WriteFile(filepath.Join(taskPath, "00-spec.yaml"), validSpec, 0644); err != nil {
		t.Fatal(err)
	}
	payload, err := store.LoadArtifact(task, "00-spec.yaml")
	if err != nil {
		t.Fatalf("valid LoadArtifact failed: %v", err)
	}
	if spec, ok := payload.(map[string]any); !ok || spec["task_id"] != "FEAT-001" {
		t.Fatalf("unexpected spec payload: %#v", payload)
	}

	if _, err := store.LoadArtifact(task, "missing.yaml"); err == nil {
		t.Fatalf("expected missing artifact to fail")
	}

	if err := os.WriteFile(filepath.Join(taskPath, "notes.yaml"), []byte("hello: world\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadArtifact(task, "notes.yaml"); err == nil || !strings.Contains(err.Error(), "unsupported artifact path") {
		t.Fatalf("unsupported artifact error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(taskPath, "00-spec.yaml"), []byte("task_id: FEAT-001\nsummary: Broken\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadArtifact(task, "00-spec.yaml"); err == nil {
		t.Fatalf("expected invalid spec to fail schema validation")
	}
}

func TestTaskStoreSaveArtifactValidatesAndPreservesTrace(t *testing.T) {
	useSchemas(t)
	tmpDir := t.TempDir()
	store := NewTaskStore(tmpDir)
	taskPath := filepath.Join(tmpDir, ".ai", "tasks", "active", "FEAT-001")
	if err := os.MkdirAll(taskPath, 0755); err != nil {
		t.Fatal(err)
	}
	task := &TaskDirMatch{Path: taskPath, Location: "active"}

	validSpec := map[string]any{
		"task_id":    "FEAT-001",
		"summary":    "Valid spec",
		"goal":       "Exercise TaskStore save validation.",
		"invariants": []any{"Preserve invariants."},
		"acceptance": []any{"Tests cover behavior."},
		"risk":       "medium",
	}
	if _, err := store.SaveArtifact(task, "00-spec.yaml", validSpec); err != nil {
		t.Fatalf("valid SaveArtifact failed: %v", err)
	}
	invalidSpec := map[string]any{"task_id": "FEAT-001", "summary": "Broken"}
	if _, err := store.SaveArtifact(task, "00-spec.yaml", invalidSpec); err == nil {
		t.Fatalf("expected invalid spec save to fail")
	}
	persisted, err := LoadArtifactPayload(filepath.Join(taskPath, "00-spec.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if spec, ok := persisted.(map[string]any); !ok || spec["goal"] != validSpec["goal"] {
		t.Fatalf("invalid save overwrote existing spec: %#v", persisted)
	}

	trace := map[string]any{
		"task_id":           "FEAT-001",
		"summary":           "Trace initialized",
		"started_at":        "2026-01-01T00:00:00Z",
		"execution_mode":    "patch_only",
		"attempts":          []any{map[string]any{"phase": "execute", "result": "passed", "duration_s": 1.0}},
		"total_cost_usd":    0.0,
		"violations":        []any{},
		"context_overflows": []any{},
	}
	if _, err := store.SaveArtifact(task, "07-trace.json", trace); err != nil {
		t.Fatalf("valid trace save failed: %v", err)
	}
	appended := map[string]any{
		"task_id":           "FEAT-001",
		"summary":           "Trace initialized",
		"started_at":        "2026-01-01T00:00:00Z",
		"execution_mode":    "patch_only",
		"attempts":          []any{map[string]any{"phase": "execute", "result": "passed", "duration_s": 1.0}, map[string]any{"phase": "verify", "result": "passed", "duration_s": 2.0}},
		"total_cost_usd":    0.0,
		"violations":        []any{},
		"context_overflows": []any{},
	}
	if _, err := store.SaveArtifact(task, "07-trace.json", appended); err != nil {
		t.Fatalf("trace append failed: %v", err)
	}
	shortened := map[string]any{
		"task_id":           "FEAT-001",
		"summary":           "Trace initialized",
		"started_at":        "2026-01-01T00:00:00Z",
		"execution_mode":    "patch_only",
		"attempts":          []any{},
		"total_cost_usd":    0.0,
		"violations":        []any{},
		"context_overflows": []any{},
	}
	if _, err := store.SaveArtifact(task, "07-trace.json", shortened); err == nil || !strings.Contains(err.Error(), "append-only trace cannot remove") {
		t.Fatalf("shortened trace error = %v", err)
	}
	mutated := map[string]any{
		"task_id":           "FEAT-001",
		"summary":           "Trace initialized",
		"started_at":        "2026-01-01T00:00:00Z",
		"execution_mode":    "patch_only",
		"attempts":          []any{map[string]any{"phase": "execute", "result": "failed", "duration_s": 1.0}, map[string]any{"phase": "verify", "result": "passed", "duration_s": 2.0}},
		"total_cost_usd":    0.0,
		"violations":        []any{},
		"context_overflows": []any{},
	}
	if _, err := store.SaveArtifact(task, "07-trace.json", mutated); err == nil || !strings.Contains(err.Error(), "append-only trace cannot reorder or mutate") {
		t.Fatalf("mutated trace error = %v", err)
	}
}

func TestTaskStoreFindTaskParity(t *testing.T) {
	root := t.TempDir()
	store := NewTaskStore(root)
	mkSpec(t, root, "active", "LEGACY-999-slug", "FEAT-001")
	mkSpec(t, root, "active", "FEAT-001", "OTHER-001")
	m, err := store.FindTask("FEAT-001", "active")
	if err != nil || m == nil || filepath.Base(m.Path) != "LEGACY-999-slug" || m.Location != "active" {
		t.Fatalf("yaml tier = %#v, %v", m, err)
	}

	mkSpec(t, root, "active", "FEAT-002", "OTHER-002")
	m, err = store.FindTask("FEAT-002", "active")
	if err != nil || m == nil || filepath.Base(m.Path) != "FEAT-002" {
		t.Fatalf("exact tier = %#v, %v", m, err)
	}

	mkSpec(t, root, "inbox", "FEAT-003-a-new-spec", "FEAT-003-a")
	m, err = store.FindTask("FEAT-003", "inbox")
	if err != nil || m != nil {
		t.Fatalf("parent resolved to child = %#v, %v", m, err)
	}
	m, err = store.FindTask("FEAT-003-a", "inbox")
	if err != nil || m == nil || filepath.Base(m.Path) != "FEAT-003-a-new-spec" {
		t.Fatalf("child tier = %#v, %v", m, err)
	}

	mkSpec(t, root, "done", "FEAT-004-alpha", "OTHER-004")
	mkSpec(t, root, "done", "FEAT-004-beta", "OTHER-005")
	_, err = store.FindTask("FEAT-004", "done")
	if err == nil || !strings.Contains(err.Error(), "AMBIGUITY ERROR") || !strings.Contains(err.Error(), "done/FEAT-004-alpha") {
		t.Fatalf("ambiguity = %v", err)
	}

	m, err = store.FindTask("FEAT-999")
	if err != nil || m != nil {
		t.Fatalf("not found = %#v, %v", m, err)
	}
}

func TestMoveTask(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewTaskStore(tmpDir)

	taskDir := filepath.Join(tmpDir, ".ai", "tasks", "inbox", "FEAT-001")
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		t.Fatalf("setup err: %v", err)
	}

	task := &TaskDirMatch{Path: taskDir, Location: "inbox"}

	// Test no-op
	moved, err := store.MoveTask(task, "inbox")
	if err != nil {
		t.Errorf("no-op move failed: %v", err)
	}
	if moved == task {
		t.Errorf("MoveTask should return a new object even on no-op")
	}
	if moved.Location != "inbox" {
		t.Errorf("no-op location mismatch")
	}

	// Test invalid target
	if _, err := store.MoveTask(task, "invalid"); err == nil {
		t.Errorf("expected error for invalid target location")
	}

	// Test valid move
	moved, err = store.MoveTask(task, "active")
	if err != nil {
		t.Errorf("valid move failed: %v", err)
	}
	if moved.Location != "active" {
		t.Errorf("expected active location, got %q", moved.Location)
	}
	if !strings.HasSuffix(moved.Path, "active/FEAT-001") {
		t.Errorf("unexpected moved path: %q", moved.Path)
	}
	if _, err := os.Stat(moved.Path); os.IsNotExist(err) {
		t.Errorf("moved directory not found at %q", moved.Path)
	}
	if _, err := os.Stat(task.Path); err == nil {
		t.Errorf("original directory still exists at %q", task.Path)
	}

	// Test clobber protection
	clobberDir := filepath.Join(tmpDir, ".ai", "tasks", "done", "FEAT-001")
	if err := os.MkdirAll(clobberDir, 0755); err != nil {
		t.Fatalf("setup err: %v", err)
	}
	if _, err := store.MoveTask(moved, "done"); err == nil {
		t.Errorf("expected error when target already exists, got nil")
	}
}
