package capsule

import (
	"fmt"
	"os"
	"path/filepath"
)

// ScannedTask represents an archived task directory found during scanning.
type ScannedTask struct {
	TaskID string
	State  TaskState
	Dir    string
}

// ScanArchivedTasks walks the done/ and failed/ subdirectories under tasksRoot read-only.
// Missing done/ or failed/ subdirectories, or non-directory entries (files, broken symlinks),
// are recorded as warnings and skipped, never returning a function error.
// Returns a non-nil error only when tasksRoot is empty.
func ScanArchivedTasks(tasksRoot string) ([]ScannedTask, []string, error) {
	if tasksRoot == "" {
		return nil, nil, fmt.Errorf("tasksRoot cannot be empty")
	}

	var tasks []ScannedTask
	var warnings []string

	subdirs := []struct {
		state TaskState
		name  string
	}{
		{StateDone, "done"},
		{StateFailed, "failed"},
	}

	for _, sub := range subdirs {
		dirPath := filepath.Join(tasksRoot, sub.name)
		entries, err := os.ReadDir(dirPath)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("archived tasks directory %s does not exist or cannot be read: %v", dirPath, err))
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				warnings = append(warnings, fmt.Sprintf("skipping non-directory entry %s in %s", entry.Name(), dirPath))
				continue
			}

			tasks = append(tasks, ScannedTask{
				TaskID: entry.Name(),
				State:  sub.state,
				Dir:    filepath.Join(dirPath, entry.Name()),
			})
		}
	}

	return tasks, warnings, nil
}
