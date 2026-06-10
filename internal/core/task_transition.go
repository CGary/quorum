package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	transitionBlueprint         = "blueprint"
	transitionStart             = "start"
	transitionClean             = "clean"
	transitionAutoArchiveParent = "auto-archive-parent"
	transitionRetryPrepare      = "retry-prepare"
)

// TransitionContext carries the task aggregate state and side-effect ports used
// by a lifecycle transition. The table is deliberately code-owned: lifecycle
// rules are constitutional behavior, not user-editable configuration.
type TransitionContext struct {
	TaskID string
	Store  TaskStore
	Git    GitRunner

	TaskDir      *TaskDirMatch
	Contract     any
	Force        bool
	Stash        bool
	ParentTaskID string

	ResultTaskDir    *TaskDirMatch
	RemovedArtifacts []string
	Noop             bool
}

// TaskTransition makes one authorized lifecycle move explicit. BackTask is not
// represented here because it is a human-only reversal dispatcher that infers
// the inverse action from current filesystem evidence.
type TaskTransition struct {
	Name   string
	From   []string
	To     string
	Guard  func(*TransitionContext) error
	Effect func(*TransitionContext) error
}

func (t TaskTransition) AllowsFrom(location string) bool {
	for _, from := range t.From {
		if from == location {
			return true
		}
	}
	return false
}

func transitionTable() []TaskTransition {
	return []TaskTransition{
		prepareBlueprintTransition(),
		startTaskTransition(),
		cleanTaskTransition(),
		autoArchiveParentTransition(),
		retryPrepareTransition(),
	}
}

func transitionByName(name string) (TaskTransition, bool) {
	for _, transition := range transitionTable() {
		if transition.Name == name {
			return transition, true
		}
	}
	return TaskTransition{}, false
}

func mustTransition(name string) TaskTransition {
	transition, ok := transitionByName(name)
	if !ok {
		panic("unknown task transition: " + name)
	}
	return transition
}

func runTaskTransition(name string, ctx *TransitionContext) error {
	transition := mustTransition(name)
	if err := transition.Guard(ctx); err != nil {
		return err
	}
	return transition.Effect(ctx)
}

func ensureTransitionStore(ctx *TransitionContext) error {
	if ctx.Store.ProjectRoot != "" {
		return nil
	}
	store, err := DefaultTaskStore()
	if err != nil {
		return err
	}
	ctx.Store = store
	return nil
}

func prepareBlueprintTransition() TaskTransition {
	return TaskTransition{
		Name: transitionBlueprint,
		From: []string{"inbox"},
		To:   "active",
		Guard: func(ctx *TransitionContext) error {
			if err := ensureTransitionStore(ctx); err != nil {
				return err
			}
			taskDir, err := ctx.Store.FindTask(ctx.TaskID, "inbox")
			if err != nil {
				return err
			}
			if taskDir == nil {
				activeDir, err := ctx.Store.FindTask(ctx.TaskID, "active")
				if err != nil {
					return err
				}
				if activeDir != nil {
					ctx.TaskDir = activeDir
					ctx.ResultTaskDir = activeDir
					ctx.Noop = true
					return nil
				}
				return fmt.Errorf("Task %s not found in inbox.", ctx.TaskID)
			}
			ctx.TaskDir = taskDir
			return nil
		},
		Effect: func(ctx *TransitionContext) error {
			if ctx.Noop {
				fmt.Printf("[*] Task %s is already in active.\n", ctx.TaskID)
				return nil
			}
			moved, err := ctx.Store.MoveTask(ctx.TaskDir, "active")
			if err != nil {
				return err
			}
			ctx.ResultTaskDir = moved
			return nil
		},
	}
}

func runPrepareBlueprintTransition(taskID string) (string, error) {
	ctx := &TransitionContext{TaskID: taskID}
	if err := runTaskTransition(transitionBlueprint, ctx); err != nil {
		return "", err
	}
	if ctx.ResultTaskDir == nil {
		return "", fmt.Errorf("Task %s transition produced no task directory", taskID)
	}
	return ctx.ResultTaskDir.Path, nil
}

func startTaskTransition() TaskTransition {
	return TaskTransition{
		Name: transitionStart,
		From: []string{"active", "inbox"},
		To:   "active",
		Guard: func(ctx *TransitionContext) error {
			if err := ensureTransitionStore(ctx); err != nil {
				return fmt.Errorf("[!] Error initializing task store: %v", err)
			}
			taskDir, err := ctx.Store.FindTask(ctx.TaskID, "active", "inbox")
			if err != nil || taskDir == nil {
				return fmt.Errorf("[!] Task %s not found.", ctx.TaskID)
			}
			ctx.TaskDir = taskDir

			contractPath, err := ctx.Store.TaskArtifactPath(taskDir, "02-contract.yaml")
			if err != nil {
				return fmt.Errorf("[!] Contract validation failed for %s: %v", ctx.TaskID, err)
			}
			if _, err := os.Stat(contractPath); os.IsNotExist(err) {
				return fmt.Errorf("[!] Contract (02-contract.yaml) not found for %s.\n[!] Please run 'quorum task blueprint %s' first.", ctx.TaskID, ctx.TaskID)
			}
			contract, err := ctx.Store.LoadArtifact(taskDir, "02-contract.yaml")
			if err != nil {
				return fmt.Errorf("[!] Contract validation failed for %s: %v", ctx.TaskID, err)
			}
			ctx.Contract = contract
			return nil
		},
		Effect: func(ctx *TransitionContext) error {
			if ctx.TaskDir.Location == "inbox" {
				fmt.Printf("[*] Moving task from inbox to active...\n")
				moved, err := ctx.Store.MoveTask(ctx.TaskDir, "active")
				if err != nil {
					return fmt.Errorf("[!] Error moving task: %v", err)
				}
				ctx.TaskDir = moved
			}

			root, _ := ProjectRoot()
			worktreePath := filepath.Join(root, "worktrees", ctx.TaskID)
			branchName := "ai/" + ctx.TaskID
			baseBranch := ctx.Git.BaseBranch()

			if _, err := os.Stat(worktreePath); err == nil {
				fmt.Printf("[*] Worktree for %s already exists.\n", ctx.TaskID)
			} else {
				fmt.Printf("[*] Creating worktree in %s (base: %s)...\n", worktreePath, baseBranch)
				if err := ctx.Git.WorktreeAdd(worktreePath, branchName, baseBranch); err != nil {
					return fmt.Errorf("[!] Error creating worktree: %s", err)
				}
			}

			if err := initializeImplementationLog(ctx); err != nil {
				return err
			}
			if err := initializeTrace(ctx); err != nil {
				return err
			}
			ctx.ResultTaskDir = ctx.TaskDir
			fmt.Printf("[+] Task %s initialized and worktree ready.\n", ctx.TaskID)
			return nil
		},
	}
}

func runStartTaskTransition(git GitRunner, taskID string) {
	fmt.Printf("[*] Starting task %s...\n", taskID)
	ctx := &TransitionContext{TaskID: taskID, Git: git}
	if err := runTaskTransition(transitionStart, ctx); err != nil {
		fmt.Println(err)
	}
}

func initializeImplementationLog(ctx *TransitionContext) error {
	logPath, err := ctx.Store.TaskArtifactPath(ctx.TaskDir, "04-implementation-log.yaml")
	if err != nil {
		return fmt.Errorf("[!] Error initializing implementation log: %v", err)
	}
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		summary := "Implementation log initialized."
		if c, ok := ctx.Contract.(map[string]any); ok {
			if s, ok := c["summary"].(string); ok {
				summary = s
			}
		}
		log := map[string]any{
			"task_id": ctx.TaskID,
			"summary": summary,
			"entries": []any{},
		}
		if _, err := ctx.Store.SaveArtifact(ctx.TaskDir, "04-implementation-log.yaml", log); err != nil {
			return fmt.Errorf("[!] Error initializing implementation log: %v", err)
		}
	}
	return nil
}

func initializeTrace(ctx *TransitionContext) error {
	tracePath, err := ctx.Store.TaskArtifactPath(ctx.TaskDir, "07-trace.json")
	if err != nil {
		return fmt.Errorf("[!] Error initializing trace: %v", err)
	}
	if _, err := os.Stat(tracePath); os.IsNotExist(err) {
		summary := "Trace initialized for task."
		execMode := "patch_only"
		if c, ok := ctx.Contract.(map[string]any); ok {
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
			"task_id":           ctx.TaskID,
			"summary":           summary,
			"started_at":        time.Now().UTC().Format(time.RFC3339Nano)[:19] + "Z",
			"execution_mode":    execMode,
			"attempts":          []any{},
			"total_cost_usd":    0.0,
			"violations":        []any{},
			"context_overflows": []any{},
		}
		if _, err := ctx.Store.SaveArtifact(ctx.TaskDir, "07-trace.json", trace); err != nil {
			return fmt.Errorf("[!] Error initializing trace: %v", err)
		}
	}
	return nil
}

func cleanTaskTransition() TaskTransition {
	return TaskTransition{
		Name: transitionClean,
		From: []string{"active", "done", "failed"},
		To:   "done",
		Guard: func(ctx *TransitionContext) error {
			if ctx.Force && ctx.Stash {
				return fmt.Errorf("[!] --force and --stash are mutually exclusive. Pick one: --force discards changes, --stash saves them to a patch.")
			}
			taskDir, err := FindTaskDir(ctx.TaskID, []string{"active", "done", "failed"})
			if err != nil || taskDir == nil {
				return fmt.Errorf("[!] Task %s not found.", ctx.TaskID)
			}
			ctx.TaskDir = taskDir
			return guardCleanParentChildren(ctx)
		},
		Effect: func(ctx *TransitionContext) error {
			root, _ := ProjectRoot()
			worktreePath := filepath.Join(root, "worktrees", ctx.TaskID)
			if _, err := os.Stat(worktreePath); err == nil {
				if err := removeCleanWorktree(ctx, worktreePath); err != nil {
					return err
				}
			}

			ctx.Git.DeleteBranchIfMerged("ai/"+ctx.TaskID, ctx.Git.BaseBranch())
			if ctx.TaskDir.Location == "active" {
				fmt.Printf("[*] Archiving task to done/...\n")
				store, err := DefaultTaskStore()
				if err != nil {
					return fmt.Errorf("[!] Error initializing task store: %v", err)
				}
				moved, err := store.MoveTask(ctx.TaskDir, "done")
				if err != nil {
					return fmt.Errorf("[!] Error archiving task: %v", err)
				}
				ctx.ResultTaskDir = moved
			}
			fmt.Printf("[+] Task %s cleaned up.\n", ctx.TaskID)
			if ctx.ParentTaskID != "" {
				AutoArchiveParentIfComplete(ctx.ParentTaskID)
			}
			return nil
		},
	}
}

func runCleanTaskTransition(git GitRunner, taskID string, force, stash bool) {
	ctx := &TransitionContext{TaskID: taskID, Git: git, Force: force, Stash: stash}
	if err := runTaskTransition(transitionClean, ctx); err != nil {
		fmt.Println(err)
	}
}

func guardCleanParentChildren(ctx *TransitionContext) error {
	specPath := filepath.Join(ctx.TaskDir.Path, "00-spec.yaml")
	if ctx.TaskDir.Location != "active" {
		return nil
	}
	if _, err := os.Stat(specPath); err != nil {
		return nil
	}
	payload, err := LoadArtifactPayload(specPath)
	if err != nil {
		return nil
	}
	spec, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	if p, ok := spec["parent_task"].(string); ok {
		ctx.ParentTaskID = p
	}
	decompObj, ok := spec["decomposition"]
	if !ok {
		return nil
	}
	decomp, ok := decompObj.([]any)
	if !ok {
		return nil
	}
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
		return fmt.Errorf("[!] Parent task %s still has unfinished children: %s\n[!] Clean each child after its human merge before cleaning the parent.", ctx.TaskID, strings.Join(notDone, ", "))
	}
	return nil
}

func removeCleanWorktree(ctx *TransitionContext, worktreePath string) error {
	dirtyPaths, _ := ctx.Git.DirtyPaths(worktreePath)
	dirty := len(dirtyPaths) > 0
	if dirty && !ctx.Force && !ctx.Stash {
		appendCleanupTrace(ctx.TaskDir.Path, "execute", "blocked", "dirty_worktree_detected: "+strings.Join(dirtyPaths, ", "))
		return fmt.Errorf("[!] Worktree %s has uncommitted changes.\n[!] Dirty paths: %s\n[!] Refusing to clean task %s silently. Choose one of:\n      cd %s && git status      # inspect changes\n      cd %s && git commit -am '...'  # commit, then re-run clean\n      quorum task clean %s --stash    # save WIP patch and clean\n      quorum task clean %s --force    # discard WIP and clean", worktreePath, strings.Join(dirtyPaths, ", "), ctx.TaskID, worktreePath, worktreePath, ctx.TaskID, ctx.TaskID)
	}
	if dirty && ctx.Stash {
		fmt.Printf("[*] Saving worktree changes as patch...\n")
		patchPath, err := ctx.Git.SavePatch(worktreePath, ctx.TaskID)
		if err != nil {
			return fmt.Errorf("[!] patch save failed: %s", strings.TrimSpace(err.Error()))
		}
		fmt.Printf("[+] Saved worktree patch: %s\n", patchPath)
		appendCleanupTrace(ctx.TaskDir.Path, "execute", "passed", "stash_path: "+patchPath)
	}
	if ctx.Force && dirty {
		appendCleanupTrace(ctx.TaskDir.Path, "execute", "passed", "force_cleanup: dirty paths "+strings.Join(dirtyPaths, ", "))
		fmt.Printf("[*] Force-removing worktree %s (discarding changes)...\n", worktreePath)
		_ = ctx.Git.WorktreeRemove(worktreePath, true)
	} else if ctx.Stash && dirty {
		fmt.Printf("[*] Removing worktree %s after saving patch...\n", worktreePath)
		_ = ctx.Git.WorktreeRemove(worktreePath, true)
	} else {
		fmt.Printf("[*] Removing worktree %s...\n", worktreePath)
		_ = ctx.Git.WorktreeRemove(worktreePath, false)
	}
	return nil
}

func autoArchiveParentTransition() TaskTransition {
	return TaskTransition{
		Name: transitionAutoArchiveParent,
		From: []string{"active"},
		To:   "done",
		Guard: func(ctx *TransitionContext) error {
			parentDir, err := FindTaskDir(ctx.TaskID, []string{"active"})
			if err != nil || parentDir == nil || parentDir.Location != "active" {
				ctx.Noop = true
				return nil
			}
			ctx.TaskDir = parentDir
			specPath := filepath.Join(parentDir.Path, "00-spec.yaml")
			payload, err := LoadArtifactPayload(specPath)
			if err != nil {
				ctx.Noop = true
				return nil
			}
			spec, ok := payload.(map[string]any)
			if !ok {
				ctx.Noop = true
				return nil
			}
			decompObj := spec["decomposition"]
			if decompObj == nil {
				ctx.Noop = true
				return nil
			}
			decomp, ok := decompObj.([]any)
			if !ok || len(decomp) == 0 {
				ctx.Noop = true
				return nil
			}
			for _, entryAny := range decomp {
				if entry, ok := entryAny.(map[string]any); ok {
					if childID, ok := entry["child_id"].(string); ok && childID != "" {
						c, err := FindTaskDir(childID, nil)
						if err != nil || c == nil || c.Location != "done" {
							ctx.Noop = true
							return nil
						}
					}
				}
			}
			return nil
		},
		Effect: func(ctx *TransitionContext) error {
			if ctx.Noop {
				return nil
			}
			fmt.Printf("[*] All children of %s are in done/; auto-archiving parent...\n", ctx.TaskID)
			store, err := DefaultTaskStore()
			if err != nil {
				return fmt.Errorf("[!] Cannot archive parent %s: %v", ctx.TaskID, err)
			}
			moved, err := store.MoveTask(ctx.TaskDir, "done")
			if err != nil {
				return fmt.Errorf("[!] Cannot archive parent %s: %v", ctx.TaskID, err)
			}
			ctx.ResultTaskDir = moved
			fmt.Printf("[+] Parent task %s archived to done/.\n", ctx.TaskID)
			return nil
		},
	}
}

func runAutoArchiveParentTransition(parentID string) {
	ctx := &TransitionContext{TaskID: parentID}
	if err := runTaskTransition(transitionAutoArchiveParent, ctx); err != nil {
		fmt.Println(err)
	}
}

func retryPrepareTransition() TaskTransition {
	return TaskTransition{
		Name:   transitionRetryPrepare,
		From:   []string{"failed"},
		To:     "active",
		Guard:  guardRetryPrepareTransition,
		Effect: effectRetryPrepareTransition,
	}
}

func guardRetryPrepareTransition(ctx *TransitionContext) error {
	if err := ensureTransitionStore(ctx); err != nil {
		return fmt.Errorf("[!] Error initializing task store: %v", err)
	}
	taskDir, err := ctx.Store.FindTask(ctx.TaskID, "active", "failed")
	if err != nil || taskDir == nil {
		return fmt.Errorf("[!] Task %s not found in active/ or failed/.", ctx.TaskID)
	}
	ctx.TaskDir = taskDir
	if taskDir.Location == "active" {
		ctx.Noop = true
		return nil
	}
	if taskDir.Location != "failed" {
		return fmt.Errorf("[!] Task %s is in %s/; retry preparation only handles failed/ children.", ctx.TaskID, taskDir.Location)
	}
	specPath, err := ctx.Store.TaskArtifactPath(taskDir, "00-spec.yaml")
	if err != nil {
		return fmt.Errorf("[!] Cannot retry %s: invalid 00-spec.yaml: %v", ctx.TaskID, err)
	}
	if _, err := os.Stat(specPath); os.IsNotExist(err) {
		return fmt.Errorf("[!] Cannot retry %s: missing 00-spec.yaml.", ctx.TaskID)
	}
	payload, err := ctx.Store.LoadArtifact(taskDir, "00-spec.yaml")
	if err != nil {
		return fmt.Errorf("[!] Cannot retry %s: invalid 00-spec.yaml: %v", ctx.TaskID, err)
	}
	spec, ok := payload.(map[string]any)
	if !ok {
		return fmt.Errorf("[!] Cannot retry %s: invalid 00-spec.yaml.", ctx.TaskID)
	}
	parentID, ok := spec["parent_task"].(string)
	if !ok || parentID == "" {
		return fmt.Errorf("[!] Retry is only authorized for failed child tasks; %s has no parent_task.", ctx.TaskID)
	}
	ctx.ParentTaskID = parentID
	activeTarget := filepath.Join(ctx.Store.ProjectRoot, ".ai", "tasks", "active", filepath.Base(taskDir.Path))
	if _, err := os.Stat(activeTarget); err == nil {
		return fmt.Errorf("[!] Cannot retry %s: active/%s already exists.", ctx.TaskID, filepath.Base(taskDir.Path))
	}
	tracePath, err := ctx.Store.TaskArtifactPath(taskDir, "07-trace.json")
	if err != nil {
		return fmt.Errorf("[!] Cannot retry %s: invalid 07-trace.json: %v", ctx.TaskID, err)
	}
	if _, err := os.Stat(tracePath); os.IsNotExist(err) {
		return fmt.Errorf("[!] Cannot retry %s: missing 07-trace.json to preserve attempts history.", ctx.TaskID)
	}
	if _, err := ctx.Store.LoadArtifact(taskDir, "07-trace.json"); err != nil {
		return fmt.Errorf("[!] Cannot retry %s: invalid 07-trace.json: %v", ctx.TaskID, err)
	}
	return nil
}

func effectRetryPrepareTransition(ctx *TransitionContext) error {
	if ctx.Noop {
		fmt.Printf("[*] Task %s is already active; retry preparation not needed.\n", ctx.TaskID)
		return nil
	}
	if err := ensureRetryWorktreeWith(ctx.Git, ctx.TaskID); err != nil {
		return err
	}
	if err := restoreParentForChildRetry(ctx.ParentTaskID); err != nil {
		return err
	}
	removed := clearRetryArtifacts(ctx.TaskDir.Path)
	moved, err := ctx.Store.MoveTask(ctx.TaskDir, "active")
	if err != nil {
		return fmt.Errorf("[!] Cannot retry %s: %v", ctx.TaskID, err)
	}
	ctx.ResultTaskDir = moved
	ctx.RemovedArtifacts = removed
	if len(removed) > 0 {
		fmt.Printf("[*] Removed stale retry artifacts for %s: %s\n", ctx.TaskID, strings.Join(removed, ", "))
	}
	fmt.Printf("[+] Failed child task %s restored to active/ for /q-implement retry.\n", ctx.TaskID)
	return nil
}

func runRetryPrepareTransition(git GitRunner, taskID string) bool {
	ctx := &TransitionContext{TaskID: taskID, Git: git}
	if err := runTaskTransition(transitionRetryPrepare, ctx); err != nil {
		fmt.Println(err)
		return false
	}
	return true
}
