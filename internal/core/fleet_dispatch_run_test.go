package core

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeRunInput builds a RunDelegateInput that re-execs this test binary in the
// requested FLEET_FAKE_MODE (see fleet_dispatch_helper_test.go). It reuses the
// same fake-delegate machinery the dispatch tests use, but exercises the
// policy-free RunDelegate primitive directly with an explicit Cwd.
func fakeRunInput(t *testing.T, mode, cwd string) RunDelegateInput {
	t.Helper()
	t.Setenv("FLEET_FAKE_MODE", mode)
	return RunDelegateInput{Binary: os.Args[0], Cwd: cwd, StdinPrompt: "do the work", TimeoutS: 30, OutputFormat: "text"}
}

// TestRunDelegateHonorsCwdNoGit proves the primitive runs the delegate with its
// working directory set to Cwd and performs NO git/task/trace side effect: the
// cwd is a plain (non-git) temp dir, yet the run succeeds and the delegate's
// file lands inside that cwd.
func TestRunDelegateHonorsCwdNoGit(t *testing.T) {
	cwd := t.TempDir()
	res := RunDelegate(fakeRunInput(t, "success_diff", cwd))
	if !res.HasExit || res.ExitCode != 0 || res.TimedOut || res.Killed {
		t.Fatalf("want clean exit, got %+v", res)
	}
	if _, err := os.Stat(filepath.Join(cwd, "delegate_change.txt")); err != nil {
		t.Fatalf("delegate did not run in Cwd=%s: %v", cwd, err)
	}
	if entries, _ := os.ReadDir(filepath.Join(cwd, ".git")); entries != nil {
		t.Fatal("RunDelegate must not create a git repo in Cwd")
	}
}

// TestRunDelegateTimeoutKills proves the SIGTERM->grace->SIGKILL loop moved into
// the primitive still fires: a delegate that sleeps past the timeout is timed
// out and killed.
func TestRunDelegateTimeoutKills(t *testing.T) {
	in := fakeRunInput(t, "timeout_sleep", t.TempDir())
	in.TimeoutS = 1
	res := RunDelegate(in)
	if !res.TimedOut {
		t.Fatalf("want timed out, got %+v", res)
	}
}

// TestRunDelegateSignalsFromOutput proves QuotaMatched and OutputParseOK are
// computed from the captured output against the DATA passed in.
func TestRunDelegateSignalsFromOutput(t *testing.T) {
	in := fakeRunInput(t, "quota", t.TempDir())
	in.FailureSignatures = []string{"model not supported when using Codex with a ChatGPT account"}
	res := RunDelegate(in)
	if !res.QuotaMatched {
		t.Fatalf("want quota signature matched, got %+v", res)
	}

	bad := fakeRunInput(t, "garbage", t.TempDir())
	bad.OutputFormat = "jsonl"
	if RunDelegate(bad).OutputParseOK {
		t.Fatal("want OutputParseOK=false for unparseable jsonl output")
	}
}
