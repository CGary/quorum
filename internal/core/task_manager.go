package core

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
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
	store, err := DefaultTaskStore()
	if err != nil {
		return "", err
	}
	taskDir, err := store.FindTask(taskID, "inbox", "active", "done", "failed")
	if err != nil {
		return "", err
	}
	if taskDir != nil {
		fmt.Printf("[!] Task %s already exists in %s/.\n", taskID, taskDir.Location)
		return taskDir.Path, nil
	}

	dirPath := filepath.Join(store.ProjectRoot, ".ai", "tasks", "inbox", taskID+"-new-spec")
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return "", err
	}
	spec := map[string]any{
		"task_id":    taskID,
		"summary":    "Draft spec; fill goal, invariants, and acceptance before blueprint.",
		"goal":       "TODO: define the feature goal.",
		"invariants": []any{"TODO: define invariant."},
		"acceptance": []any{"TODO: define acceptance criterion."},
		"risk":       "medium",
	}
	_, err = store.SaveArtifact(&TaskDirMatch{Path: dirPath, Location: "inbox"}, "00-spec.yaml", spec)
	return dirPath, err
}

func PrepareBlueprint(taskID string) (string, error) {
	store, err := DefaultTaskStore()
	if err != nil {
		return "", err
	}
	taskDir, err := store.FindTask(taskID, "inbox")
	if err != nil {
		return "", err
	}
	if taskDir == nil {
		activeDir, err := store.FindTask(taskID, "active")
		if err != nil {
			return "", err
		}
		if activeDir != nil {
			fmt.Printf("[*] Task %s is already in active.\n", taskID)
			return activeDir.Path, nil
		}
		return "", fmt.Errorf("Task %s not found in inbox.", taskID)
	}

	moved, err := store.MoveTask(taskDir, "active")
	if err != nil {
		return "", err
	}
	return moved.Path, nil
}

// acceptanceStatement returns the plain statement text of an acceptance item:
// the string itself for string items, the "statement" field for object items.
func acceptanceStatement(item any) string {
	if s, ok := item.(string); ok {
		return s
	}
	if stmt, ok := lookupKey(item, "statement"); ok {
		if s, ok := stmt.(string); ok {
			return s
		}
	}
	return fmt.Sprintf("%v", item)
}

// flattenAcceptance strips object-form acceptance criteria down to their plain
// statement. Acceptance ids belong to the spec where they are born; children
// must not inherit the parent's AC-* identities.
func flattenAcceptance(acceptance any) any {
	items, ok := asSlice(acceptance)
	if !ok {
		return acceptance
	}
	flat := make([]any, len(items))
	for i, item := range items {
		flat[i] = acceptanceStatement(item)
	}
	return flat
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
			"task_id":     childID,
			"summary":     item["summary"],
			"goal":        "Subset of " + parentID + ": " + fmt.Sprintf("%v", item["summary"]),
			"invariants":  parentSpec["invariants"],
			"acceptance":  flattenAcceptance(parentSpec["acceptance"]),
			"risk":        parentSpec["risk"],
			"parent_task": parentID,
			"non_goals":   parentSpec["non_goals"],
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
	return execGitRunner{}.BaseBranch()
}

func StartTask(taskID string) {
	startTaskWith(execGitRunner{}, taskID)
}

func startTaskWith(git GitRunner, taskID string) {
	fmt.Printf("[*] Starting task %s...\n", taskID)
	store, err := DefaultTaskStore()
	if err != nil {
		fmt.Printf("[!] Error initializing task store: %v\n", err)
		return
	}
	taskDir, err := store.FindTask(taskID, "active", "inbox")
	if err != nil || taskDir == nil {
		fmt.Printf("[!] Task %s not found.\n", taskID)
		return
	}
	contractPath, err := store.TaskArtifactPath(taskDir, "02-contract.yaml")
	if err != nil {
		fmt.Printf("[!] Contract validation failed for %s: %v\n", taskID, err)
		return
	}
	if _, err := os.Stat(contractPath); os.IsNotExist(err) {
		fmt.Printf("[!] Contract (02-contract.yaml) not found for %s.\n", taskID)
		fmt.Printf("[!] Please run 'quorum task blueprint %s' first.\n", taskID)
		return
	}
	contract, err := store.LoadArtifact(taskDir, "02-contract.yaml")
	if err != nil {
		fmt.Printf("[!] Contract validation failed for %s: %v\n", taskID, err)
		return
	}
	if taskDir.Location == "inbox" {
		fmt.Printf("[*] Moving task from inbox to active...\n")
		if moved, err := store.MoveTask(taskDir, "active"); err == nil {
			taskDir = moved
		} else {
			fmt.Printf("[!] Error moving task: %v\n", err)
			return
		}
	}

	root, _ := ProjectRoot()
	worktreePath := filepath.Join(root, "worktrees", taskID)
	branchName := "ai/" + taskID
	baseBranch := git.BaseBranch()

	if _, err := os.Stat(worktreePath); err == nil {
		fmt.Printf("[*] Worktree for %s already exists.\n", taskID)
	} else {
		fmt.Printf("[*] Creating worktree in %s (base: %s)...\n", worktreePath, baseBranch)
		if err := git.WorktreeAdd(worktreePath, branchName, baseBranch); err != nil {
			fmt.Printf("[!] Error creating worktree: %s\n", err)
			return
		}
	}

	logPath, err := store.TaskArtifactPath(taskDir, "04-implementation-log.yaml")
	if err != nil {
		fmt.Printf("[!] Error initializing implementation log: %v\n", err)
		return
	}
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
		if _, err := store.SaveArtifact(taskDir, "04-implementation-log.yaml", log); err != nil {
			fmt.Printf("[!] Error initializing implementation log: %v\n", err)
			return
		}
	}

	tracePath, err := store.TaskArtifactPath(taskDir, "07-trace.json")
	if err != nil {
		fmt.Printf("[!] Error initializing trace: %v\n", err)
		return
	}
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
		if _, err := store.SaveArtifact(taskDir, "07-trace.json", trace); err != nil {
			fmt.Printf("[!] Error initializing trace: %v\n", err)
			return
		}
	}
	fmt.Printf("[+] Task %s initialized and worktree ready.\n", taskID)
}

func WorktreeDirtyPaths(worktreePath string) []string {
	paths, _ := execGitRunner{}.DirtyPaths(worktreePath)
	return paths
}

func IsWorktreeDirty(worktreePath string) bool {
	return len(WorktreeDirtyPaths(worktreePath)) > 0
}

func SaveWorktreeChanges(worktreePath, taskID string) (bool, string) {
	patchPath, err := execGitRunner{}.SavePatch(worktreePath, taskID)
	if err != nil {
		return false, err.Error()
	}
	return true, patchPath
}

func appendCleanupTrace(taskDirPath, phase, result, notes string) {
	tracePath := filepath.Join(taskDirPath, "07-trace.json")
	payload, err := LoadArtifactPayload(tracePath)
	if err != nil {
		return
	}
	trace, ok := payload.(map[string]any)
	if !ok {
		return
	}
	items, _ := asSlice(trace["attempts"])
	items = append(items, map[string]any{"phase": phase, "result": result, "duration_s": 0.0, "notes": notes})
	trace["attempts"] = items
	_, _ = SaveArtifact(tracePath, trace)
}

func BranchExists(branchName string) bool {
	return execGitRunner{}.BranchExists(branchName)
}

func DeleteBranchIfMerged(branchName, baseBranch string) bool {
	return execGitRunner{}.DeleteBranchIfMerged(branchName, baseBranch)
}

func CleanTask(taskID string, force, stash bool) {
	cleanTaskWith(execGitRunner{}, taskID, force, stash)
}

func cleanTaskWith(git GitRunner, taskID string, force, stash bool) {
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
		dirtyPaths, _ := git.DirtyPaths(worktreePath)
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
			patchPath, err := git.SavePatch(worktreePath, taskID)
			if err != nil {
				fmt.Printf("[!] patch save failed: %s\n", strings.TrimSpace(err.Error()))
				return
			}
			fmt.Printf("[+] Saved worktree patch: %s\n", patchPath)
			appendCleanupTrace(taskDir.Path, "execute", "passed", "stash_path: "+patchPath)
		}
		if force && dirty {
			appendCleanupTrace(taskDir.Path, "execute", "passed", "force_cleanup: dirty paths "+strings.Join(dirtyPaths, ", "))
			fmt.Printf("[*] Force-removing worktree %s (discarding changes)...\n", worktreePath)
			_ = git.WorktreeRemove(worktreePath, true)
		} else if stash && dirty {
			fmt.Printf("[*] Removing worktree %s after saving patch...\n", worktreePath)
			_ = git.WorktreeRemove(worktreePath, true)
		} else {
			fmt.Printf("[*] Removing worktree %s...\n", worktreePath)
			_ = git.WorktreeRemove(worktreePath, false)
		}
	}

	git.DeleteBranchIfMerged("ai/"+taskID, git.BaseBranch())
	if taskDir.Location == "active" {
		fmt.Printf("[*] Archiving task to done/...\n")
		store, err := DefaultTaskStore()
		if err != nil {
			fmt.Printf("[!] Error initializing task store: %v\n", err)
			return
		}
		if _, err := store.MoveTask(taskDir, "done"); err != nil {
			fmt.Printf("[!] Error archiving task: %v\n", err)
			return
		}
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
	fmt.Printf("[*] All children of %s are in done/; auto-archiving parent...\n", parentID)
	store, err := DefaultTaskStore()
	if err != nil {
		fmt.Printf("[!] Cannot archive parent %s: %v\n", parentID, err)
		return
	}
	if _, err := store.MoveTask(parentDir, "done"); err != nil {
		fmt.Printf("[!] Cannot archive parent %s: %v\n", parentID, err)
		return
	}
	fmt.Printf("[+] Parent task %s archived to done/.\n", parentID)
}

func BackTask(taskID string, opts ...bool) {
	backTaskWith(execGitRunner{}, taskID, opts...)
}

func backTaskWith(git GitRunner, taskID string, opts ...bool) {
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
		dirtyPaths, _ := git.DirtyPaths(worktreePath)
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
			patchPath, err := git.SavePatch(worktreePath, taskID)
			if err != nil {
				fmt.Printf("[!] patch save failed: %s\n", strings.TrimSpace(err.Error()))
				return
			}
			fmt.Printf("[+] Saved worktree patch: %s\n", patchPath)
			appendCleanupTrace(taskDir.Path, "execute", "passed", "stash_path: "+patchPath)
		}
		fmt.Printf("[*] Reversing 'start': removing worktree %s...\n", worktreePath)
		forceRemove := dirty && (force || stash)
		if forceRemove && force {
			appendCleanupTrace(taskDir.Path, "execute", "passed", "force_cleanup: dirty paths "+strings.Join(dirtyPaths, ", "))
		}
		if err := git.WorktreeRemove(worktreePath, forceRemove); err != nil {
			fmt.Printf("[!] git worktree remove failed: %s\n", err)
			return
		}
		git.ForceDeleteBranchIfEmpty("ai/"+taskID, git.BaseBranch())
		fmt.Printf("[+] Worktree removed. Task %s stays in %s/. Re-run '/q-blueprint %s' if the contract needs changes, or 'quorum task start %s' when ready.\n", taskID, taskDir.Location, taskID, taskID)
		return
	}
	if taskDir.Location == "done" || taskDir.Location == "failed" {
		fmt.Printf("[*] Reversing 'clean': moving task from %s/ back to active/...\n", taskDir.Location)
		store, err := DefaultTaskStore()
		if err != nil {
			fmt.Printf("[!] Error initializing task store: %v\n", err)
			return
		}
		if _, err := store.MoveTask(taskDir, "active"); err != nil {
			fmt.Printf("[!] Error moving task: %v\n", err)
			return
		}
		fmt.Printf("[+] Task %s restored to active/.\n", taskID)
		return
	}
	if taskDir.Location == "active" {
		fmt.Printf("[*] Reversing 'blueprint': moving task from active/ back to inbox/...\n")
		store, err := DefaultTaskStore()
		if err != nil {
			fmt.Printf("[!] Error initializing task store: %v\n", err)
			return
		}
		if _, err := store.MoveTask(taskDir, "inbox"); err != nil {
			fmt.Printf("[!] Error moving task: %v\n", err)
			return
		}
		fmt.Printf("[+] Task %s returned to inbox/. Re-run '/q-brief %s' to refine the spec.\n", taskID, taskID)
		return
	}
	if taskDir.Location == "inbox" {
		fmt.Printf("[!] Task %s is already in inbox/. There is no earlier state. Edit '00-spec.yaml' directly or delete the directory if you want to start over.\n", taskID)
		return
	}
}

func sameFilesystemPath(a, b string) bool {
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA == nil && errB == nil && filepath.Clean(absA) == filepath.Clean(absB) {
		return true
	}
	infoA, errA := os.Stat(a)
	infoB, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(infoA, infoB)
}

func CopyFile(src, dst string) error {
	if sameFilesystemPath(src, dst) {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func CopyDir(src, dst string) error {
	if sameFilesystemPath(src, dst) {
		return nil
	}
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
			if strings.HasSuffix(filepath.ToSlash(currentTarget), "/quorum/.agents/skills") {
				fmt.Printf("  [*] Updating legacy .claude/skills symlink from %s to %s\n", currentTarget, expectedTarget)
				if err := os.Remove(linkPath); err != nil {
					return err
				}
				return os.Symlink(expectedTarget, linkPath)
			}
			return fmt.Errorf(".claude/skills es un symlink hacia un destino distinto al esperado.\n  Encontrado: %s\n  Esperado:   %s\nEl symlink no parece ser legacy de Quorum; ajustalo manualmente antes de re-ejecutar quorum init.", currentTarget, expectedTarget)
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

func usableResourceSrc(path string) bool {
	checks := []string{
		filepath.Join(path, "skills", "q-brief", "SKILL.md"),
		filepath.Join(path, "schemas", "spec.schema.json"),
		filepath.Join(path, "policies", "risk.yaml"),
	}
	for _, check := range checks {
		info, err := os.Stat(check)
		if err != nil || info.IsDir() || info.Size() == 0 {
			return false
		}
	}
	return true
}

func getResourceSrc() string {
	var fallback string
	if root, err := ProjectRoot(); err == nil {
		fallback = filepath.Join(root, ".agents")
	}
	exe, _ := os.Executable()
	if exe != "" {
		candidate := filepath.Join(filepath.Dir(exe), ".agents")
		if usableResourceSrc(candidate) {
			return candidate
		}
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		candidate := filepath.Join(filepath.Dir(file), "..", "..", ".agents")
		if usableResourceSrc(candidate) {
			return filepath.Clean(candidate)
		}
	}
	// Hermetic fallback: the canonical bundle compiled into the binary. This is
	// preferred over the project's OWN .agents so re-running `quorum init`
	// resyncs from canonical instead of copying a stale tree onto itself.
	if dir, ok := EmbeddedAgentsDir(); ok {
		return dir
	}
	if fallback != "" && usableResourceSrc(fallback) {
		return fallback
	}
	return fallback
}

type InitOptions struct {
	ProjectID      string
	ProjectName    string
	NonInteractive bool
}

func InitializeProject() {
	if err := InitializeProjectWithOptions(InitOptions{}); err != nil {
		fmt.Printf("[!] %v\n", err)
	}
}

func InitializeProjectWithOptions(opts InitOptions) error {
	projectRoot, err := ProjectRoot()
	if err != nil {
		return fmt.Errorf("could not determine project root")
	}
	resourceSrc := getResourceSrc()

	fmt.Printf("[*] Initializing Quorum in %s...\n", projectRoot)

	dirs := []string{
		".ai/tasks/inbox",
		".ai/tasks/active",
		".ai/tasks/done",
		".ai/tasks/failed",
		"worktrees",
	}
	for _, d := range dirs {
		p := filepath.Join(projectRoot, filepath.FromSlash(d))
		if _, err := os.Stat(p); os.IsNotExist(err) {
			fmt.Printf("  [+] Creating %s/\n", d)
			if err := os.MkdirAll(p, 0755); err != nil {
				return err
			}
			if d != "worktrees" {
				if err := os.WriteFile(filepath.Join(p, ".gitkeep"), []byte(""), 0644); err != nil {
					return err
				}
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
				err = CopyDir(srcPath, tgtPath)
			} else {
				err = CopyFile(srcPath, tgtPath)
			}
			if err != nil {
				fmt.Printf("  [!] Warning: could not scaffold %s: %v\n", tgtSub, err)
			}
		} else {
			if srcSub == "templates" {
				fallbackSrc := filepath.Join(filepath.Dir(resourceSrc), ".ai", "tasks", "_template")
				if info, err := os.Stat(fallbackSrc); err == nil && info.IsDir() {
					fmt.Printf("  [*] Scaffolding %s from fallback...\n", tgtSub)
					if err := CopyDir(fallbackSrc, tgtPath); err != nil {
						fmt.Printf("  [!] Warning: could not scaffold %s: %v\n", tgtSub, err)
					}
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
		if err := os.MkdirAll(filepath.Dir(configTgt), 0755); err != nil {
			return err
		}
		if err := CopyFile(configSrc, configTgt); err != nil {
			fmt.Printf("  [!] Warning: could not update .agents/config.yaml: %v\n", err)
		}
	}

	config, err := ensureProjectConfig(projectRoot, opts)
	if err != nil {
		return err
	}
	dbPath, err := MemoryDBPath()
	if err != nil {
		return err
	}
	db, err := OpenMemoryDB(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := EnsureMemoryProject(db, config, projectRoot, GitRemote(projectRoot)); err != nil {
		return err
	}
	migration, err := RunInitMemoryMigration(db, projectRoot, config)
	if err != nil {
		return err
	}
	if migration.FilesSeen > 0 {
		fmt.Printf("  [*] Migrated %d legacy memory files; deleted %d verified files.\n", migration.FilesInserted, migration.FilesDeleted)
	}

	if err := ensureClaudeSkillsSymlink(projectRoot, filepath.Join(projectRoot, ".agents")); err != nil {
		return err
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
			f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_WRONLY, 0644)
			if err != nil {
				return err
			}
			for _, e := range newEntries {
				_, _ = f.WriteString(e + "\n")
			}
			_ = f.Close()
		}
	} else {
		fmt.Printf("[*] Creating .gitignore...\n")
		f, err := os.Create(gitignorePath)
		if err != nil {
			return err
		}
		for _, e := range ignoreEntries {
			_, _ = f.WriteString(e + "\n")
		}
		_ = f.Close()
	}
	fmt.Printf("[+] Quorum initialized successfully.\n")
	return nil
}

func ensureProjectConfig(projectRoot string, opts InitOptions) (*QuorumConfig, error) {
	config, err := ReadQuorumConfigFrom(projectRoot)
	if err == nil {
		if opts.ProjectID != "" || opts.ProjectName != "" {
			if opts.ProjectID != "" && opts.ProjectID != config.ProjectID {
				return nil, fmt.Errorf("existing .quorumrc project_id %q does not match --project-id %q", config.ProjectID, opts.ProjectID)
			}
			if opts.ProjectName != "" && opts.ProjectName != config.ProjectName {
				config.ProjectName = opts.ProjectName
				if err := WriteQuorumConfigTo(config, projectRoot); err != nil {
					return nil, err
				}
			}
		}
		return config, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	if (opts.ProjectID == "") != (opts.ProjectName == "") {
		return nil, fmt.Errorf("both --project-id and --project-name are required when .quorumrc is absent")
	}
	if opts.ProjectID == "" && opts.ProjectName == "" {
		if opts.NonInteractive {
			return nil, fmt.Errorf(".quorumrc is missing; provide --project-id and --project-name for non-interactive init")
		}
		suggested := SuggestProjectIdentity(projectRoot)
		config, err = promptProjectConfig(os.Stdin, os.Stdout, suggested)
		if err != nil {
			return nil, err
		}
		if err := WriteQuorumConfigTo(config, projectRoot); err != nil {
			return nil, err
		}
		fmt.Printf("  [+] Created .quorumrc for project %s.\n", config.ProjectID)
		return config, nil
	}
	config = &QuorumConfig{ProjectID: opts.ProjectID, ProjectName: opts.ProjectName}
	if err := WriteQuorumConfigTo(config, projectRoot); err != nil {
		return nil, err
	}
	fmt.Printf("  [+] Created .quorumrc for project %s.\n", config.ProjectID)
	return config, nil
}

// promptProjectConfig drives an interactive capture of project_id and
// project_name from the injected reader/writer. It is pure and injectable so
// tests never touch real stdin: ensureProjectConfig passes os.Stdin/os.Stdout
// at the call site. Empty input accepts the suggested default; a non-empty
// project_id is normalized via SlugifyProjectID and re-prompted only when
// normalization yields empty. The final value is confirmed and validated
// through ValidateQuorumConfig before it is returned. Closing the reader (EOF)
// at any prompt aborts with an error pointing at the flags.
func promptProjectConfig(in io.Reader, out io.Writer, suggested *QuorumConfig) (*QuorumConfig, error) {
	if suggested == nil {
		suggested = &QuorumConfig{}
	}
	scanner := bufio.NewScanner(in)
	eofErr := fmt.Errorf("input closed before completing prompt; use --project-id and --project-name to set the project identity non-interactively")

	var projectID string
	for {
		fmt.Fprintf(out, "project_id [%s]: ", suggested.ProjectID)
		if !scanner.Scan() {
			return nil, eofErr
		}
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			projectID = suggested.ProjectID
			break
		}
		projectID = SlugifyProjectID(raw)
		if projectID == "" {
			fmt.Fprintf(out, "  invalid project_id %q: normalizes to empty; please enter alphanumeric characters\n", raw)
			continue
		}
		break
	}

	defaultName := suggested.ProjectName
	if projectID != suggested.ProjectID {
		defaultName = humanizeProjectName(projectID)
	}
	fmt.Fprintf(out, "project_name [%s]: ", defaultName)
	if !scanner.Scan() {
		return nil, eofErr
	}
	projectName := strings.TrimSpace(scanner.Text())
	if projectName == "" {
		projectName = defaultName
	}

	final := &QuorumConfig{ProjectID: projectID, ProjectName: projectName}
	fmt.Fprintf(out, "\nproject_id:   %s\nproject_name: %s\nWrite .quorumrc? [Y/n]: ", final.ProjectID, final.ProjectName)
	if !scanner.Scan() {
		return nil, eofErr
	}
	switch strings.ToLower(strings.TrimSpace(scanner.Text())) {
	case "", "y", "yes":
		// confirmed
	default:
		return nil, fmt.Errorf("aborted: .quorumrc not written")
	}

	if err := ValidateQuorumConfig(final); err != nil {
		return nil, err
	}
	return final, nil
}

func ensureRetryWorktreeWith(git GitRunner, taskID string) bool {
	root, _ := ProjectRoot()
	worktreePath := filepath.Join(root, "worktrees", taskID)
	if _, err := os.Stat(worktreePath); err == nil {
		if dirtyPaths, _ := git.DirtyPaths(worktreePath); len(dirtyPaths) > 0 {
			fmt.Printf("[!] Worktree %s has uncommitted changes.\n", worktreePath)
			fmt.Printf("[!] Refusing retry for %s until the worktree is clean.\n", taskID)
			fmt.Printf("      cd %s && git status\n", worktreePath)
			return false
		}
		return true
	}

	branchName := "ai/" + taskID
	var err error
	if git.RefExists(branchName) {
		err = git.WorktreeAttach(worktreePath, branchName)
	} else {
		err = git.WorktreeAdd(worktreePath, branchName, git.BaseBranch())
	}
	if err != nil {
		fmt.Printf("[!] Could not prepare retry worktree for %s: %s\n", taskID, err)
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

	store, _ := DefaultTaskStore()
	fmt.Printf("[*] Restoring parent task %s from done/ to active/ for child retry...\n", parentID)
	if _, err := store.MoveTask(parentDir, "active"); err != nil {
		fmt.Printf("[!] Cannot restore parent %s: %v\n", parentID, err)
		return false
	}
	return true
}

func PrepareFailedChildRetry(taskID string) bool {
	return prepareFailedChildRetryWith(execGitRunner{}, taskID)
}

func prepareFailedChildRetryWith(git GitRunner, taskID string) bool {
	store, err := DefaultTaskStore()
	if err != nil {
		fmt.Printf("[!] Error initializing task store: %v\n", err)
		return false
	}
	taskDir, err := store.FindTask(taskID, "active", "failed")
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

	specPath, err := store.TaskArtifactPath(taskDir, "00-spec.yaml")
	if err != nil {
		fmt.Printf("[!] Cannot retry %s: invalid 00-spec.yaml: %v\n", taskID, err)
		return false
	}
	if _, err := os.Stat(specPath); os.IsNotExist(err) {
		fmt.Printf("[!] Cannot retry %s: missing 00-spec.yaml.\n", taskID)
		return false
	}
	payload, err := store.LoadArtifact(taskDir, "00-spec.yaml")
	if err != nil {
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

	activeTarget := filepath.Join(store.ProjectRoot, ".ai", "tasks", "active", filepath.Base(taskDir.Path))
	if _, err := os.Stat(activeTarget); err == nil {
		fmt.Printf("[!] Cannot retry %s: active/%s already exists.\n", taskID, filepath.Base(taskDir.Path))
		return false
	}

	tracePath, err := store.TaskArtifactPath(taskDir, "07-trace.json")
	if err != nil {
		fmt.Printf("[!] Cannot retry %s: invalid 07-trace.json: %v\n", taskID, err)
		return false
	}
	if _, err := os.Stat(tracePath); os.IsNotExist(err) {
		fmt.Printf("[!] Cannot retry %s: missing 07-trace.json to preserve attempts history.\n", taskID)
		return false
	}
	if _, err := store.LoadArtifact(taskDir, "07-trace.json"); err != nil {
		fmt.Printf("[!] Cannot retry %s: invalid 07-trace.json: %v\n", taskID, err)
		return false
	}

	if !ensureRetryWorktreeWith(git, taskID) {
		return false
	}
	if !restoreParentForChildRetry(parentID) {
		return false
	}

	removed := clearRetryArtifacts(taskDir.Path)
	if _, err := store.MoveTask(taskDir, "active"); err != nil {
		fmt.Printf("[!] Cannot retry %s: %v\n", taskID, err)
		return false
	}
	if len(removed) > 0 {
		fmt.Printf("[*] Removed stale retry artifacts for %s: %s\n", taskID, strings.Join(removed, ", "))
	}
	fmt.Printf("[+] Failed child task %s restored to active/ for /q-implement retry.\n", taskID)
	return true
}

func DeriveParentState(spec map[string]any) string {
	root, err := ProjectRoot()
	if err != nil {
		return "active"
	}
	return DeriveParentStateIn(root, spec)
}

func DeriveParentStateIn(projectRoot string, spec map[string]any) string {
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
				c, err := FindTaskDirIn(projectRoot, childID, nil)
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
