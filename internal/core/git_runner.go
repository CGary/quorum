package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// GitRunner is the port for the git/worktree side effects performed by the
// task-state transitions in task_manager.go (start, clean, back,
// retry-prepare). It wraps only the operations Quorum actually issues; it does
// not simulate git. Safety decisions stay visible in the signatures: the two
// worktree-add modes are distinct methods, branch deletion is split by policy
// (merged-only vs force-if-empty), and worktree removal takes an explicit
// force flag.
//
// Deliberately OUT of the port: ProjectRoot() (rev-parse --show-toplevel) and
// remote.origin.url reading in quorum_config.go — both are bootstrap calls
// that run before any dependency injection is possible.
//
// Error normalization: methods returning error wrap the failed git command's
// trimmed combined output (failed command), or a structured message for the
// non-command failures ("no patch content produced" for an empty patch).
// DirtyPaths reports the dirty-worktree state as data; the boolean methods
// (missing branch, unresolvable merge-base) normalize those conditions to
// false exactly as the previous inline call-sites did.
type GitRunner interface {
	// BaseBranch detects the base branch: origin/HEAD, then the first existing
	// branch among main/master/develop/trunk, then the current branch, then
	// the literal "main".
	BaseBranch() string
	// BranchExists reports whether refs/heads/<branch> exists (show-ref
	// --verify). Stricter than RefExists: it never matches tags or remote refs.
	BranchExists(branch string) bool
	// RefExists reports whether <ref> resolves via rev-parse --verify. Used by
	// the retry-prepare re-attach check; kept separate from BranchExists
	// because rev-parse also resolves tags and ambiguous refs, and unifying
	// them would change that call-site's edge-case semantics.
	RefExists(ref string) bool
	// WorktreeAdd creates worktree <path> on the NEW branch <branch> from
	// <base> (worktree add <path> -b <branch> <base>). Used by start and by
	// retry-prepare when the task branch no longer exists.
	WorktreeAdd(path, branch, base string) error
	// WorktreeAttach creates worktree <path> re-attaching the EXISTING branch
	// <branch> (worktree add <path> <branch>, no -b). Used by retry-prepare.
	WorktreeAttach(path, branch string) error
	// WorktreeRemove removes worktree <path>; force discards uncommitted
	// changes (worktree remove [--force] <path>). The caller decides force.
	WorktreeRemove(path string, force bool) error
	// DirtyPaths lists paths reported dirty by status --porcelain in the
	// worktree (rename lines collapse to their destination path).
	DirtyPaths(worktreePath string) ([]string, error)
	// SavePatch stashes the worktree's full diff (including untracked files,
	// via the intent-to-add step: add -N . before diff --binary HEAD) into
	// worktrees/.stash/<taskID>-<timestamp>.patch and returns the patch path.
	SavePatch(worktreePath, taskID string) (string, error)
	// DeleteBranchIfMerged deletes <branch> only when merge-base --is-ancestor
	// confirms it is merged into <base> (branch -d). Used by clean.
	DeleteBranchIfMerged(branch, base string) bool
	// ForceDeleteBranchIfEmpty force-deletes <branch> (branch -D) only when
	// rev-list --count <base>..<branch> is 0, i.e. no own commits. Used by back.
	ForceDeleteBranchIfEmpty(branch, base string) bool
}

// execGitRunner is the production adapter: it issues the exact git commands
// the task_manager.go call-sites issued inline, including their user-facing
// outcome messages for the branch-deletion policies.
//
// Documented normalization: WorktreeAdd and WorktreeRemove always run with
// -C <projectRoot>. Previously StartTask's add and BackTask's remove relied on
// the process cwd while CleanTask's remove and retry-prepare's add used
// -C <root>; behavior is identical because every path passed is absolute and
// the process cwd is always inside the same repository ProjectRoot resolves.
type execGitRunner struct{}

func (execGitRunner) BaseBranch() string {
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

func (execGitRunner) BranchExists(branch string) bool {
	root, _ := ProjectRoot()
	return exec.Command("git", "-C", root, "show-ref", "--verify", "refs/heads/"+branch).Run() == nil
}

func (execGitRunner) RefExists(ref string) bool {
	root, _ := ProjectRoot()
	return exec.Command("git", "-C", root, "rev-parse", "--verify", ref).Run() == nil
}

func (execGitRunner) WorktreeAdd(path, branch, base string) error {
	root, _ := ProjectRoot()
	out, err := exec.Command("git", "-C", root, "worktree", "add", path, "-b", branch, base).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return nil
}

func (execGitRunner) WorktreeAttach(path, branch string) error {
	root, _ := ProjectRoot()
	out, err := exec.Command("git", "-C", root, "worktree", "add", path, branch).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return nil
}

func (execGitRunner) WorktreeRemove(path string, force bool) error {
	root, _ := ProjectRoot()
	args := []string{"-C", root, "worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, path)
	out, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return nil
}

func (execGitRunner) DirtyPaths(worktreePath string) ([]string, error) {
	out, err := exec.Command("git", "-C", worktreePath, "status", "--porcelain").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		path := strings.TrimSpace(line[2:])
		if strings.Contains(path, " -> ") {
			parts := strings.Split(path, " -> ")
			path = parts[len(parts)-1]
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func (execGitRunner) SavePatch(worktreePath, taskID string) (string, error) {
	root, err := ProjectRoot()
	if err != nil {
		return "", err
	}
	stashDir := filepath.Join(root, "worktrees", ".stash")
	if err := os.MkdirAll(stashDir, 0755); err != nil {
		return "", err
	}
	patchPath := filepath.Join(stashDir, fmt.Sprintf("%s-%s.patch", taskID, time.Now().UTC().Format("20060102T150405Z")))
	_ = exec.Command("git", "-C", worktreePath, "add", "-N", ".").Run()
	out, err := exec.Command("git", "-C", worktreePath, "diff", "--binary", "HEAD").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	if len(out) == 0 {
		return "", fmt.Errorf("no patch content produced")
	}
	if err := os.WriteFile(patchPath, out, 0644); err != nil {
		return "", err
	}
	return patchPath, nil
}

func (g execGitRunner) DeleteBranchIfMerged(branch, base string) bool {
	if !g.BranchExists(branch) {
		fmt.Printf("[*] Branch %s is absent; skipping local branch cleanup.\n", branch)
		return false
	}
	root, _ := ProjectRoot()
	err := exec.Command("git", "-C", root, "merge-base", "--is-ancestor", branch, base).Run()
	if err == nil {
		out, err2 := exec.Command("git", "-C", root, "branch", "-d", branch).CombinedOutput()
		if err2 == nil {
			fmt.Printf("[+] Deleted merged local branch %s.\n", branch)
			return true
		}
		fmt.Printf("[!] Could not delete merged local branch %s: %s\n", branch, strings.TrimSpace(string(out)))
		return false
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		fmt.Printf("[!] Preserving local branch %s; it has commits not merged into %s.\n", branch, base)
		fmt.Printf("    After merging, delete it manually with: git branch -d %s\n", branch)
		return false
	}
	fmt.Printf("[!] Could not determine whether %s is merged into %s; preserving branch. \n", branch, base)
	return false
}

func (execGitRunner) ForceDeleteBranchIfEmpty(branch, base string) bool {
	out, err := exec.Command("git", "rev-list", "--count", base+".."+branch).CombinedOutput()
	if err != nil {
		return false
	}
	if strings.TrimSpace(string(out)) == "0" {
		exec.Command("git", "branch", "-D", branch).Run()
		fmt.Printf("[+] Removed empty branch %s.\n", branch)
		return true
	}
	fmt.Printf("[!] Branch %s has commits; not deleted. Use 'git branch -D %s' if you really want to drop them.\n", branch, branch)
	return false
}
