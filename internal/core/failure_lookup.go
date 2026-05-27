package core

import (
	"os"
	"path/filepath"
	"strings"
)

type FailureOverlap struct {
	TaskID            string  `json:"task_id"`
	OverlapRatio      float64 `json:"overlap_ratio"`
	ValidationExcerpt string  `json:"validation_excerpt"`
}

func FindRelatedFailedTasks(draftBlueprint Blueprint, tasksDir string) ([]FailureOverlap, error) {
	failedDir := filepath.Join(tasksDir, "failed")
	entries, err := os.ReadDir(failedDir)
	if os.IsNotExist(err) {
		return []FailureOverlap{}, nil
	}
	if err != nil {
		return nil, err
	}

	var results []FailureOverlap
	draftFiles := make(map[string]bool)
	for _, f := range draftBlueprint.AffectedFiles {
		draftFiles[f] = true
	}

	if len(draftFiles) == 0 {
		return results, nil
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		taskPath := filepath.Join(failedDir, entry.Name())
		
		id, ok := readSpecTaskID(taskPath)
		if !ok {
			// fallback to parsing name
			parts := strings.Split(entry.Name(), "-")
			if len(parts) >= 2 {
				id = parts[0] + "-" + parts[1]
			} else {
				continue
			}
		}

		bpPayload, err := LoadArtifactPayload(filepath.Join(taskPath, "01-blueprint.yaml"))
		if err != nil {
			continue
		}
		
		bpMap, ok := bpPayload.(map[string]any)
		if !ok {
			continue
		}

		affectedAny, ok := asSlice(bpMap["affected_files"])
		if !ok {
			continue
		}

		failedFiles := make(map[string]bool)
		for _, fAny := range affectedAny {
			if fStr, ok := fAny.(string); ok {
				failedFiles[fStr] = true
			}
		}

		intersection := 0
		for f := range draftFiles {
			if failedFiles[f] {
				intersection++
			}
		}
		union := len(draftFiles)
		for f := range failedFiles {
			if !draftFiles[f] {
				union++
			}
		}

		if union == 0 {
			continue
		}

		ratio := float64(intersection) / float64(union)
		if ratio > 0.5 {
			excerpt := "No validation or review details found."
			
			// Try reading 05-validation.json
			if valPayload, err := LoadArtifactPayload(filepath.Join(taskPath, "05-validation.json")); err == nil {
				if valMap, ok := valPayload.(map[string]any); ok {
					if fc, ok := asSlice(valMap["failed_commands"]); ok && len(fc) > 0 {
						if fcMap, ok := fc[0].(map[string]any); ok {
							if out, ok := fcMap["stdout"].(string); ok && out != "" {
								excerpt = "Validation failed: " + truncate(out, 100)
							} else if errOut, ok := fcMap["stderr"].(string); ok && errOut != "" {
								excerpt = "Validation error: " + truncate(errOut, 100)
							}
						}
					}
				}
			} else if revPayload, err := LoadArtifactPayload(filepath.Join(taskPath, "06-review.json")); err == nil {
				if revMap, ok := revPayload.(map[string]any); ok {
					if notes, ok := revMap["notes"].(string); ok {
						excerpt = "Review failed: " + truncate(notes, 100)
					}
				}
			}

			results = append(results, FailureOverlap{
				TaskID:            id,
				OverlapRatio:      ratio,
				ValidationExcerpt: excerpt,
			})
		}
	}

	return results, nil
}

func truncate(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}

// asSlice is defined in task_manager.go, but it operates on `any`.
// If it's not exported, we can just duplicate its logic here or since they are in the same package `core`,
// we can use it directly if it's exported or just available in the package.
// Wait, in `task_manager.go` it's `asSlice`. Let's assume it's available since we're in the same package `core`.
// If `asSlice` is in `task_manager.go`, it's available to `failure_lookup.go`.
