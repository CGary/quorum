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
