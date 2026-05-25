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

	"gopkg.in/yaml.v3"
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

func LoadArtifactPayload(path string) (any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var payload any
	if filepath.Ext(path) == ".json" {
		if err := json.Unmarshal(b, &payload); err != nil {
			return nil, err
		}
	} else {
		if err := yaml.Unmarshal(b, &payload); err != nil {
			return nil, err
		}
	}
	return payload, nil
}

func DumpArtifactPayload(path string, payload any) error {
	var b []byte
	var err error
	if filepath.Ext(path) == ".json" {
		b, err = json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		b = append(b, '\n')
	} else {
		b, err = yaml.Marshal(payload)
		if err != nil {
			return err
		}
	}
	return os.WriteFile(path, b, 0644)
}

func SaveArtifact(artifactPath string, payload any) (string, error) {
	var existingPayload any
	if _, err := os.Stat(artifactPath); err == nil {
		existingPayload, _ = LoadArtifactPayload(artifactPath)
	}

	if filepath.Base(artifactPath) == "07-trace.json" && existingPayload != nil {
		if err := EnsureTraceAppendOnly(artifactPath, existingPayload, payload); err != nil {
			return "", err
		}
	}

	if err := ValidateArtifact(artifactPath, payload); err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(artifactPath), 0755); err != nil {
		return "", err
	}
	if err := DumpArtifactPayload(artifactPath, payload); err != nil {
		return "", err
	}
	return artifactPath, nil
}

func ConsumeFeedback(taskDir string) (bool, error) {
	feedbackPath := filepath.Join(taskDir, "feedback.json")
	if _, err := os.Stat(feedbackPath); os.IsNotExist(err) {
		return false, nil
	}
	if err := os.Remove(feedbackPath); err != nil {
		return false, err
	}
	return true, nil
}

func InitializeSpecify(taskID string) (string, error) {
	if taskID == "" {
		return "", fmt.Errorf("task ID is required")
	}
	taskDir, err := FindTaskDir(taskID, []string{"inbox", "active", "done", "failed"})
	if err != nil {
		return "", err
	}
	if taskDir != nil {
		fmt.Printf("[!] Task %s already exists in %s/.\n", taskID, taskDir.Location)
		return taskDir.Path, nil
	}
	
	root, err := ProjectRoot()
	if err != nil {
		return "", err
	}
	dirPath := filepath.Join(root, ".ai", "tasks", "inbox", taskID+"-new-spec")
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return "", err
	}
	specPath := filepath.Join(dirPath, "00-spec.yaml")
	spec := map[string]any{
		"task_id": taskID,
		"summary": "Draft spec; fill goal, invariants, and acceptance before blueprint.",
		"goal": "TODO: define the feature goal.",
		"invariants": []any{"TODO: define invariant."},
		"acceptance": []any{"TODO: define acceptance criterion."},
		"risk": "medium",
	}
	_, err = SaveArtifact(specPath, spec)
	return dirPath, err
}

func PrepareBlueprint(taskID string) (string, error) {
	taskDir, err := FindTaskDir(taskID, []string{"inbox"})
	if err != nil {
		return "", err
	}
	if taskDir == nil {
		activeDir, err := FindTaskDir(taskID, []string{"active"})
		if err != nil {
			return "", err
		}
		if activeDir != nil {
			fmt.Printf("[*] Task %s is already in active.\n", taskID)
			return activeDir.Path, nil
		}
		return "", fmt.Errorf("Task %s not found in inbox.", taskID)
	}
	
	root, err := ProjectRoot()
	if err != nil {
		return "", err
	}
	activePath := filepath.Join(root, ".ai", "tasks", "active", filepath.Base(taskDir.Path))
	if err := os.MkdirAll(filepath.Dir(activePath), 0755); err != nil {
		return "", err
	}
	if err := os.Rename(taskDir.Path, activePath); err != nil {
		return "", err
	}
	return activePath, nil
}

func SplitTask(parentID string) error {
	if !parentIDRE.MatchString(parentID) {
		fmt.Printf("[!] '%s' is not a valid parent task ID (expected '<PREFIX>-<NUMBER>', e.g. FEAT-001).\n", parentID)
		return nil
	}
	parentDir, err := FindTaskDir(parentID, nil)
	if err != nil {
		return err
	}
	if parentDir == nil {
		fmt.Printf("[!] Parent task %s not found.\n", parentID)
		return nil
	}
	if parentDir.Location != "active" {
		fmt.Printf("[!] Parent task %s must be in active/ before splitting (currently in %s/).\n", parentID, parentDir.Location)
		return nil
	}
	specPath := filepath.Join(parentDir.Path, "00-spec.yaml")
	if _, err := os.Stat(specPath); os.IsNotExist(err) {
		fmt.Printf("[!] Parent %s has no 00-spec.yaml.\n", parentID)
		return nil
	}
	payload, err := LoadArtifactPayload(specPath)
	if err != nil {
		return err
	}
	parentSpec, ok := payload.(map[string]any)
	if !ok {
		parentSpec = map[string]any{}
	}
	if parentSpec["parent_task"] != nil {
		fmt.Printf("[!] Task %s is already a child task; recursive decomposition is not supported.\n", parentID)
		return nil
	}
	
	decompObj := parentSpec["decomposition"]
	if decompObj == nil {
		fmt.Printf("[!] Parent task %s has no 'decomposition' field in 00-spec.yaml.\n", parentID)
		return nil
	}
	decomp, ok := asSlice(decompObj)
	if !ok {
		return nil
	}
	
	root, err := ProjectRoot()
	if err != nil {
		return err
	}
	
	letters := "abcdefghijklmnopqrstuvwxyz"
	for i, itemAny := range decomp {
		if i >= len(letters) {
			break
		}
		item, ok := itemAny.(map[string]any)
		if !ok {
			continue
		}
		childID := fmt.Sprintf("%s-%c", parentID, letters[i])
		childName := childID
		if slug, ok := item["slug"].(string); ok && slug != "" {
			childName = childID + "-" + slug
		}
		
		childDir := filepath.Join(root, ".ai", "tasks", "inbox", childName)
		if _, err := os.Stat(childDir); err == nil {
			fmt.Printf("[-] Child task %s already exists, skipping.\n", childID)
			continue
		}
		
		if err := os.MkdirAll(childDir, 0755); err != nil {
			return err
		}
		
		childSpec := map[string]any{
			"task_id": childID,
			"summary": item["summary"],
			"goal": "Subset of " + parentID + ": " + fmt.Sprintf("%v", item["summary"]),
			"invariants": parentSpec["invariants"],
			"acceptance": parentSpec["acceptance"],
			"risk": parentSpec["risk"],
			"parent_task": parentID,
			"non_goals": parentSpec["non_goals"],
			"constraints": parentSpec["constraints"],
		}
		if deps := item["depends_on"]; deps != nil {
			childSpec["depends_on"] = deps
		}
		
		childSpecPath := filepath.Join(childDir, "00-spec.yaml")
		if _, err := SaveArtifact(childSpecPath, childSpec); err != nil {
			return err
		}
		fmt.Printf("[+] Materialized %s in inbox/.\n", childID)
	}
	return nil
}

func ListTasks() error {
	root, err := ProjectRoot()
	if err != nil {
		return err
	}
	locations := []string{"active", "inbox", "done", "failed"}
	for _, loc := range locations {
		entries, err := os.ReadDir(filepath.Join(root, ".ai", "tasks", loc))
		if os.IsNotExist(err) || err != nil {
			continue
		}
		hasPrinted := false
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if !hasPrinted {
				fmt.Printf("\n[%s]\n", strings.ToUpper(loc))
				hasPrinted = true
			}
			dir := filepath.Join(root, ".ai", "tasks", loc, entry.Name())
			id, _ := readSpecTaskID(dir)
			if id == "" {
				id = "???"
			}
			fmt.Printf("  %s (%s)\n", entry.Name(), id)
		}
	}
	return nil
}

func ShowStatus(taskID string) error {
	taskDir, err := FindTaskDir(taskID, nil)
	if err != nil {
		return err
	}
	if taskDir == nil {
		fmt.Printf("[!] Task %s not found.\n", taskID)
		return nil
	}
	fmt.Printf("Task ID: %s\n", taskID)
	fmt.Printf("Location: %s/\n", taskDir.Location)
	fmt.Printf("Directory: %s\n", filepath.Base(taskDir.Path))
	
	artifacts := []string{"00-spec.yaml", "01-blueprint.yaml", "02-contract.yaml", "04-implementation-log.yaml", "05-validation.json", "06-review.json", "07-trace.json", "feedback.json"}
	fmt.Printf("\nArtifacts:\n")
	for _, a := range artifacts {
		if _, err := os.Stat(filepath.Join(taskDir.Path, a)); err == nil {
			fmt.Printf("  [x] %s\n", a)
		} else {
			fmt.Printf("  [ ] %s\n", a)
		}
	}
	return nil
}
