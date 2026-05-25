package core

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var parentIDRE = regexp.MustCompile(`^[A-Z]+-[0-9]+$`)

type TaskDirMatch struct{ Path, Location string }

func ProjectRoot() (string, error) {
	if out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output(); err == nil {
		return strings.TrimSpace(string(out)), nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := cwd; ; dir = filepath.Dir(dir) {
		if st, err := os.Stat(filepath.Join(dir, ".git")); err == nil && (st.IsDir() || st.Mode().IsRegular()) {
			return dir, nil
		}
		if filepath.Dir(dir) == dir {
			return "", fmt.Errorf("project root not found")
		}
	}
}

func FindTaskDir(taskID string, locations []string) (*TaskDirMatch, error) {
	root, err := ProjectRoot()
	if err != nil {
		return nil, err
	}
	return FindTaskDirIn(root, taskID, locations)
}

func FindTaskDirIn(projectRoot, taskID string, locations []string) (*TaskDirMatch, error) {
	if len(locations) == 0 {
		locations = []string{"inbox", "active", "done", "failed"}
	}
	var yamlMatches, nameExact, namePrefix []TaskDirMatch
	for _, loc := range locations {
		entries, err := os.ReadDir(filepath.Join(projectRoot, ".ai", "tasks", loc))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			dir := filepath.Join(projectRoot, ".ai", "tasks", loc, entry.Name())
			if id, ok := readSpecTaskID(dir); ok && id == taskID {
				yamlMatches = append(yamlMatches, TaskDirMatch{dir, loc})
				continue
			}
			if entry.Name() == taskID {
				nameExact = append(nameExact, TaskDirMatch{dir, loc})
				continue
			}
			prefix := taskID + "-"
			if strings.HasPrefix(entry.Name(), prefix) && !(parentIDRE.MatchString(taskID) && isChildSuffixRest(entry.Name()[len(prefix):])) {
				namePrefix = append(namePrefix, TaskDirMatch{dir, loc})
			}
		}
	}
	matches := firstNonEmpty(yamlMatches, nameExact, namePrefix)
	if len(matches) > 1 {
		return nil, ambiguityError(taskID, matches)
	}
	if len(matches) == 0 {
		return nil, nil
	}
	return &matches[0], nil
}

func EnsureTraceAppendOnly(path string, existingPayload, newPayload any) error {
	existing, next := attempts(existingPayload), attempts(newPayload)
	if len(next) < len(existing) {
		return ArtifactValidationError{fmt.Sprintf("artifact=%s; field=$.attempts; reason=append-only trace cannot remove existing attempts", path)}
	}
	for i := range existing {
		if !sameJSON(existing[i], next[i]) {
			return ArtifactValidationError{fmt.Sprintf("artifact=%s; field=$.attempts; reason=append-only trace cannot reorder or mutate existing attempts", path)}
		}
	}
	return nil
}

func readSpecTaskID(dir string) (string, bool) {
	raw, err := os.ReadFile(filepath.Join(dir, "00-spec.yaml"))
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || !strings.HasPrefix(line, "task_id:") {
			continue
		}
		id := strings.Trim(strings.Split(strings.TrimSpace(strings.TrimPrefix(line, "task_id:")), " #")[0], "'\"")
		if id != "" {
			return id, true
		}
	}
	return "", false
}

func isChildSuffixRest(rest string) bool {
	return (len(rest) == 1 && rest[0] >= 'a' && rest[0] <= 'z') || (len(rest) >= 2 && rest[0] >= 'a' && rest[0] <= 'z' && rest[1] == '-')
}
func firstNonEmpty(groups ...[]TaskDirMatch) []TaskDirMatch {
	for _, g := range groups {
		if len(g) > 0 {
			return g
		}
	}
	return nil
}
func ambiguityError(taskID string, matches []TaskDirMatch) error {
	var b strings.Builder
	fmt.Fprintf(&b, "[!] AMBIGUITY ERROR: Multiple tasks match '%s':", taskID)
	for _, m := range matches {
		fmt.Fprintf(&b, "\n  - %s/%s", m.Location, filepath.Base(m.Path))
	}
	return fmt.Errorf("%s", b.String())
}
func attempts(payload any) []any {
	if obj, ok := payload.(map[string]any); ok {
		if items, ok := asSlice(obj["attempts"]); ok {
			return items
		}
	}
	return nil
}
func sameJSON(a, b any) bool {
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	return string(aj) == string(bj)
}
