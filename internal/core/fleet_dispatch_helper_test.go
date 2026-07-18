package core

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if mode := os.Getenv("FLEET_FAKE_MODE"); mode != "" {
		os.Exit(runFakeDelegate(mode))
	}
	os.Exit(m.Run())
}

func runFakeDelegate(mode string) int {
	switch mode {
	case "success_diff":
		_ = os.WriteFile("delegate_change.txt", []byte("delegate change\n"), 0o644)
		fmt.Println("NOTES: applied the requested change")
		return 0
	case "diff_then_fail":
		_ = os.WriteFile("delegate_partial.txt", []byte("partial work\n"), 0o644)
		fmt.Fprintln(os.Stderr, "delegate produced a partial change then failed")
		return 1
	case "success_diff_unreadable":
		// Modifies a normal file (so staging captures real content), then drops
		// an unreadable file into the worktree so `git add -A` fails partway
		// through staging while leaving the already-scanned index untouched --
		// this reproduces a real staging_failed with a forensic ref that IS
		// capturable (write-tree only reads the current index, not the broken
		// file), exercising the case that must still skip reset --hard.
		_ = os.WriteFile("delegate_change.txt", []byte("delegate change\n"), 0o644)
		_ = os.WriteFile("delegate_unreadable.txt", []byte("x"), 0o644)
		_ = os.Chmod("delegate_unreadable.txt", 0o000)
		fmt.Println("NOTES: applied the requested change")
		return 0
	case "diff_then_fail_readonly_wt":
		// Modifies an existing tracked file (seed.txt) then makes the worktree
		// directory read-only, so `git add -A`/diff/forensic capture (which only
		// read the working tree and write into the separate git dir) still
		// succeed, but a later `git reset --hard` -- which must unlink+recreate
		// seed.txt in the working tree -- fails with a real permission error.
		_ = os.WriteFile("seed.txt", []byte("modified by delegate\n"), 0o644)
		cwd, _ := os.Getwd()
		_ = os.Chmod(cwd, 0o555)
		fmt.Fprintln(os.Stderr, "delegate produced a partial change then failed")
		return 1
	case "fail_empty":
		fmt.Fprintln(os.Stderr, "delegate could not complete the task")
		return 3
	case "noop":
		return 0
	case "garbage":
		fmt.Println("<<< this is not valid json at all >>>")
		return 0
	case "quota":
		fmt.Println("boom: model not supported when using Codex with a ChatGPT account")
		return 1
	case "blocked":
		fmt.Println("BLOCKED: missing_file=cmd/new_helper.go; reason=needs a helper not in touch; severity=critical")
		return 0
	case "timeout_sleep":
		time.Sleep(60 * time.Second)
		return 0
	case "group_child":
		child := exec.Command(os.Args[0])
		// Replace (never duplicate) FLEET_FAKE_MODE: glibc getenv returns the
		// first match, so a duplicate key would re-spawn group_child forever.
		child.Env = append(envWithout(os.Environ(), "FLEET_FAKE_MODE"), "FLEET_FAKE_MODE=orphan_sleep")
		if err := child.Start(); err == nil && child.Process != nil {
			if pf := os.Getenv("FLEET_FAKE_PIDFILE"); pf != "" {
				_ = os.WriteFile(pf, []byte(strconv.Itoa(child.Process.Pid)), 0o644)
			}
		}
		signal.Ignore(syscall.SIGTERM)
		time.Sleep(60 * time.Second)
		return 0
	case "orphan_sleep":
		signal.Ignore(syscall.SIGTERM)
		time.Sleep(120 * time.Second)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown fake mode %q\n", mode)
		return 2
	}
}
func envWithout(env []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if !strings.HasPrefix(kv, prefix) {
			out = append(out, kv)
		}
	}
	return out
}
