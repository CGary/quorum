package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateTaskID(t *testing.T) {
	valids := []string{"FEAT-123", "TASK-001-a", "BUG-999-z"}
	invalids := []string{"feat-123", "FEAT-123-A", "FEAT-123-ab", "../../", "FEAT-123/a", "FEAT-123.."}

	for _, v := range valids {
		if err := ValidateTaskID(v); err != nil {
			t.Errorf("expected %q to be valid, got err: %v", v, err)
		}
	}
	for _, inv := range invalids {
		if err := ValidateTaskID(inv); err == nil {
			t.Errorf("expected %q to be invalid, got nil err", inv)
		}
	}
}

func TestQueryTasksAndDetails(t *testing.T) {
	tmpDir := t.TempDir()

	// Set up folder structure
	// .ai/tasks/{inbox,active,done,failed}
	for _, loc := range []string{"inbox", "active", "done", "failed"} {
		if err := os.MkdirAll(filepath.Join(tmpDir, ".ai", "tasks", loc), 0755); err != nil {
			t.Fatalf("failed to create dirs: %v", err)
		}
	}

	// 1. Create a task in inbox without spec (fallback ID / summary check)
	inboxTaskDir := filepath.Join(tmpDir, ".ai", "tasks", "inbox", "TASK-100-some-slug")
	os.MkdirAll(inboxTaskDir, 0755)

	// 2. Create a parent task in active
	parentTaskDir := filepath.Join(tmpDir, ".ai", "tasks", "active", "FEAT-001-parent")
	os.MkdirAll(parentTaskDir, 0755)
	parentSpec := map[string]any{
		"task_id": "FEAT-001",
		"summary": "This is parent task",
		"goal":    "Accomplish parent goals",
		"risk":    "high",
		"decomposition": []any{
			map[string]any{"child_id": "FEAT-001-a"},
			map[string]any{"child_id": "FEAT-001-b"},
		},
	}
	writeJSONOrYAML(t, filepath.Join(parentTaskDir, "00-spec.yaml"), parentSpec)

	// 3. Create child A in active (failed state)
	childATaskDir := filepath.Join(tmpDir, ".ai", "tasks", "failed", "FEAT-001-a-child")
	os.MkdirAll(childATaskDir, 0755)
	childASpec := map[string]any{
		"task_id":     "FEAT-001-a",
		"parent_task": "FEAT-001",
		"summary":     "Child A summary",
		"goal":        "Goal of child A",
	}
	writeJSONOrYAML(t, filepath.Join(childATaskDir, "00-spec.yaml"), childASpec)

	// Mock validation and review for child A
	writeJSONOrYAML(t, filepath.Join(childATaskDir, "05-validation.json"), map[string]any{"status": "failed", "error": "test failure"})
	writeJSONOrYAML(t, filepath.Join(childATaskDir, "06-review.json"), map[string]any{"verdict": "rejected", "comments": "code issues"})

	// 4. Create child B in done
	childBTaskDir := filepath.Join(tmpDir, ".ai", "tasks", "done", "FEAT-001-b-child")
	os.MkdirAll(childBTaskDir, 0755)
	childBSpec := map[string]any{
		"task_id":     "FEAT-001-b",
		"parent_task": "FEAT-001",
		"summary":     "Child B summary",
		"goal":        "Goal of child B",
	}
	writeJSONOrYAML(t, filepath.Join(childBTaskDir, "00-spec.yaml"), childBSpec)
	writeJSONOrYAML(t, filepath.Join(childBTaskDir, "02-contract.yaml"), map[string]any{
		"task_id": "FEAT-001-b",
		"summary": "Child B contract summary",
		"goal":    "Child B contract goal",
		"touch":   []string{"file1.go"},
		"verify": map[string]any{
			"commands": []string{"go test ./..."},
		},
		"forbid":       map[string]any{"files": []string{}, "behaviors": []string{}},
		"limits":       map[string]any{},
		"execution":    map[string]any{},
		"retry_policy": map[string]any{},
	})
	writeJSONOrYAML(t, filepath.Join(childBTaskDir, "07-trace.json"), map[string]any{
		"summary":        "Trace summary",
		"total_cost_usd": 0.15,
		"attempts": []any{
			map[string]any{"attempt": 1, "status": "success"},
		},
	})

	// Set up worktrees mock
	os.MkdirAll(filepath.Join(tmpDir, "worktrees", "FEAT-001"), 0755)

	// Query All Tasks
	res, err := QueryTasks(TaskListOptions{
		ProjectRoot: tmpDir,
	})
	if err != nil {
		t.Fatalf("QueryTasks failed: %v", err)
	}

	// Verify Counts
	if res.Counts.Inbox != 1 || res.Counts.Active != 1 || res.Counts.Failed != 1 || res.Counts.Done != 1 {
		t.Errorf("Counts mismatch: %+v", res.Counts)
	}

	// Verify items count
	if len(res.Items) != 4 {
		t.Errorf("expected 4 items, got %d", len(res.Items))
	}

	// Find parent task in response
	var parentItem *TaskListItem
	for i := range res.Items {
		if res.Items[i].ID == "FEAT-001" {
			parentItem = &res.Items[i]
		}
	}
	if parentItem == nil {
		t.Fatalf("parent task FEAT-001 not found in list")
	}

	// Parent state check (one failed, one done -> partial)
	if parentItem.ParentState != "partial" {
		t.Errorf("expected parentState to be partial, got %q", parentItem.ParentState)
	}
	if len(parentItem.Children) != 2 {
		t.Errorf("expected parent to have 2 children refs, got %d", len(parentItem.Children))
	}
	if !parentItem.WorktreePresent {
		t.Errorf("expected worktree to be present for FEAT-001")
	}

	// Query with parent_task filter
	resChildren, err := QueryTasks(TaskListOptions{
		ProjectRoot: tmpDir,
		ParentTask:  "FEAT-001",
	})
	if err != nil {
		t.Fatalf("QueryTasks failed: %v", err)
	}
	if len(resChildren.Items) != 2 {
		t.Errorf("expected 2 children, got %d", len(resChildren.Items))
	}

	// Query with search text
	resSearch, err := QueryTasks(TaskListOptions{
		ProjectRoot: tmpDir,
		Query:       "Child A",
	})
	if err != nil {
		t.Fatalf("QueryTasks failed: %v", err)
	}
	if len(resSearch.Items) != 1 || resSearch.Items[0].ID != "FEAT-001-a" {
		t.Errorf("expected only child A, got %+v", resSearch.Items)
	}

	// Query with pagination
	resPage, err := QueryTasks(TaskListOptions{
		ProjectRoot: tmpDir,
		Limit:       2,
		Offset:      1,
	})
	if err != nil {
		t.Fatalf("QueryTasks failed: %v", err)
	}
	if len(resPage.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(resPage.Items))
	}

	// Test GetTaskDetailIn for Child B (has contract & trace)
	detailB, err := GetTaskDetailIn(tmpDir, "FEAT-001-b")
	if err != nil {
		t.Fatalf("GetTaskDetailIn failed: %v", err)
	}
	if detailB == nil {
		t.Fatalf("expected child B detail, got nil")
	}

	if detailB.Contract.Summary != "Child B contract summary" || len(detailB.Contract.Touch) != 1 || detailB.Contract.Touch[0] != "file1.go" {
		t.Errorf("Contract summary mismatch: %+v", detailB.Contract)
	}
	if detailB.Trace.Summary != "Trace summary" || detailB.Trace.AttemptsCount != 1 || detailB.Trace.TotalCostUSD != 0.15 {
		t.Errorf("Trace summary mismatch: %+v", detailB.Trace)
	}

	// Test invalid ID detail query
	_, err = GetTaskDetailIn(tmpDir, "invalid-id-format")
	if err == nil {
		t.Errorf("expected error querying invalid ID format")
	}

	// Test path traversal query rejection (via ValidateTaskID, which doesn't accept traversal characters anyway)
	_, err = GetTaskDetailIn(tmpDir, "FEAT-001/../../etc")
	if err == nil {
		t.Errorf("expected error querying traversal ID")
	}
}

func writeJSONOrYAML(t *testing.T, path string, data any) {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}
	if err := os.WriteFile(path, b, 0644); err != nil {
		t.Fatalf("failed to write file %s: %v", path, err)
	}
}
