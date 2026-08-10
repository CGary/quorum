package modules

import (
	"path/filepath"
	"testing"

	"github.com/hsme/core/src/core/capsule"
)

func TestScanArchivedTasks_EmptyTasksRoot(t *testing.T) {
	_, _, err := capsule.ScanArchivedTasks("")
	if err == nil {
		t.Fatal("Expected error when tasksRoot is empty, got nil")
	}
}

func TestScanArchivedTasks_HappyPathAndTolerances(t *testing.T) {
	tasksRoot := filepath.Join("testdata", "capsule_scan")

	tasks, warnings, err := capsule.ScanArchivedTasks(tasksRoot)
	if err != nil {
		t.Fatalf("ScanArchivedTasks failed: %v", err)
	}

	if len(warnings) == 0 {
		t.Errorf("Expected warnings for stray file and broken symlink, got none")
	}

	taskMap := make(map[string]capsule.ScannedTask)
	for _, task := range tasks {
		taskMap[task.TaskID] = task
	}

	expectedTasks := []struct {
		taskID string
		state  capsule.TaskState
	}{
		{"HSME-777", capsule.StateDone},
		{"HSME-778", capsule.StateDone},
		{"HSME-780-empty", capsule.StateDone},
		{"HSME-779", capsule.StateFailed},
	}

	if len(tasks) != len(expectedTasks) {
		t.Errorf("Expected %d scanned tasks, got %d", len(expectedTasks), len(tasks))
	}

	for _, exp := range expectedTasks {
		st, ok := taskMap[exp.taskID]
		if !ok {
			t.Errorf("Expected task %s in scanned tasks, but was not found", exp.taskID)
			continue
		}
		if st.State != exp.state {
			t.Errorf("Expected task %s state to be %s, got %s", exp.taskID, exp.state, st.State)
		}
	}

	if _, ok := taskMap["not-a-task.txt"]; ok {
		t.Errorf("Stray non-directory file 'not-a-task.txt' should not be in scanned tasks")
	}
	if _, ok := taskMap["broken-symlink"]; ok {
		t.Errorf("Broken symlink 'broken-symlink' should not be in scanned tasks")
	}
}

func TestScanArchivedTasks_MissingSubdirectory(t *testing.T) {
	tempDir := t.TempDir()

	tasks, warnings, err := capsule.ScanArchivedTasks(tempDir)
	if err != nil {
		t.Fatalf("ScanArchivedTasks failed on missing done/failed subdirs: %v", err)
	}

	if len(tasks) != 0 {
		t.Errorf("Expected 0 tasks when done/failed subdirs are missing, got %d", len(tasks))
	}
	if len(warnings) != 2 {
		t.Errorf("Expected 2 warnings for missing done and failed subdirs, got %d", len(warnings))
	}
}
