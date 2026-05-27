package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyzeParentChildCoverage_NotDecomposed(t *testing.T) {
	tempDir := t.TempDir()
	taskDir := filepath.Join(tempDir, ".ai", "tasks", "inbox", "F-100")
	os.MkdirAll(taskDir, 0755)
	
	specPath := filepath.Join(taskDir, "00-spec.yaml")
	DumpArtifactPayload(specPath, map[string]any{
		"task_id": "F-100",
		"summary": "something",
		"goal": "this is a valid goal",
		"invariants": []string{"inv 1"},
		"acceptance": []string{"acc 1"},
		"risk": "low",
	})

	res := AnalyzeParentChildCoverage(specPath, filepath.Join(tempDir, ".ai", "tasks"))
	if res.Applies {
		t.Errorf("expected applies=false")
	}
	if res.Status != "not_decomposed" {
		t.Errorf("expected not_decomposed, got %q", res.Status)
	}
}

func TestAnalyzeParentChildCoverage_MissingChild(t *testing.T) {
	tempDir := t.TempDir()
	taskDir := filepath.Join(tempDir, ".ai", "tasks", "inbox", "F-100")
	os.MkdirAll(taskDir, 0755)
	
	specPath := filepath.Join(taskDir, "00-spec.yaml")
	DumpArtifactPayload(specPath, map[string]any{
		"task_id": "F-100",
		"summary": "something",
		"goal": "this is a valid goal",
		"invariants": []string{"inv 1"},
		"acceptance": []string{"acc 1"},
		"risk": "low",
		"decomposition": []any{
			map[string]any{"child_id": "F-100-a", "summary": "child a"},
		},
	})

	res := AnalyzeParentChildCoverage(specPath, filepath.Join(tempDir, ".ai", "tasks"))
	if !res.Applies {
		t.Errorf("expected applies=true")
	}
	if res.Status != "issues_found" {
		t.Errorf("expected issues_found, got %q", res.Status)
	}
	if len(res.Findings) == 0 {
		t.Fatalf("expected findings")
	}
	
	hasMissingChild := false
	for _, f := range res.Findings {
		if f.Severity == "high" && f.Artifact == "00-spec.yaml.decomposition[F-100-a]" {
			hasMissingChild = true
		}
	}
	if !hasMissingChild {
		t.Errorf("missing child not reported in findings")
	}
}

func TestAnalyzeParentChildCoverage_Pass(t *testing.T) {
	tempDir := t.TempDir()
	taskDir := filepath.Join(tempDir, ".ai", "tasks", "inbox", "F-100")
	childDir := filepath.Join(tempDir, ".ai", "tasks", "inbox", "F-100-a")
	os.MkdirAll(taskDir, 0755)
	os.MkdirAll(childDir, 0755)
	
	specPath := filepath.Join(taskDir, "00-spec.yaml")
	DumpArtifactPayload(specPath, map[string]any{
		"task_id": "F-100",
		"summary": "something",
		"goal": "this is a valid goal",
		"invariants": []string{"inv 1"},
		"acceptance": []string{"acc 1"},
		"risk": "low",
		"decomposition": []any{
			map[string]any{"child_id": "F-100-a", "summary": "child a"},
		},
	})

	childSpecPath := filepath.Join(childDir, "00-spec.yaml")
	DumpArtifactPayload(childSpecPath, map[string]any{
		"task_id": "F-100-a",
		"summary": "child a",
		"goal": "this is a valid goal",
		"parent_task": "F-100",
		"invariants": []string{"inv 1"},
		"acceptance": []string{"acc 1"},
		"risk": "low",
	})

	res := AnalyzeParentChildCoverage(specPath, filepath.Join(tempDir, ".ai", "tasks"))
	if res.Status != "pass" {
		t.Errorf("expected pass, got %q", res.Status)
	}
	if len(res.Findings) > 0 {
		t.Errorf("expected no findings, got: %v", res.Findings)
	}
}
