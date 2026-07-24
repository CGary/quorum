package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// doctorTaskIDRE extracts a task ID from a task-state directory name. It
// mirrors (without importing) the ID-shape task_manager.go relies on:
// PARENT-NNN optionally followed by a single-lowercase-letter child suffix,
// itself optionally followed by a free-form "-slug" tail. This is a
// deliberate, minimal replication of the convention read from
// task_manager.go's readSpecTaskID/CHILD_ID_RE behavior (see 02-contract.yaml
// forbid.behaviors: doctor.go must not export or modify task_manager.go
// symbols, so it cannot call its unexported helpers directly).
var doctorTaskIDRE = regexp.MustCompile(`^([A-Z][A-Z0-9]*-[0-9]+(?:-[a-z])?)(?:-.*)?$`)

// doctorAIBranchRE matches ai/<ID> branch names as listed by `git branch
// --list ai/*`.
var doctorAIBranchRE = regexp.MustCompile(`^ai/(.+)$`)

// DoctorTaskEntry is one task-state directory found under
// .ai/tasks/{inbox,active,done,failed}.
type DoctorTaskEntry struct {
	ID           string `json:"id"`
	State        string `json:"state"`
	Dir          string `json:"dir"`
	HasBlueprint bool   `json:"has_blueprint"`
}

// DoctorBranchFact is one ai/<ID> branch, with its commit count relative to
// the base branch (informational only; never a delete trigger).
type DoctorBranchFact struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	UniqueCommits int    `json:"unique_commits"`
}

// DoctorPorcelainEntry is one `git worktree list --porcelain` registration
// whose path falls under worktrees/.
type DoctorPorcelainEntry struct {
	Path   string `json:"path"`
	Branch string `json:"branch,omitempty"`
	Exists bool   `json:"exists"`
}

// DoctorFacts is the read-only snapshot CollectDoctorFacts gathers from the
// filesystem and read-only git plumbing. It carries no interpretation; that
// is EvaluateDoctor's job.
type DoctorFacts struct {
	ProjectRoot        string
	BaseBranch         string
	TaskEntries        []DoctorTaskEntry
	WorktreeDirs       []string
	AIBranches         []DoctorBranchFact
	PorcelainWorktrees []DoctorPorcelainEntry
}

// DoctorFinding is one divergence detected by EvaluateDoctor.
type DoctorFinding struct {
	Check   string `json:"check"`
	TaskID  string `json:"task_id,omitempty"`
	Message string `json:"message"`
}

// DoctorReport is the final, deterministic diagnosis.
type DoctorReport struct {
	OK       bool            `json:"ok"`
	Findings []DoctorFinding `json:"findings"`
}

// CollectDoctorFacts is the IO layer: it enumerates .ai/tasks state
// directories, worktrees/ subdirectories, and read-only git plumbing scoped
// strictly to worktrees/<ID> paths and ai/<ID> branches. It never writes,
// mutates, or runs `git worktree prune` or any other mutating command.
func CollectDoctorFacts(projectRoot string) (DoctorFacts, error) {
	facts := DoctorFacts{
		ProjectRoot: projectRoot,
		BaseBranch:  GetBaseBranch(),
	}

	for _, state := range []string{"inbox", "active", "done", "failed"} {
		stateRoot := filepath.Join(projectRoot, ".ai", "tasks", state)
		entries, err := os.ReadDir(stateRoot)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return DoctorFacts{}, err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			dir := filepath.Join(stateRoot, entry.Name())
			id, ok := doctorReadSpecTaskID(dir)
			if !ok {
				id = doctorDeriveTaskID(entry.Name())
			}
			if id == "" {
				continue
			}
			_, blueprintErr := os.Stat(filepath.Join(dir, "01-blueprint.yaml"))
			facts.TaskEntries = append(facts.TaskEntries, DoctorTaskEntry{
				ID:           id,
				State:        state,
				Dir:          dir,
				HasBlueprint: blueprintErr == nil,
			})
		}
	}

	worktreesRoot := filepath.Join(projectRoot, "worktrees")
	if entries, err := os.ReadDir(worktreesRoot); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() || entry.Name() == ".stash" {
				continue
			}
			facts.WorktreeDirs = append(facts.WorktreeDirs, entry.Name())
		}
	} else if !os.IsNotExist(err) {
		return DoctorFacts{}, err
	}

	branches, err := doctorListAIBranches(projectRoot)
	if err != nil {
		return DoctorFacts{}, err
	}
	for _, name := range branches {
		m := doctorAIBranchRE.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		count := doctorUniqueCommitCount(projectRoot, facts.BaseBranch, name)
		facts.AIBranches = append(facts.AIBranches, DoctorBranchFact{
			ID:            m[1],
			Name:          name,
			UniqueCommits: count,
		})
	}

	porcelain, err := doctorListPorcelainWorktrees(projectRoot, worktreesRoot)
	if err != nil {
		return DoctorFacts{}, err
	}
	facts.PorcelainWorktrees = porcelain

	return facts, nil
}

// doctorReadSpecTaskID replicates (never mutates, never calls) the
// task_manager.go readSpecTaskID convention: read the task_id field out of
// 00-spec.yaml, tolerating a trailing comment and surrounding quotes.
func doctorReadSpecTaskID(dir string) (string, bool) {
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

// doctorDeriveTaskID replicates (never mutates) the leading-ID convention
// task_manager.go's directory naming follows, without calling its unexported
// helpers. Used only as a fallback when 00-spec.yaml is missing or lacks a
// task_id field.
func doctorDeriveTaskID(dirName string) string {
	m := doctorTaskIDRE.FindStringSubmatch(dirName)
	if m == nil {
		return ""
	}
	return m[1]
}

// doctorListAIBranches runs read-only `git branch --list ai/*` scoped to the
// project root; never mutates refs.
func doctorListAIBranches(projectRoot string) ([]string, error) {
	out, err := exec.Command("git", "-C", projectRoot, "branch", "--list", "ai/*").Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, &exitErrorWithOutput{err: exitErr, output: string(exitErr.Stderr)}
		}
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "*"))
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		names = append(names, line)
	}
	return names, nil
}

// doctorUniqueCommitCount reports commits on <branch> not reachable from
// <base>, purely informational (mirrors the read-only primitive
// ForceDeleteBranchIfEmpty already uses, without importing or calling it).
func doctorUniqueCommitCount(projectRoot, base, branch string) int {
	out, err := exec.Command("git", "-C", projectRoot, "rev-list", "--count", base+".."+branch).Output()
	if err != nil {
		return 0
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0
	}
	return count
}

// doctorListPorcelainWorktrees runs read-only `git worktree list --porcelain`
// and returns only the entries whose path falls under worktrees/, checking
// on-disk existence for the stale-registration check. It never runs `git
// worktree prune`.
func doctorListPorcelainWorktrees(projectRoot, worktreesRoot string) ([]DoctorPorcelainEntry, error) {
	out, err := exec.Command("git", "-C", projectRoot, "worktree", "list", "--porcelain").Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, &exitErrorWithOutput{err: exitErr, output: string(exitErr.Stderr)}
		}
		return nil, err
	}

	absWorktreesRoot, err := filepath.Abs(worktreesRoot)
	if err != nil {
		return nil, err
	}

	var results []DoctorPorcelainEntry
	var currentPath, currentBranch string
	flush := func() {
		if currentPath == "" {
			return
		}
		absPath, err := filepath.Abs(currentPath)
		if err != nil {
			absPath = currentPath
		}
		rel, err := filepath.Rel(absWorktreesRoot, absPath)
		if err != nil || strings.HasPrefix(rel, "..") {
			return
		}
		_, statErr := os.Stat(currentPath)
		results = append(results, DoctorPorcelainEntry{
			Path:   currentPath,
			Branch: currentBranch,
			Exists: statErr == nil,
		})
	}
	for _, line := range strings.Split(string(out), "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			currentPath = strings.TrimPrefix(line, "worktree ")
			currentBranch = ""
		case strings.HasPrefix(line, "branch "):
			ref := strings.TrimPrefix(line, "branch ")
			currentBranch = strings.TrimPrefix(ref, "refs/heads/")
		case line == "":
			flush()
			currentPath = ""
			currentBranch = ""
		}
	}
	flush()
	return results, nil
}

// exitErrorWithOutput adapts an *exec.ExitError to carry stderr text in
// Error() for more actionable diagnostics; it is never used to retry or
// escalate to a mutating command.
type exitErrorWithOutput struct {
	err    error
	output string
}

func (e *exitErrorWithOutput) Error() string {
	if strings.TrimSpace(e.output) != "" {
		return strings.TrimSpace(e.output)
	}
	return e.err.Error()
}

// EvaluateDoctor is the pure evaluator: it never touches disk or git, and
// computes the six divergence checks deterministically from facts already
// collected.
func EvaluateDoctor(facts DoctorFacts) DoctorReport {
	worktreeSet := map[string]bool{}
	for _, w := range facts.WorktreeDirs {
		worktreeSet[w] = true
	}
	branchByID := map[string]DoctorBranchFact{}
	for _, b := range facts.AIBranches {
		branchByID[b.ID] = b
	}

	activeByID := map[string]DoctorTaskEntry{}
	knownByID := map[string]bool{}    // any state dir at all
	terminalByID := map[string]bool{} // done or failed
	locationsByID := map[string][]string{}
	for _, e := range facts.TaskEntries {
		knownByID[e.ID] = true
		locationsByID[e.ID] = append(locationsByID[e.ID], e.State+"/"+filepath.Base(e.Dir))
		if e.State == "active" {
			activeByID[e.ID] = e
		}
		if e.State == "done" || e.State == "failed" {
			terminalByID[e.ID] = true
		}
	}

	findings := []DoctorFinding{}

	// AC-2: active task with 01-blueprint.yaml but no worktrees/<ID>.
	for _, e := range facts.TaskEntries {
		if e.State != "active" || !e.HasBlueprint {
			continue
		}
		if !worktreeSet[e.ID] {
			findings = append(findings, DoctorFinding{
				Check:   "missing_worktree",
				TaskID:  e.ID,
				Message: "active task '" + e.ID + "' has 01-blueprint.yaml but no worktrees/" + e.ID,
			})
		}
	}

	// AC-3: worktrees/<ID> with no matching active task (and not accounted for
	// by a done/failed residue, which AC-6 reports instead).
	for _, w := range facts.WorktreeDirs {
		if _, ok := activeByID[w]; ok {
			continue
		}
		if terminalByID[w] {
			continue
		}
		findings = append(findings, DoctorFinding{
			Check:   "orphaned_worktree",
			TaskID:  w,
			Message: "worktrees/" + w + " has no matching task in .ai/tasks/active/",
		})
	}

	// AC-4: ai/<ID> branch with neither worktree nor active task (and not a
	// done/failed residue, which AC-6 reports instead).
	for _, b := range facts.AIBranches {
		if worktreeSet[b.ID] {
			continue
		}
		if _, ok := activeByID[b.ID]; ok {
			continue
		}
		if terminalByID[b.ID] {
			continue
		}
		commitsMsg := "no unique commits vs " + facts.BaseBranch
		if b.UniqueCommits > 0 {
			commitsMsg = strconv.Itoa(b.UniqueCommits) + " unique commit(s) vs " + facts.BaseBranch
		}
		findings = append(findings, DoctorFinding{
			Check:   "orphaned_branch",
			TaskID:  b.ID,
			Message: "branch " + b.Name + " has no worktree and no active task (" + commitsMsg + ")",
		})
	}

	// AC-5: stale git worktree registration whose path no longer exists.
	for _, p := range facts.PorcelainWorktrees {
		if p.Exists {
			continue
		}
		findings = append(findings, DoctorFinding{
			Check:   "stale_worktree_registration",
			Message: "git worktree registration '" + p.Path + "' has no directory on disk; consider 'git worktree prune' (not run automatically)",
		})
	}

	// AC-6: done/failed task that still has a live worktree and/or branch.
	for _, e := range facts.TaskEntries {
		if e.State != "done" && e.State != "failed" {
			continue
		}
		hasWorktree := worktreeSet[e.ID]
		branch, hasBranch := branchByID[e.ID]
		if !hasWorktree && !hasBranch {
			continue
		}
		var residue []string
		if hasWorktree {
			residue = append(residue, "worktrees/"+e.ID)
		}
		if hasBranch {
			residue = append(residue, branch.Name)
		}
		findings = append(findings, DoctorFinding{
			Check:   "incomplete_clean",
			TaskID:  e.ID,
			Message: "task '" + e.ID + "' is in " + e.State + "/ but still has: " + strings.Join(residue, ", "),
		})
	}

	// AC-7: same task ID present under more than one state dir.
	var dupIDs []string
	for id, locs := range locationsByID {
		if len(locs) > 1 {
			dupIDs = append(dupIDs, id)
		}
	}
	sort.Strings(dupIDs)
	for _, id := range dupIDs {
		locs := append([]string(nil), locationsByID[id]...)
		sort.Strings(locs)
		findings = append(findings, DoctorFinding{
			Check:   "duplicate_task_id",
			TaskID:  id,
			Message: "task id '" + id + "' found in multiple locations: " + strings.Join(locs, ", "),
		})
	}

	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Check != findings[j].Check {
			return findings[i].Check < findings[j].Check
		}
		if findings[i].TaskID != findings[j].TaskID {
			return findings[i].TaskID < findings[j].TaskID
		}
		return findings[i].Message < findings[j].Message
	})

	return DoctorReport{
		OK:       len(findings) == 0,
		Findings: findings,
	}
}
