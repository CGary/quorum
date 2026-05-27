package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindRelatedFailedTasks(t *testing.T) {
	tempDir := t.TempDir()
	failedDir := filepath.Join(tempDir, "failed")
	os.MkdirAll(failedDir, 0755)

	task1Dir := filepath.Join(failedDir, "F-99-test")
	os.MkdirAll(task1Dir, 0755)

	// Create 00-spec.yaml to provide the ID
	DumpArtifactPayload(filepath.Join(task1Dir, "00-spec.yaml"), map[string]any{
		"task_id": "F-99",
	})
	
	// Create blueprint with 2 overlapping files
	DumpArtifactPayload(filepath.Join(task1Dir, "01-blueprint.yaml"), map[string]any{
		"affected_files": []string{"a.go", "b.go"},
	})
	
	// Create validation
	DumpArtifactPayload(filepath.Join(task1Dir, "05-validation.json"), map[string]any{
		"failed_commands": []any{
			map[string]any{
				"stdout": "compiler error on line 42",
			},
		},
	})

	draft := Blueprint{
		AffectedFiles: []string{"a.go", "b.go", "c.go"},
	}

	// intersection: a, b (2)
	// union: a, b, c (3)
	// ratio: 2/3 = 0.66 > 0.50

	results, err := FindRelatedFailedTasks(draft, tempDir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	res := results[0]
	if res.TaskID != "F-99" {
		t.Errorf("expected F-99, got %s", res.TaskID)
	}
	if res.OverlapRatio < 0.66 || res.OverlapRatio > 0.67 {
		t.Errorf("expected ratio ~0.66, got %f", res.OverlapRatio)
	}
	if res.ValidationExcerpt != "Validation failed: compiler error on line 42" {
		t.Errorf("unexpected excerpt: %q", res.ValidationExcerpt)
	}
}

func TestFindRelatedFailedTasks_NoOverlap(t *testing.T) {
	tempDir := t.TempDir()
	failedDir := filepath.Join(tempDir, "failed")
	os.MkdirAll(failedDir, 0755)
	task1Dir := filepath.Join(failedDir, "F-99-test")
	os.MkdirAll(task1Dir, 0755)
	
	SaveArtifact(filepath.Join(task1Dir, "01-blueprint.yaml"), map[string]any{
		"affected_files": []string{"x.go", "y.go"},
	})

	draft := Blueprint{AffectedFiles: []string{"a.go", "b.go"}}
	results, err := FindRelatedFailedTasks(draft, tempDir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}
