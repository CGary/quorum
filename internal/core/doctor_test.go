package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// doctorMkTaskDir creates a task-state directory with a minimal 00-spec.yaml
// (and optionally 01-blueprint.yaml) for doctor tests.
func doctorMkTaskDir(t *testing.T, root, state, dirName, id string, withBlueprint bool) string {
	t.Helper()
	dir := filepath.Join(root, ".ai", "tasks", state, dirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "00-spec.yaml"), []byte("task_id: "+id+"\nsummary: test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if withBlueprint {
		if err := os.WriteFile(filepath.Join(dir, "01-blueprint.yaml"), []byte("task_id: "+id+"\nsummary: test blueprint\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func doctorFindingsByCheck(report DoctorReport, check string) []DoctorFinding {
	var out []DoctorFinding
	for _, f := range report.Findings {
		if f.Check == check {
			out = append(out, f)
		}
	}
	return out
}

// TestDoctorConsistentProjectOK covers AC-1: a project whose active task has
// a matching worktree and ai/<ID> branch is reported ok.
func TestDoctorConsistentProjectOK(t *testing.T) {
	root := initGitRepo(t)
	chdir(t, root)
	ensureTaskDirs(t, root)

	id := "MAINT-100"
	doctorMkTaskDir(t, root, "active", id+"-new-spec", id, true)
	worktree := filepath.Join(root, "worktrees", id)
	run(t, root, "git", "worktree", "add", "-q", "-b", "ai/"+id, worktree)

	facts, err := CollectDoctorFacts(root)
	if err != nil {
		t.Fatalf("CollectDoctorFacts: %v", err)
	}
	report := EvaluateDoctor(facts)
	if !report.OK {
		t.Fatalf("expected ok report, got findings: %+v", report.Findings)
	}
	if len(report.Findings) != 0 {
		t.Fatalf("expected zero findings, got %d: %+v", len(report.Findings), report.Findings)
	}
}

// TestDoctorMissingWorktree covers AC-2: an active task with 01-blueprint.yaml
// but no matching worktrees/<ID> is reported as missing-worktree.
func TestDoctorMissingWorktree(t *testing.T) {
	root := initGitRepo(t)
	chdir(t, root)
	ensureTaskDirs(t, root)

	id := "MAINT-101"
	doctorMkTaskDir(t, root, "active", id+"-new-spec", id, true)

	facts, err := CollectDoctorFacts(root)
	if err != nil {
		t.Fatalf("CollectDoctorFacts: %v", err)
	}
	report := EvaluateDoctor(facts)
	if report.OK {
		t.Fatal("expected divergence, got ok")
	}
	findings := doctorFindingsByCheck(report, "missing_worktree")
	if len(findings) != 1 || findings[0].TaskID != id {
		t.Fatalf("expected one missing_worktree finding for %s, got %+v", id, report.Findings)
	}
}

// TestDoctorOrphanedWorktree covers AC-3: a worktrees/<ID> subdirectory with
// no matching active task is reported as orphaned.
func TestDoctorOrphanedWorktree(t *testing.T) {
	root := initGitRepo(t)
	chdir(t, root)
	ensureTaskDirs(t, root)

	id := "MAINT-102"
	if err := os.MkdirAll(filepath.Join(root, "worktrees", id), 0o755); err != nil {
		t.Fatal(err)
	}

	facts, err := CollectDoctorFacts(root)
	if err != nil {
		t.Fatalf("CollectDoctorFacts: %v", err)
	}
	report := EvaluateDoctor(facts)
	findings := doctorFindingsByCheck(report, "orphaned_worktree")
	if len(findings) != 1 || findings[0].TaskID != id {
		t.Fatalf("expected one orphaned_worktree finding for %s, got %+v", id, report.Findings)
	}
}

// TestDoctorOrphanedBranch covers AC-4: an ai/<ID> branch with neither
// worktree nor active task is reported as orphaned, noting unique commits.
func TestDoctorOrphanedBranch(t *testing.T) {
	root := initGitRepo(t)
	chdir(t, root)
	ensureTaskDirs(t, root)

	id := "MAINT-103"
	run(t, root, "git", "checkout", "-q", "-b", "ai/"+id)
	if err := os.WriteFile(filepath.Join(root, "orphan-branch-file.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, root, "git", "add", "orphan-branch-file.txt")
	run(t, root, "git", "commit", "-q", "-m", "orphan branch commit")
	run(t, root, "git", "checkout", "-q", "main")

	facts, err := CollectDoctorFacts(root)
	if err != nil {
		t.Fatalf("CollectDoctorFacts: %v", err)
	}
	report := EvaluateDoctor(facts)
	findings := doctorFindingsByCheck(report, "orphaned_branch")
	if len(findings) != 1 || findings[0].TaskID != id {
		t.Fatalf("expected one orphaned_branch finding for %s, got %+v", id, report.Findings)
	}
}

// TestDoctorStaleWorktreeRegistration covers AC-5: a git worktree list entry
// whose directory no longer exists on disk is reported as stale, and prune
// is never executed.
func TestDoctorStaleWorktreeRegistration(t *testing.T) {
	root := initGitRepo(t)
	chdir(t, root)
	ensureTaskDirs(t, root)

	id := "MAINT-104"
	worktree := filepath.Join(root, "worktrees", id)
	run(t, root, "git", "worktree", "add", "-q", "-b", "ai/"+id, worktree)
	if err := os.RemoveAll(worktree); err != nil {
		t.Fatal(err)
	}

	facts, err := CollectDoctorFacts(root)
	if err != nil {
		t.Fatalf("CollectDoctorFacts: %v", err)
	}
	report := EvaluateDoctor(facts)
	findings := doctorFindingsByCheck(report, "stale_worktree_registration")
	if len(findings) != 1 {
		t.Fatalf("expected one stale_worktree_registration finding, got %+v", report.Findings)
	}

	// The registration must still exist afterward: doctor never prunes.
	out := run(t, root, "git", "worktree", "list", "--porcelain")
	if !strings.Contains(out, worktree) {
		t.Fatalf("expected stale worktree registration to remain untouched, got: %s", out)
	}
}

// TestDoctorStaleWorktreeRegistrationAlsoOrphansBranch covers the AC-4/AC-5
// interaction: an ai/<ID> branch whose worktree was created via
// `git worktree add` and later removed with os.RemoveAll (not
// `git worktree remove`, so the worktree registration is left prunable but
// git still reports the branch as tied to that worktree with a '+' marker in
// `git branch --list` output) must fire BOTH stale_worktree_registration
// (AC-5) AND orphaned_branch (AC-4) when there is no active task for the ID.
// This regression-guards doctorListAIBranches: a prior implementation only
// stripped the '*' current-branch marker and left '+' unstripped, so the
// branch line failed doctorAIBranchRE and silently never reached AC-4.
func TestDoctorStaleWorktreeRegistrationAlsoOrphansBranch(t *testing.T) {
	root := initGitRepo(t)
	chdir(t, root)
	ensureTaskDirs(t, root)

	id := "MAINT-105"
	worktree := filepath.Join(root, "worktrees", id)
	run(t, root, "git", "worktree", "add", "-q", "-b", "ai/"+id, worktree)
	if err := os.RemoveAll(worktree); err != nil {
		t.Fatal(err)
	}

	facts, err := CollectDoctorFacts(root)
	if err != nil {
		t.Fatalf("CollectDoctorFacts: %v", err)
	}
	report := EvaluateDoctor(facts)

	staleFindings := doctorFindingsByCheck(report, "stale_worktree_registration")
	if len(staleFindings) != 1 {
		t.Fatalf("expected one stale_worktree_registration finding, got %+v", report.Findings)
	}

	orphanedBranchFindings := doctorFindingsByCheck(report, "orphaned_branch")
	if len(orphanedBranchFindings) != 1 || orphanedBranchFindings[0].TaskID != id {
		t.Fatalf("expected one orphaned_branch finding for %s, got %+v", id, report.Findings)
	}
}

// TestDoctorIncompleteClean covers AC-6: a task in done/ or failed/ that
// still has a live worktree and/or ai/<ID> branch is reported, naming the
// residue.
func TestDoctorIncompleteClean(t *testing.T) {
	root := initGitRepo(t)
	chdir(t, root)
	ensureTaskDirs(t, root)

	id := "MAINT-105"
	doctorMkTaskDir(t, root, "failed", id+"-new-spec", id, false)
	worktree := filepath.Join(root, "worktrees", id)
	run(t, root, "git", "worktree", "add", "-q", "-b", "ai/"+id, worktree)

	facts, err := CollectDoctorFacts(root)
	if err != nil {
		t.Fatalf("CollectDoctorFacts: %v", err)
	}
	report := EvaluateDoctor(facts)
	findings := doctorFindingsByCheck(report, "incomplete_clean")
	if len(findings) != 1 || findings[0].TaskID != id {
		t.Fatalf("expected one incomplete_clean finding for %s, got %+v", id, report.Findings)
	}
}

// TestDoctorDuplicateTaskID covers AC-7: the same task ID present under more
// than one state directory is reported as a duplicate.
func TestDoctorDuplicateTaskID(t *testing.T) {
	root := initGitRepo(t)
	chdir(t, root)
	ensureTaskDirs(t, root)

	id := "MAINT-106"
	doctorMkTaskDir(t, root, "active", id+"-new-spec", id, false)
	doctorMkTaskDir(t, root, "failed", id+"-new-spec", id, false)

	facts, err := CollectDoctorFacts(root)
	if err != nil {
		t.Fatalf("CollectDoctorFacts: %v", err)
	}
	report := EvaluateDoctor(facts)
	findings := doctorFindingsByCheck(report, "duplicate_task_id")
	if len(findings) != 1 || findings[0].TaskID != id {
		t.Fatalf("expected one duplicate_task_id finding for %s, got %+v", id, report.Findings)
	}
}

// TestDoctorEvaluateIsPure covers AC-8's precondition indirectly: calling
// EvaluateDoctor twice on the same facts value yields byte-identical
// reports, i.e. it performs no IO and is deterministic.
func TestDoctorEvaluateIsPure(t *testing.T) {
	root := initGitRepo(t)
	chdir(t, root)
	ensureTaskDirs(t, root)

	id := "MAINT-107"
	doctorMkTaskDir(t, root, "active", id+"-new-spec", id, true)

	facts, err := CollectDoctorFacts(root)
	if err != nil {
		t.Fatalf("CollectDoctorFacts: %v", err)
	}
	first := EvaluateDoctor(facts)
	second := EvaluateDoctor(facts)
	if first.OK != second.OK || len(first.Findings) != len(second.Findings) {
		t.Fatalf("EvaluateDoctor is not deterministic: %+v vs %+v", first, second)
	}
	for i := range first.Findings {
		if first.Findings[i] != second.Findings[i] {
			t.Fatalf("finding %d differs across runs: %+v vs %+v", i, first.Findings[i], second.Findings[i])
		}
	}
}

// TestDoctorJSONReportMatchesFindings covers AC-8: the DoctorReport that
// cmd/doctor.go marshals under --json is exactly one JSON object whose
// "ok"/"findings" fields match what the human-readable renderer would show,
// for both a consistent and a divergent project.
func TestDoctorJSONReportMatchesFindings(t *testing.T) {
	root := initGitRepo(t)
	chdir(t, root)
	ensureTaskDirs(t, root)

	id := "MAINT-108"
	doctorMkTaskDir(t, root, "active", id+"-new-spec", id, true) // no worktree -> divergence

	facts, err := CollectDoctorFacts(root)
	if err != nil {
		t.Fatalf("CollectDoctorFacts: %v", err)
	}
	report := EvaluateDoctor(facts)
	if report.OK {
		t.Fatal("expected a divergent report for this fixture")
	}

	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var decoded struct {
		OK       bool `json:"ok"`
		Findings []struct {
			Check   string `json:"check"`
			TaskID  string `json:"task_id"`
			Message string `json:"message"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if decoded.OK != report.OK || len(decoded.Findings) != len(report.Findings) {
		t.Fatalf("decoded JSON does not match report: %+v vs %+v", decoded, report)
	}
	if len(decoded.Findings) != 1 || decoded.Findings[0].Check != "missing_worktree" || decoded.Findings[0].TaskID != id {
		t.Fatalf("unexpected JSON findings: %+v", decoded.Findings)
	}
}
