package core

import (
	"os"
	"path/filepath"
	"testing"
)

// --- ValidateAgyArgv unit tests -------------------------------------------

func TestValidateAgyArgvAcceptsCanonicalTemplate(t *testing.T) {
	// Shape of the real agents.yaml agy argv_template after substitution.
	argv := []string{"--model", "Gemini 3.1 Pro (High)", "--print-timeout", "5m0s", "--print", "do the task"}
	if err := ValidateAgyArgv(argv); err != nil {
		t.Fatalf("expected canonical agy argv to pass, got %v", err)
	}
}

func TestValidateAgyArgvRejectsFlagAfterPrint(t *testing.T) {
	// A mutated template where a flag follows --print: agy's greedy string
	// flag would swallow "--model" itself as the prompt (Fase 0a Sec8.1 trap).
	argv := []string{"--print-timeout", "5m0s", "--print", "--model", "Gemini 3.1 Pro (High)"}
	if err := ValidateAgyArgv(argv); err == nil {
		t.Fatal("expected mutated agy argv (flag following --print) to be rejected")
	}
}

func TestValidateAgyArgvRejectsMissingPrint(t *testing.T) {
	argv := []string{"--model", "Gemini 3.1 Pro (High)", "--print-timeout", "5m0s"}
	if err := ValidateAgyArgv(argv); err == nil {
		t.Fatal("expected argv without --print/-p to be rejected")
	}
}

func TestValidateAgyArgvRejectsTrailingTokenAfterPrompt(t *testing.T) {
	// Prompt is no longer the last token -- an extra token trails it.
	argv := []string{"--print", "do the task", "--model", "Gemini 3.1 Pro (High)"}
	if err := ValidateAgyArgv(argv); err == nil {
		t.Fatal("expected argv with a token trailing the prompt to be rejected")
	}
}

func TestAgyUsageIsAlwaysSourceNone(t *testing.T) {
	if got := AgyUsage(); got.Source != "none" {
		t.Fatalf("agy usage must report Source=none, got %q", got.Source)
	}
}

// --- Fake-binary integration tests (self-contained) -----------------------
//
// These deliberately do NOT reuse fleet_dispatch_helper_test.go's shared
// FLEET_FAKE_MODE re-exec TestMain trampoline (that mechanism is reserved for
// the parallel codex adapter task); instead each case writes its own tiny
// shell-script fixture and runs it directly as spec.Binary, so agy adapter
// coverage can never collide with the codex adapter's fake-mode cases.

func writeAgyFakeScript(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "fake-agy.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func agyShapedSpec(env dispatchEnv, dispatchID, binary string) DispatchSpec {
	spec := env.fakeSpec(dispatchID)
	spec.Agent = "agy"
	spec.Binary = binary
	spec.OutputFormat = "text" // agy's declared output_format in agents.yaml
	spec.Argv = []string{"--model", "Gemini 3.1 Pro (High)", "--print-timeout", "5m0s", "--print", "do the task"}
	return spec
}

func TestFleetAdapterAgySuccessWithDiff(t *testing.T) {
	env := setupDispatchEnv(t)
	scriptDir := t.TempDir()
	script := writeAgyFakeScript(t, scriptDir, "printf 'agy change\\n' > agy_change.txt\necho done\n")
	spec := agyShapedSpec(env, "agy-success", script)
	if err := ValidateAgyArgv(spec.Argv); err != nil {
		t.Fatalf("agy argv guard rejected a well-formed spec: %v", err)
	}
	res, err := Dispatch(spec)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if res.Outcome.Class != "attempt" || !res.Applied {
		t.Fatalf("want applied attempt for success-with-diff, got class=%s applied=%v", res.Outcome.Class, res.Applied)
	}
	if res.Usage.Source != "none" {
		t.Fatalf("agy dispatch must report Usage.Source=none, got %q", res.Usage.Source)
	}
	if _, statErr := os.Stat(filepath.Join(env.worktree, "agy_change.txt")); statErr != nil {
		t.Fatalf("expected delegate diff file in worktree: %v", statErr)
	}
}

func TestFleetAdapterAgyTimeout(t *testing.T) {
	env := setupDispatchEnv(t)
	scriptDir := t.TempDir()
	script := writeAgyFakeScript(t, scriptDir, "sleep 5\necho done\n")
	spec := agyShapedSpec(env, "agy-timeout", script)
	spec.TimeoutS = 1
	res, err := Dispatch(spec)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !res.TimedOut {
		t.Fatalf("want TimedOut=true for a delegate exceeding TimeoutS, got %+v", res.Outcome)
	}
	if res.Outcome.Class != "reroute" || res.Outcome.Cause == nil || *res.Outcome.Cause != "timeout" {
		t.Fatalf("want reroute/timeout outcome, got %+v", res.Outcome)
	}
	if res.Usage.Source != "none" {
		t.Fatalf("agy dispatch must report Usage.Source=none, got %q", res.Usage.Source)
	}
}

func TestFleetAdapterAgyGarbageOutputNoDiff(t *testing.T) {
	env := setupDispatchEnv(t)
	scriptDir := t.TempDir()
	script := writeAgyFakeScript(t, scriptDir, "printf '<<< not structured output at all >>>\\n'\nexit 0\n")
	spec := agyShapedSpec(env, "agy-garbage", script)
	res, err := Dispatch(spec)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !res.Diff.Empty {
		t.Fatalf("garbage-output fixture must not touch the worktree, got diff=%+v", res.Diff)
	}
	if res.Applied {
		t.Fatalf("garbage text output with no diff must never be Applied, got %+v", res)
	}
	if res.Usage.Source != "none" {
		t.Fatalf("agy dispatch must report Usage.Source=none, got %q", res.Usage.Source)
	}
}

func TestFleetAdapterAgyNoop(t *testing.T) {
	env := setupDispatchEnv(t)
	scriptDir := t.TempDir()
	script := writeAgyFakeScript(t, scriptDir, "exit 0\n")
	spec := agyShapedSpec(env, "agy-noop", script)
	res, err := Dispatch(spec)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if res.Outcome.Class != "attempt" || !res.Outcome.Noop {
		t.Fatalf("want noop attempt for empty output + empty diff + exit 0, got %+v", res.Outcome)
	}
	if res.Applied {
		t.Fatal("a noop dispatch must never be Applied")
	}
	if res.Usage.Source != "none" {
		t.Fatalf("agy dispatch must report Usage.Source=none, got %q", res.Usage.Source)
	}
}
