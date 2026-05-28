package core

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

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
	locations := []string{"inbox", "active", "done", "failed"}
	for _, loc := range locations {
		locPath := filepath.Join(root, ".ai", "tasks", loc)
		entries, err := os.ReadDir(locPath)
		if os.IsNotExist(err) || err != nil {
			continue
		}
		var taskDirs []string
		for _, entry := range entries {
			if entry.IsDir() {
				taskDirs = append(taskDirs, entry.Name())
			}
		}
		sort.Strings(taskDirs)

		for _, taskDirName := range taskDirs {
			dir := filepath.Join(locPath, taskDirName)
			id, _ := readSpecTaskID(dir)
			if id == "" {
				parts := strings.Split(taskDirName, "-")
				if len(parts) >= 3 && len(parts[2]) == 1 && parts[2] >= "a" && parts[2] <= "z" {
					id = strings.Join(parts[:3], "-")
				} else if len(parts) >= 2 {
					id = strings.Join(parts[:2], "-")
				} else {
					id = taskDirName
				}
			}

			summary := ""
			var spec map[string]any
			for _, artifact := range []string{"00-spec.yaml", "01-blueprint.yaml", "02-contract.yaml", "07-trace.json"} {
				path := filepath.Join(dir, artifact)
				if _, err := os.Stat(path); err == nil {
					if payload, err := LoadArtifactPayload(path); err == nil {
						if data, ok := payload.(map[string]any); ok {
							if artifact == "00-spec.yaml" {
								spec = data
							}
							if s, ok := data["summary"].(string); ok && summary == "" {
								summary = s
							}
						}
					} else {
						summary = "<unreadable summary>"
					}
				}
				if summary != "" && spec != nil {
					break
				}
			}
			
			stateMarker := ""
			if spec != nil && spec["decomposition"] != nil {
				state := DeriveParentState(spec)
				stateMarker = fmt.Sprintf(" [%s]", state)
			}

			fmt.Printf("%-6s %-14s %s%s\n", loc, id, summary, stateMarker)
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
	
	tracePath := filepath.Join(taskDir.Path, "07-trace.json")
	if _, err := os.Stat(tracePath); err == nil {
		if payload, err := LoadArtifactPayload(tracePath); err == nil {
			if trace, ok := payload.(map[string]any); ok {
				if summary, ok := trace["summary"].(string); ok {
					fmt.Printf("Summary: %s\n", summary)
				}
				if cost, ok := trace["total_cost_usd"].(float64); ok {
					fmt.Printf("Cost: $%.3f\n", cost)
				}
			}
		}
	}

	artifacts := []string{"00-spec.yaml", "01-blueprint.yaml", "02-contract.yaml", "04-implementation-log.yaml", "05-validation.json", "06-review.json", "07-trace.json", "feedback.json"}
	fmt.Printf("\nArtifacts:\n")
	for _, a := range artifacts {
		if _, err := os.Stat(filepath.Join(taskDir.Path, a)); err == nil {
			fmt.Printf("  [x] %s\n", a)
		} else {
			fmt.Printf("  [ ] %s\n", a)
		}
	}

	root, _ := ProjectRoot()
	worktreePath := filepath.Join(root, "worktrees", taskID)
	worktreeStatus := "Missing"
	if _, err := os.Stat(worktreePath); err == nil {
		worktreeStatus = "Present"
	}
	fmt.Printf("- worktree: %s\n", worktreeStatus)

	specPath := filepath.Join(taskDir.Path, "00-spec.yaml")
	if _, err := os.Stat(specPath); err == nil {
		if payload, err := LoadArtifactPayload(specPath); err == nil {
			if spec, ok := payload.(map[string]any); ok {
				if p, ok := spec["parent_task"].(string); ok && p != "" {
					fmt.Printf("- parent_task: %s\n", p)
					if deps, ok := asSlice(spec["depends_on"]); ok && len(deps) > 0 {
						var depStrs []string
						for _, d := range deps {
							depStrs = append(depStrs, fmt.Sprintf("%v", d))
						}
						fmt.Printf("- depends_on: %s\n", strings.Join(depStrs, ", "))
					}
				}
				if decompObj, ok := spec["decomposition"]; ok {
					if decomp, ok := decompObj.([]any); ok && len(decomp) > 0 {
						var children []string
						for _, entryAny := range decomp {
							if entry, ok := entryAny.(map[string]any); ok {
								if childID, ok := entry["child_id"].(string); ok && childID != "" {
									children = append(children, childID)
								} else {
									children = append(children, "?")
								}
							} else {
								children = append(children, "?")
							}
						}
						fmt.Printf("- decomposition (children): %s\n", strings.Join(children, ", "))
						for _, childID := range children {
							loc := "missing"
							if c, err := FindTaskDir(childID, nil); err == nil && c != nil {
								loc = c.Location
							}
							fmt.Printf("  - %s: %s\n", childID, loc)
						}
						fmt.Printf("- parent_state: %s\n", DeriveParentState(spec))
					}
				}
			}
		}
	}

	return nil
}

func GetBaseBranch() string {
	out, err := exec.Command("git", "symbolic-ref", "refs/remotes/origin/HEAD").Output()
	if err == nil {
		parts := strings.Split(strings.TrimSpace(string(out)), "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}
	for _, b := range []string{"main", "master", "develop", "trunk"} {
		out, err := exec.Command("git", "show-ref", "--verify", "refs/heads/"+b).Output()
		if err == nil && len(out) > 0 {
			return b
		}
	}
	out, err = exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	return "main"
}

func StartTask(taskID string) {
	fmt.Printf("[*] Starting task %s...\n", taskID)
	taskDir, err := FindTaskDir(taskID, []string{"active", "inbox"})
	if err != nil || taskDir == nil {
		fmt.Printf("[!] Task %s not found.\n", taskID)
		return
	}
	contractPath := filepath.Join(taskDir.Path, "02-contract.yaml")
	if _, err := os.Stat(contractPath); os.IsNotExist(err) {
		fmt.Printf("[!] Contract (02-contract.yaml) not found for %s.\n", taskID)
		fmt.Printf("[!] Please run 'quorum task blueprint %s' first.\n", taskID)
		return
	}
	contract, err := LoadArtifactPayload(contractPath)
	if err != nil {
		fmt.Printf("[!] Contract validation failed for %s: %v\n", taskID, err)
		return
	}
	if err := ValidateArtifact(contractPath, contract); err != nil {
		fmt.Printf("[!] Contract validation failed for %s: %v\n", taskID, err)
		return
	}
	if taskDir.Location == "inbox" {
		root, _ := ProjectRoot()
		activePath := filepath.Join(root, ".ai", "tasks", "active", filepath.Base(taskDir.Path))
		os.MkdirAll(filepath.Dir(activePath), 0755)
		fmt.Printf("[*] Moving task from inbox to active...\n")
		os.Rename(taskDir.Path, activePath)
		taskDir.Path = activePath
		taskDir.Location = "active"
	}

	root, _ := ProjectRoot()
	worktreePath := filepath.Join(root, "worktrees", taskID)
	branchName := "ai/" + taskID
	baseBranch := GetBaseBranch()

	if _, err := os.Stat(worktreePath); err == nil {
		fmt.Printf("[*] Worktree for %s already exists.\n", taskID)
	} else {
		fmt.Printf("[*] Creating worktree in %s (base: %s)...\n", worktreePath, baseBranch)
		cmd := exec.Command("git", "worktree", "add", worktreePath, "-b", branchName, baseBranch)
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Printf("[!] Error creating worktree: %s\n", strings.TrimSpace(string(out)))
			return
		}
	}

	logPath := filepath.Join(taskDir.Path, "04-implementation-log.yaml")
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		summary := "Implementation log initialized."
		if c, ok := contract.(map[string]any); ok {
			if s, ok := c["summary"].(string); ok {
				summary = s
			}
		}
		log := map[string]any{
			"task_id": taskID,
			"summary": summary,
			"entries": []any{},
		}
		if _, err := SaveArtifact(logPath, log); err != nil {
			fmt.Printf("[!] Error initializing implementation log: %v\n", err)
			return
		}
	}

	tracePath := filepath.Join(taskDir.Path, "07-trace.json")
	if _, err := os.Stat(tracePath); os.IsNotExist(err) {
		summary := "Trace initialized for task."
		execMode := "patch_only"
		if c, ok := contract.(map[string]any); ok {
			if s, ok := c["summary"].(string); ok {
				summary = s
			}
			if ex, ok := c["execution"].(map[string]any); ok {
				if m, ok := ex["mode"].(string); ok {
					execMode = m
				}
			}
		}
		trace := map[string]any{
			"task_id":           taskID,
			"summary":           summary,
			"started_at":        time.Now().UTC().Format(time.RFC3339Nano)[:19] + "Z",
			"execution_mode":    execMode,
			"attempts":          []any{},
			"total_cost_usd":    0.0,
			"violations":        []any{},
			"context_overflows": []any{},
		}
		SaveArtifact(tracePath, trace)
	}
	fmt.Printf("[+] Task %s initialized and worktree ready.\n", taskID)
}

func WorktreeDirtyPaths(worktreePath string) []string {
	out, err := exec.Command("git", "-C", worktreePath, "status", "--porcelain").CombinedOutput()
	if err != nil { return nil }
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" { continue }
		path := strings.TrimSpace(line[2:])
		if strings.Contains(path, " -> ") {
			parts := strings.Split(path, " -> ")
			path = parts[len(parts)-1]
		}
		paths = append(paths, path)
	}
	return paths
}

func IsWorktreeDirty(worktreePath string) bool {
	return len(WorktreeDirtyPaths(worktreePath)) > 0
}

func SaveWorktreeChanges(worktreePath, taskID string) (bool, string) {
	root, err := ProjectRoot()
	if err != nil { return false, err.Error() }
	stashDir := filepath.Join(root, "worktrees", ".stash")
	if err := os.MkdirAll(stashDir, 0755); err != nil { return false, err.Error() }
	patchPath := filepath.Join(stashDir, fmt.Sprintf("%s-%s.patch", taskID, time.Now().UTC().Format("20060102T150405Z")))
	_ = exec.Command("git", "-C", worktreePath, "add", "-N", ".").Run()
	out, err := exec.Command("git", "-C", worktreePath, "diff", "--binary", "HEAD").CombinedOutput()
	if err != nil { return false, string(out) }
	if len(out) == 0 { return false, "no patch content produced" }
	if err := os.WriteFile(patchPath, out, 0644); err != nil { return false, err.Error() }
	return true, patchPath
}

func appendCleanupTrace(taskDirPath, phase, result, notes string) {
	tracePath := filepath.Join(taskDirPath, "07-trace.json")
	payload, err := LoadArtifactPayload(tracePath)
	if err != nil { return }
	trace, ok := payload.(map[string]any)
	if !ok { return }
	items, _ := asSlice(trace["attempts"])
	items = append(items, map[string]any{"phase": phase, "result": result, "duration_s": 0.0, "notes": notes})
	trace["attempts"] = items
	_, _ = SaveArtifact(tracePath, trace)
}

func BranchExists(branchName string) bool {
	root, _ := ProjectRoot()
	err := exec.Command("git", "-C", root, "show-ref", "--verify", "refs/heads/"+branchName).Run()
	return err == nil
}

func DeleteBranchIfMerged(branchName, baseBranch string) bool {
	if !BranchExists(branchName) {
		fmt.Printf("[*] Branch %s is absent; skipping local branch cleanup.\n", branchName)
		return false
	}
	root, _ := ProjectRoot()
	err := exec.Command("git", "-C", root, "merge-base", "--is-ancestor", branchName, baseBranch).Run()
	if err == nil {
		out, err2 := exec.Command("git", "-C", root, "branch", "-d", branchName).CombinedOutput()
		if err2 == nil {
			fmt.Printf("[+] Deleted merged local branch %s.\n", branchName)
			return true
		}
		fmt.Printf("[!] Could not delete merged local branch %s: %s\n", branchName, strings.TrimSpace(string(out)))
		return false
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		fmt.Printf("[!] Preserving local branch %s; it has commits not merged into %s.\n", branchName, baseBranch)
		fmt.Printf("    After merging, delete it manually with: git branch -d %s\n", branchName)
		return false
	}
	fmt.Printf("[!] Could not determine whether %s is merged into %s; preserving branch. \n", branchName, baseBranch)
	return false
}

func CleanTask(taskID string, force, stash bool) {
	if force && stash {
		fmt.Printf("[!] --force and --stash are mutually exclusive. Pick one: --force discards changes, --stash saves them to a patch.\n")
		return
	}
	
	taskDir, err := FindTaskDir(taskID, []string{"active", "done", "failed"})
	if err != nil || taskDir == nil {
		fmt.Printf("[!] Task %s not found.\n", taskID)
		return
	}
	
	specPath := filepath.Join(taskDir.Path, "00-spec.yaml")
	var parentID string
	if taskDir.Location == "active" {
		if _, err := os.Stat(specPath); err == nil {
			if payload, err := LoadArtifactPayload(specPath); err == nil {
				if spec, ok := payload.(map[string]any); ok {
					if p, ok := spec["parent_task"].(string); ok {
						parentID = p
					}
					if decompObj, ok := spec["decomposition"]; ok {
						if decomp, ok := decompObj.([]any); ok {
							var notDone []string
							for _, entryAny := range decomp {
								if entry, ok := entryAny.(map[string]any); ok {
									if childID, ok := entry["child_id"].(string); ok && childID != "" {
										_, childLoc := "", ""
										if c, err := FindTaskDir(childID, nil); err == nil && c != nil {
											childLoc = c.Location
										}
										if childLoc != "done" {
											if childLoc == "" {
												childLoc = "missing"
											}
											notDone = append(notDone, fmt.Sprintf("%s (%s)", childID, childLoc))
										}
									}
								}
							}
							if len(notDone) > 0 {
								fmt.Printf("[!] Parent task %s still has unfinished children: %s\n", taskID, strings.Join(notDone, ", "))
								fmt.Printf("[!] Clean each child after its human merge before cleaning the parent.\n")
								return
							}
						}
					}
				}
			}
		}
	}
	
	root, _ := ProjectRoot()
	worktreePath := filepath.Join(root, "worktrees", taskID)
	if _, err := os.Stat(worktreePath); err == nil {
		dirtyPaths := WorktreeDirtyPaths(worktreePath)
		dirty := len(dirtyPaths) > 0
		if dirty && !force && !stash {
			appendCleanupTrace(taskDir.Path, "execute", "blocked", "dirty_worktree_detected: "+strings.Join(dirtyPaths, ", "))
			fmt.Printf("[!] Worktree %s has uncommitted changes.\n", worktreePath)
			fmt.Printf("[!] Dirty paths: %s\n", strings.Join(dirtyPaths, ", "))
			fmt.Printf("[!] Refusing to clean task %s silently. Choose one of:\n", taskID)
			fmt.Printf("      cd %s && git status      # inspect changes\n", worktreePath)
			fmt.Printf("      cd %s && git commit -am '...'  # commit, then re-run clean\n", worktreePath)
			fmt.Printf("      quorum task clean %s --stash    # save WIP patch and clean\n", taskID)
			fmt.Printf("      quorum task clean %s --force    # discard WIP and clean\n", taskID)
			return
		}
		if dirty && stash {
			fmt.Printf("[*] Saving worktree changes as patch...\n")
			if ok, patchPath := SaveWorktreeChanges(worktreePath, taskID); !ok {
				fmt.Printf("[!] patch save failed: %s\n", strings.TrimSpace(patchPath)); return
			} else {
				fmt.Printf("[+] Saved worktree patch: %s\n", patchPath)
				appendCleanupTrace(taskDir.Path, "execute", "passed", "stash_path: "+patchPath)
			}
		}
		if force && dirty {
			appendCleanupTrace(taskDir.Path, "execute", "passed", "force_cleanup: dirty paths "+strings.Join(dirtyPaths, ", "))
			fmt.Printf("[*] Force-removing worktree %s (discarding changes)...\n", worktreePath)
			exec.Command("git", "-C", root, "worktree", "remove", "--force", worktreePath).Run()
		} else if stash && dirty {
			fmt.Printf("[*] Removing worktree %s after saving patch...\n", worktreePath)
			exec.Command("git", "-C", root, "worktree", "remove", "--force", worktreePath).Run()
		} else {
			fmt.Printf("[*] Removing worktree %s...\n", worktreePath)
			exec.Command("git", "-C", root, "worktree", "remove", worktreePath).Run()
		}
	}
	
	DeleteBranchIfMerged("ai/"+taskID, GetBaseBranch())
	if taskDir.Location == "active" {
		targetDir := filepath.Join(root, ".ai", "tasks", "done", filepath.Base(taskDir.Path))
		fmt.Printf("[*] Archiving task to done/...\n")
		os.MkdirAll(filepath.Dir(targetDir), 0755)
		os.Rename(taskDir.Path, targetDir)
	}
	fmt.Printf("[+] Task %s cleaned up.\n", taskID)
	if parentID != "" {
		AutoArchiveParentIfComplete(parentID)
	}
}

func AutoArchiveParentIfComplete(parentID string) {
	parentDir, err := FindTaskDir(parentID, []string{"active"})
	if err != nil || parentDir == nil || parentDir.Location != "active" {
		return
	}
	specPath := filepath.Join(parentDir.Path, "00-spec.yaml")
	payload, err := LoadArtifactPayload(specPath)
	if err != nil {
		return
	}
	spec, ok := payload.(map[string]any)
	if !ok {
		return
	}
	decompObj := spec["decomposition"]
	if decompObj == nil {
		return
	}
	decomp, ok := decompObj.([]any)
	if !ok || len(decomp) == 0 {
		return
	}
	for _, entryAny := range decomp {
		if entry, ok := entryAny.(map[string]any); ok {
			if childID, ok := entry["child_id"].(string); ok && childID != "" {
				c, err := FindTaskDir(childID, nil)
				if err != nil || c == nil || c.Location != "done" {
					return
				}
			}
		}
	}
	root, _ := ProjectRoot()
	targetDir := filepath.Join(root, ".ai", "tasks", "done", filepath.Base(parentDir.Path))
	fmt.Printf("[*] All children of %s are in done/; auto-archiving parent...\n", parentID)
	os.MkdirAll(filepath.Dir(targetDir), 0755)
	os.Rename(parentDir.Path, targetDir)
	fmt.Printf("[+] Parent task %s archived to done/.\n", parentID)
}

func BackTask(taskID string, opts ...bool) {
	force := false
	stash := false
	if len(opts) > 0 {
		force = opts[0]
	}
	if len(opts) > 1 {
		stash = opts[1]
	}
	if force && stash {
		fmt.Printf("[!] --force and --stash are mutually exclusive. Pick one: --force discards changes, --stash saves them to a patch.\n")
		return
	}
	taskDir, err := FindTaskDir(taskID, nil)
	if err != nil || taskDir == nil {
		fmt.Printf("[!] Task %s not found.\n", taskID)
		return
	}
	root, _ := ProjectRoot()
	worktreePath := filepath.Join(root, "worktrees", taskID)
	if _, err := os.Stat(worktreePath); err == nil {
		dirtyPaths := WorktreeDirtyPaths(worktreePath)
		dirty := len(dirtyPaths) > 0
		if dirty && !force && !stash {
			appendCleanupTrace(taskDir.Path, "execute", "blocked", "dirty_worktree_detected: "+strings.Join(dirtyPaths, ", "))
			fmt.Printf("[!] Worktree %s has uncommitted changes.\n", worktreePath)
			fmt.Printf("[!] Dirty paths: %s\n", strings.Join(dirtyPaths, ", "))
			fmt.Printf("[!] Refusing to remove task %s worktree silently. Choose one of:\n", taskID)
			fmt.Printf("      cd %s && git status      # inspect changes\n", worktreePath)
			fmt.Printf("      cd %s && git commit -am '...'  # commit, then re-run back\n", worktreePath)
			fmt.Printf("      quorum task back %s --stash     # save WIP patch and remove worktree\n", taskID)
			fmt.Printf("      quorum task back %s --force     # discard WIP and remove worktree\n", taskID)
			return
		}
		if dirty && stash {
			fmt.Printf("[*] Saving worktree changes as patch...\n")
			if ok, patchPath := SaveWorktreeChanges(worktreePath, taskID); !ok {
				fmt.Printf("[!] patch save failed: %s\n", strings.TrimSpace(patchPath))
				return
			} else {
				fmt.Printf("[+] Saved worktree patch: %s\n", patchPath)
				appendCleanupTrace(taskDir.Path, "execute", "passed", "stash_path: "+patchPath)
			}
		}
		fmt.Printf("[*] Reversing 'start': removing worktree %s...\n", worktreePath)
		args := []string{"worktree", "remove"}
		if dirty && (force || stash) {
			args = append(args, "--force")
			if force {
				appendCleanupTrace(taskDir.Path, "execute", "passed", "force_cleanup: dirty paths "+strings.Join(dirtyPaths, ", "))
			}
		}
		args = append(args, worktreePath)
		out, err := exec.Command("git", args...).CombinedOutput()
		if err != nil {
			fmt.Printf("[!] git worktree remove failed: %s\n", strings.TrimSpace(string(out)))
			return
		}
		branchName := "ai/" + taskID
		baseBranch := GetBaseBranch()
		out, err = exec.Command("git", "rev-list", "--count", baseBranch+".."+branchName).CombinedOutput()
		if err == nil {
			if strings.TrimSpace(string(out)) == "0" {
				exec.Command("git", "branch", "-D", branchName).Run()
				fmt.Printf("[+] Removed empty branch %s.\n", branchName)
			} else {
				fmt.Printf("[!] Branch %s has commits; not deleted. Use 'git branch -D %s' if you really want to drop them.\n", branchName, branchName)
			}
		}
		fmt.Printf("[+] Worktree removed. Task %s stays in %s/. Re-run '/q-blueprint %s' if the contract needs changes, or 'quorum task start %s' when ready.\n", taskID, taskDir.Location, taskID, taskID)
		return
	}
	if taskDir.Location == "done" || taskDir.Location == "failed" {
		targetDir := filepath.Join(root, ".ai", "tasks", "active", filepath.Base(taskDir.Path))
		os.MkdirAll(filepath.Dir(targetDir), 0755)
		fmt.Printf("[*] Reversing 'clean': moving task from %s/ back to active/...\n", taskDir.Location)
		os.Rename(taskDir.Path, targetDir)
		fmt.Printf("[+] Task %s restored to active/.\n", taskID)
		return
	}
	if taskDir.Location == "active" {
		targetDir := filepath.Join(root, ".ai", "tasks", "inbox", filepath.Base(taskDir.Path))
		os.MkdirAll(filepath.Dir(targetDir), 0755)
		fmt.Printf("[*] Reversing 'blueprint': moving task from active/ back to inbox/...\n")
		os.Rename(taskDir.Path, targetDir)
		fmt.Printf("[+] Task %s returned to inbox/. Re-run '/q-brief %s' to refine the spec.\n", taskID, taskID)
		return
	}
	if taskDir.Location == "inbox" {
		fmt.Printf("[!] Task %s is already in inbox/. There is no earlier state. Edit '00-spec.yaml' directly or delete the directory if you want to start over.\n", taskID)
		return
	}
}

func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func CopyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return CopyFile(path, target)
	})
}

func ensureClaudeSkillsSymlink(projectRoot, resourceSrc string) error {
	claudeDir := filepath.Join(projectRoot, ".claude")
	linkPath := filepath.Join(claudeDir, "skills")
	expectedTarget, err := filepath.EvalSymlinks(filepath.Join(resourceSrc, "skills"))
	if err != nil {
		expectedTarget, err = filepath.Abs(filepath.Join(resourceSrc, "skills"))
		if err != nil {
			return err
		}
	}
	os.MkdirAll(claudeDir, 0755)
	
	info, err := os.Lstat(linkPath)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			currentTarget, err := filepath.EvalSymlinks(linkPath)
			if err != nil {
				currentTarget, _ = os.Readlink(linkPath)
				if !filepath.IsAbs(currentTarget) {
					currentTarget = filepath.Join(claudeDir, currentTarget)
				}
			}
			if currentTarget == expectedTarget {
				fmt.Printf("  [=] .claude/skills already linked to %s\n", expectedTarget)
				return nil
			}
			return fmt.Errorf(".claude/skills es un symlink hacia un destino distinto al esperado.\n  Encontrado: %s\n  Esperado:   %s\nEliminá o ajustá el symlink manualmente antes de re-ejecutar quorum init.", currentTarget, expectedTarget)
		}
		kind := "archivo"
		if info.IsDir() {
			kind = "directorio"
		}
		return fmt.Errorf(".claude/skills existe como %s y no como symlink.\n  Encontrado: %s (%s)\n  Esperado:   symlink hacia %s\nEliminá o renombrá el contenido existente antes de re-ejecutar quorum init.", kind, linkPath, kind, expectedTarget)
	}
	
	fmt.Printf("  [+] Creating .claude/skills -> %s\n", expectedTarget)
	return os.Symlink(expectedTarget, linkPath)
}

func getResourceSrc() string {
	root, err := ProjectRoot()
	if err == nil {
		if _, err := os.Stat(filepath.Join(root, ".agents")); err == nil {
			return filepath.Join(root, ".agents")
		}
	}
	exe, _ := os.Executable()
	dir := filepath.Dir(exe)
	if _, err := os.Stat(filepath.Join(dir, ".agents")); err == nil {
		return filepath.Join(dir, ".agents")
	}
	return filepath.Join(root, ".agents")
}

func InitializeProject() {
	projectRoot, err := ProjectRoot()
	if err != nil {
		fmt.Printf("[!] Could not determine project root.\n")
		return
	}
	resourceSrc := getResourceSrc()
	
	fmt.Printf("[*] Initializing Quorum in %s...\n", projectRoot)
	
	dirs := []string{
		".ai/tasks/inbox",
		".ai/tasks/active",
		".ai/tasks/done",
		".ai/tasks/failed",
		"memory/decisions",
		"memory/patterns",
		"memory/lessons",
		"worktrees",
	}
	for _, d := range dirs {
		p := filepath.Join(projectRoot, filepath.FromSlash(d))
		if _, err := os.Stat(p); os.IsNotExist(err) {
			fmt.Printf("  [+] Creating %s/\n", d)
			os.MkdirAll(p, 0755)
			if d != "worktrees" {
				os.WriteFile(filepath.Join(p, ".gitkeep"), []byte(""), 0644)
			}
		}
	}
	
	scaffoldMap := map[string]string{
		"templates": ".ai/tasks/_template",
		"skills":    ".agents/skills",
		"schemas":   ".agents/schemas",
		"policies":  ".agents/policies",
		"prompts":   ".agents/prompts",
	}
	for srcSub, tgtSub := range scaffoldMap {
		srcPath := filepath.Join(resourceSrc, filepath.FromSlash(srcSub))
		tgtPath := filepath.Join(projectRoot, filepath.FromSlash(tgtSub))
		
		info, err := os.Stat(srcPath)
		if err == nil {
			fmt.Printf("  [*] Scaffolding %s...\n", tgtSub)
			if info.IsDir() {
				CopyDir(srcPath, tgtPath)
			} else {
				CopyFile(srcPath, tgtPath)
			}
		} else {
			if srcSub == "templates" {
				fallbackSrc := filepath.Join(filepath.Dir(resourceSrc), ".ai", "tasks", "_template")
				if info, err := os.Stat(fallbackSrc); err == nil && info.IsDir() {
					fmt.Printf("  [*] Scaffolding %s from fallback...\n", tgtSub)
					CopyDir(fallbackSrc, tgtPath)
					continue
				}
			}
			fmt.Printf("  [!] Warning: Source %s not found in %s\n", srcSub, resourceSrc)
		}
	}
	
	configSrc := filepath.Join(resourceSrc, "config.yaml")
	configTgt := filepath.Join(projectRoot, ".agents", "config.yaml")
	if _, err := os.Stat(configSrc); err == nil {
		fmt.Printf("  [*] Updating .agents/config.yaml...\n")
		os.MkdirAll(filepath.Dir(configTgt), 0755)
		CopyFile(configSrc, configTgt)
	}
	
	if err := ensureClaudeSkillsSymlink(projectRoot, resourceSrc); err != nil {
		fmt.Printf("[!] %v\n", err)
		os.Exit(1)
	}
	
	gitignorePath := filepath.Join(projectRoot, ".gitignore")
	ignoreEntries := []string{
		"\n# Quorum",
		"worktrees/",
		".ai/tasks/active/*",
		".ai/tasks/done/*",
		".ai/tasks/failed/*",
		".ai/tasks/inbox/*",
		"!.ai/tasks/active/.gitkeep",
		"!.ai/tasks/done/.gitkeep",
		"!.ai/tasks/failed/.gitkeep",
		"!.ai/tasks/inbox/.gitkeep",
	}
	if _, err := os.Stat(gitignorePath); err == nil {
		contentBytes, _ := os.ReadFile(gitignorePath)
		content := string(contentBytes)
		var newEntries []string
		for _, e := range ignoreEntries {
			if strings.TrimSpace(e) != "" && !strings.Contains(content, strings.TrimSpace(e)) {
				newEntries = append(newEntries, e)
			}
		}
		if len(newEntries) > 0 {
			fmt.Printf("[*] Updating .gitignore...\n")
			f, _ := os.OpenFile(gitignorePath, os.O_APPEND|os.O_WRONLY, 0644)
			for _, e := range newEntries {
				f.WriteString(e + "\n")
			}
			f.Close()
		}
	} else {
		fmt.Printf("[*] Creating .gitignore...\n")
		f, _ := os.Create(gitignorePath)
		for _, e := range ignoreEntries {
			f.WriteString(e + "\n")
		}
		f.Close()
	}
	fmt.Printf("[+] Quorum initialized successfully.\n")
}

func ensureRetryWorktree(taskID string) bool {
	root, _ := ProjectRoot()
	worktreePath := filepath.Join(root, "worktrees", taskID)
	if _, err := os.Stat(worktreePath); err == nil {
		if IsWorktreeDirty(worktreePath) {
			fmt.Printf("[!] Worktree %s has uncommitted changes.\n", worktreePath)
			fmt.Printf("[!] Refusing retry for %s until the worktree is clean.\n", taskID)
			fmt.Printf("      cd %s && git status\n", worktreePath)
			return false
		}
		return true
	}

	branchName := "ai/" + taskID
	var cmd *exec.Cmd
	if err := exec.Command("git", "-C", root, "rev-parse", "--verify", branchName).Run(); err == nil {
		cmd = exec.Command("git", "-C", root, "worktree", "add", worktreePath, branchName)
	} else {
		cmd = exec.Command("git", "-C", root, "worktree", "add", worktreePath, "-b", branchName, GetBaseBranch())
	}
	
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("[!] Could not prepare retry worktree for %s: %s\n", taskID, strings.TrimSpace(string(out)))
		return false
	}
	return true
}

func clearRetryArtifacts(taskDir string) []string {
	var removed []string
	for _, name := range []string{"05-validation.json", "06-review.json"} {
		p := filepath.Join(taskDir, name)
		if _, err := os.Stat(p); err == nil {
			os.Remove(p)
			removed = append(removed, name)
		}
	}
	return removed
}

func restoreParentForChildRetry(parentID string) bool {
	parentDir, err := FindTaskDir(parentID, []string{"active", "done"})
	if err != nil || parentDir == nil {
		fmt.Printf("[!] Parent task %s not found for retry.\n", parentID)
		return false
	}
	if parentDir.Location == "active" {
		return true
	}

	root, _ := ProjectRoot()
	targetDir := filepath.Join(root, ".ai", "tasks", "active", filepath.Base(parentDir.Path))
	if _, err := os.Stat(targetDir); err == nil {
		fmt.Printf("[!] Cannot restore parent %s: active/%s already exists.\n", parentID, filepath.Base(parentDir.Path))
		return false
	}
	fmt.Printf("[*] Restoring parent task %s from done/ to active/ for child retry...\n", parentID)
	os.MkdirAll(filepath.Dir(targetDir), 0755)
	os.Rename(parentDir.Path, targetDir)
	return true
}

func PrepareFailedChildRetry(taskID string) bool {
	taskDir, err := FindTaskDir(taskID, []string{"active", "failed"})
	if err != nil || taskDir == nil {
		fmt.Printf("[!] Task %s not found in active/ or failed/.\n", taskID)
		return false
	}
	if taskDir.Location == "active" {
		fmt.Printf("[*] Task %s is already active; retry preparation not needed.\n", taskID)
		return true
	}
	if taskDir.Location != "failed" {
		fmt.Printf("[!] Task %s is in %s/; retry preparation only handles failed/ children.\n", taskID, taskDir.Location)
		return false
	}

	specPath := filepath.Join(taskDir.Path, "00-spec.yaml")
	if _, err := os.Stat(specPath); os.IsNotExist(err) {
		fmt.Printf("[!] Cannot retry %s: missing 00-spec.yaml.\n", taskID)
		return false
	}
	payload, err := LoadArtifactPayload(specPath)
	if err != nil {
		fmt.Printf("[!] Cannot retry %s: invalid 00-spec.yaml: %v\n", taskID, err)
		return false
	}
	if err := ValidateArtifact(specPath, payload); err != nil {
		fmt.Printf("[!] Cannot retry %s: invalid 00-spec.yaml: %v\n", taskID, err)
		return false
	}
	spec, ok := payload.(map[string]any)
	if !ok {
		return false
	}

	parentID, ok := spec["parent_task"].(string)
	if !ok || parentID == "" {
		fmt.Printf("[!] Retry is only authorized for failed child tasks; %s has no parent_task.\n", taskID)
		return false
	}

	root, _ := ProjectRoot()
	activeTarget := filepath.Join(root, ".ai", "tasks", "active", filepath.Base(taskDir.Path))
	if _, err := os.Stat(activeTarget); err == nil {
		fmt.Printf("[!] Cannot retry %s: active/%s already exists.\n", taskID, filepath.Base(taskDir.Path))
		return false
	}

	tracePath := filepath.Join(taskDir.Path, "07-trace.json")
	if _, err := os.Stat(tracePath); os.IsNotExist(err) {
		fmt.Printf("[!] Cannot retry %s: missing 07-trace.json to preserve attempts history.\n", taskID)
		return false
	}
	tracePayload, err := LoadArtifactPayload(tracePath)
	if err == nil {
		err = ValidateArtifact(tracePath, tracePayload)
	}
	if err != nil {
		fmt.Printf("[!] Cannot retry %s: invalid 07-trace.json: %v\n", taskID, err)
		return false
	}

	if !ensureRetryWorktree(taskID) {
		return false
	}
	if !restoreParentForChildRetry(parentID) {
		return false
	}

	removed := clearRetryArtifacts(taskDir.Path)
	os.MkdirAll(filepath.Dir(activeTarget), 0755)
	os.Rename(taskDir.Path, activeTarget)
	if len(removed) > 0 {
		fmt.Printf("[*] Removed stale retry artifacts for %s: %s\n", taskID, strings.Join(removed, ", "))
	}
	fmt.Printf("[+] Failed child task %s restored to active/ for /q-implement retry.\n", taskID)
	return true
}

func DeriveParentState(spec map[string]any) string {
	decompObj := spec["decomposition"]
	if decompObj == nil {
		return "active"
	}
	decomp, ok := decompObj.([]any)
	if !ok {
		return "active"
	}
	var childLocs []string
	for _, entryAny := range decomp {
		if entry, ok := entryAny.(map[string]any); ok {
			if childID, ok := entry["child_id"].(string); ok && childID != "" {
				c, err := FindTaskDir(childID, nil)
				if err == nil && c != nil {
					childLocs = append(childLocs, c.Location)
				}
			}
		}
	}
	for _, loc := range childLocs {
		if loc == "failed" {
			return "partial"
		}
	}
	if len(childLocs) > 0 {
		allDone := true
		for _, loc := range childLocs {
			if loc != "done" {
				allDone = false
				break
			}
		}
		if allDone {
			return "completed"
		}
	}
	return "active"
}
