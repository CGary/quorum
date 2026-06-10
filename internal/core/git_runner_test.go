package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var _ GitRunner = execGitRunner{}

// Pin AC-1 at compile time: the destructive operations keep their safety
// decision visible in the port signatures.
var (
	_ func(path, branch, base string) error = execGitRunner{}.WorktreeAdd
	_ func(path, branch string) error       = execGitRunner{}.WorktreeAttach
	_ func(path string, force bool) error   = execGitRunner{}.WorktreeRemove
	_ func(branch, base string) bool        = execGitRunner{}.DeleteBranchIfMerged
	_ func(branch, base string) bool        = execGitRunner{}.ForceDeleteBranchIfEmpty
)

// AC-3: direct git invocations in task_manager.go are reduced to the
// documented exclusion (ProjectRoot bootstrap); everything else goes through
// the GitRunner port.
func TestTaskManagerDirectGitCallsReducedToProjectRootBootstrap(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(sourceRoot(t), "internal", "core", "task_manager.go"))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(src), `exec.Command("git"`); n != 1 {
		t.Fatalf("task_manager.go has %d direct git exec call-sites, want 1 (ProjectRoot bootstrap only)", n)
	}
}

func TestExecGitRunnerWorktreeAndBranchSemantics(t *testing.T) {
	root := initGitRepo(t)
	chdir(t, root)
	g := execGitRunner{}

	if base := g.BaseBranch(); base != "main" {
		t.Fatalf("BaseBranch = %q, want main", base)
	}

	wt := filepath.Join(root, "worktrees", "FEAT-050")
	if err := g.WorktreeAdd(wt, "ai/FEAT-050", "main"); err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}
	if !g.BranchExists("ai/FEAT-050") || !g.RefExists("ai/FEAT-050") {
		t.Fatal("new branch should exist via show-ref and rev-parse")
	}
	if g.BranchExists("ai/missing") || g.RefExists("ai/missing") {
		t.Fatal("missing branch must not exist")
	}

	if paths, err := g.DirtyPaths(wt); err != nil || len(paths) != 0 {
		t.Fatalf("clean worktree DirtyPaths = %v, %v", paths, err)
	}
	if err := os.WriteFile(filepath.Join(wt, "wip.txt"), []byte("wip\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if paths, err := g.DirtyPaths(wt); err != nil || len(paths) != 1 || paths[0] != "wip.txt" {
		t.Fatalf("dirty worktree DirtyPaths = %v, %v", paths, err)
	}

	if err := g.WorktreeRemove(wt, false); err == nil {
		t.Fatal("non-force remove of dirty worktree must fail")
	}
	if err := g.WorktreeRemove(wt, true); err != nil {
		t.Fatalf("force remove: %v", err)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatalf("worktree still present: %v", err)
	}

	// Branch has no own commits: back's policy force-deletes it.
	out := captureStdout(t, func() {
		if !g.ForceDeleteBranchIfEmpty("ai/FEAT-050", "main") {
			t.Fatal("empty branch should be force-deleted")
		}
	})
	if !strings.Contains(out, "Removed empty branch ai/FEAT-050") {
		t.Fatalf("output = %q", out)
	}
	if g.BranchExists("ai/FEAT-050") {
		t.Fatal("branch should be gone after force delete")
	}

	// WorktreeAttach re-attaches an existing branch without -b.
	run(t, root, "git", "branch", "ai/FEAT-051", "main")
	wt2 := filepath.Join(root, "worktrees", "FEAT-051")
	if err := g.WorktreeAttach(wt2, "ai/FEAT-051"); err != nil {
		t.Fatalf("WorktreeAttach: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wt2, "feature.txt"), []byte("feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, wt2, "git", "add", "feature.txt")
	run(t, wt2, "git", "commit", "-q", "-m", "feature commit")
	if err := g.WorktreeRemove(wt2, false); err != nil {
		t.Fatalf("remove committed worktree: %v", err)
	}

	// Unmerged branch with own commits: both deletion policies preserve it.
	if g.ForceDeleteBranchIfEmpty("ai/FEAT-051", "main") {
		t.Fatal("branch with own commits must not be force-deleted")
	}
	if g.DeleteBranchIfMerged("ai/FEAT-051", "main") {
		t.Fatal("unmerged branch must not be deleted")
	}
	if !g.BranchExists("ai/FEAT-051") {
		t.Fatal("unmerged branch was deleted")
	}

	// Once merged, clean's policy deletes it with -d.
	run(t, root, "git", "merge", "-q", "ai/FEAT-051")
	if !g.DeleteBranchIfMerged("ai/FEAT-051", "main") {
		t.Fatal("merged branch should be deleted")
	}
	if g.BranchExists("ai/FEAT-051") {
		t.Fatal("merged branch still exists")
	}
}

// AC-4: SavePatch preserves intent-to-add (git add -N . before
// diff --binary HEAD) so new untracked files never disappear from the patch.
func TestExecGitRunnerSavePatchPreservesIntentToAdd(t *testing.T) {
	root := initGitRepo(t)
	chdir(t, root)
	g := execGitRunner{}

	wt := filepath.Join(root, "worktrees", "FEAT-060")
	if err := g.WorktreeAdd(wt, "ai/FEAT-060", "main"); err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}

	if _, err := g.SavePatch(wt, "FEAT-060"); err == nil || err.Error() != "no patch content produced" {
		t.Fatalf("clean worktree SavePatch error = %v, want empty-patch error", err)
	}

	if err := os.WriteFile(filepath.Join(wt, "brand-new.txt"), []byte("untracked content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	patchPath, err := g.SavePatch(wt, "FEAT-060")
	if err != nil {
		t.Fatalf("SavePatch: %v", err)
	}
	if !strings.HasPrefix(filepath.Base(patchPath), "FEAT-060-") {
		t.Fatalf("patch path = %q", patchPath)
	}
	patch, err := os.ReadFile(patchPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(patch), "brand-new.txt") || !strings.Contains(string(patch), "+untracked content") {
		t.Fatalf("patch lost the untracked file:\n%s", patch)
	}
}
