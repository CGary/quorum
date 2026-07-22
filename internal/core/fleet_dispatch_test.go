package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func loadResult(t *testing.T, path string) DispatchResult {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read result.json: %v", err)
	}
	var res DispatchResult
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("unmarshal result.json: %v", err)
	}
	return res
}
func causeOf(o classifiedOutcome) string {
	if o.cause == nil {
		return ""
	}
	return *o.cause
}
func resCause(res DispatchResult) string {
	if res.Outcome.Cause == nil {
		return ""
	}
	return *res.Outcome.Cause
}

func TestFleetDispatchOutcomeClassification(t *testing.T) {
	sig := &BlockedSignal{Path: "cmd/x.go", Reason: "needs helper", Severity: "critical"}
	cases := []struct {
		name, class, cause string
		noop               bool
		in                 classifyInput
	}{
		{"blocked", "blocked", "", false, classifyInput{diffEmpty: true, blocked: sig, outputParseOK: true}},
		{"diff-attempt", "attempt", "", false, classifyInput{diffEmpty: false, outputParseOK: true}},
		{"noop", "attempt", "", true, classifyInput{diffEmpty: true, notesEmpty: true, outputParseOK: true}},
		{"quota", "reroute", "quota", false, classifyInput{diffEmpty: true, exitCode: 1, quotaMatched: true, outputParseOK: true}},
		{"timeout", "reroute", "timeout", false, classifyInput{diffEmpty: true, timedOut: true, outputParseOK: true}},
		{"wrapper-broken", "reroute", "wrapper_broken", false, classifyInput{diffEmpty: true, outputParseOK: false}},
		{"failed-attempt", "attempt", "", false, classifyInput{diffEmpty: true, exitCode: 1, outputParseOK: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyOutcome(tc.in)
			if got.class != tc.class || got.noop != tc.noop || causeOf(got) != tc.cause {
				t.Fatalf("= {%s noop:%v %q}, want {%s noop:%v %q}", got.class, got.noop, causeOf(got), tc.class, tc.noop, tc.cause)
			}
		})
	}
}

func TestFleetDispatchIntegrationOutcomes(t *testing.T) {
	cases := []struct {
		name, mode, format, class, cause                   string
		timeoutS                                           int
		applied, noop, timedOut, wantForensic, wantCleanWT bool
	}{
		{"failed-empty", "fail_empty", "text", "attempt", "", 30, false, false, false, false, true},
		{"timeout", "timeout_sleep", "text", "reroute", "timeout", 1, false, false, true, false, true},
		{"unparseable", "garbage", "jsonl", "reroute", "wrapper_broken", 30, false, false, false, false, true},
		{"noop", "noop", "text", "attempt", "", 30, false, true, false, false, true},
		{"failed-reset-with-diff", "diff_then_fail", "text", "attempt", "", 30, false, false, false, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := setupDispatchEnv(t)
			t.Setenv("FLEET_FAKE_MODE", tc.mode)
			spec := env.fakeSpec("d1")
			spec.OutputFormat, spec.TimeoutS = tc.format, tc.timeoutS
			res, err := Dispatch(spec)
			if err != nil {
				t.Fatalf("Dispatch: %v", err)
			}
			if res.Outcome.Class != tc.class || resCause(res) != tc.cause || res.Applied != tc.applied || res.Outcome.Noop != tc.noop || res.TimedOut != tc.timedOut {
				t.Fatalf("got {%s %q applied:%v noop:%v timed:%v}, want {%s %q %v %v %v}", res.Outcome.Class, resCause(res), res.Applied, res.Outcome.Noop, res.TimedOut, tc.class, tc.cause, tc.applied, tc.noop, tc.timedOut)
			}
			if (res.ForensicRef != nil) != tc.wantForensic {
				t.Fatalf("forensic ref presence = %v, want %v", res.ForensicRef != nil, tc.wantForensic)
			}
			if _, e := os.Stat(filepath.Join(env.dispatchDir("d1"), "result.json")); e != nil {
				t.Fatalf("result.json must always be written: %v", e)
			}
			if clean := strings.TrimSpace(run(t, env.worktree, "git", "status", "--porcelain")) == ""; clean != tc.wantCleanWT {
				t.Fatalf("worktree clean = %v, want %v", clean, tc.wantCleanWT)
			}
		})
	}
}

func TestFleetDispatchSuccessAppliesDiffAndForensicRef(t *testing.T) {
	env := setupDispatchEnv(t)
	t.Setenv("FLEET_FAKE_MODE", "success_diff")
	res, err := Dispatch(env.fakeSpec("d1"))
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if res.Outcome.Class != "attempt" || !res.Applied || res.Diff.Empty {
		t.Fatalf("want applied attempt with diff, got %+v applied=%v", res.Outcome, res.Applied)
	}
	if _, err := os.Stat(filepath.Join(env.worktree, "delegate_change.txt")); err != nil {
		t.Fatalf("worktree diff not preserved: %v", err)
	}
	wantRef := "refs/fleet/FLEET-700/attempt-1-openai-gpt-5.5-medium"
	if res.ForensicRef == nil || *res.ForensicRef != wantRef {
		t.Fatalf("forensic ref = %v, want %s", res.ForensicRef, wantRef)
	}
	run(t, env.worktree, "git", "rev-parse", "--verify", wantRef)
	if names := run(t, env.worktree, "git", "diff", "--name-only", "main", wantRef); !strings.Contains(names, "delegate_change.txt") {
		t.Fatalf("forensic ref does not hold the diff: %q", names)
	}
	if loadResult(t, filepath.Join(env.dispatchDir("d1"), "result.json")).Outcome.Class != "attempt" {
		t.Fatal("result.json class should be attempt")
	}
}

// TestFleetDispatchStagingFailurePreservesWorktree covers the data-loss bug:
// when stageAndDiffStat cannot even complete (here, a live index.lock
// simulating disk-full/permission-denied style infra failures), Dispatch must
// NOT treat the resulting Empty:true diff stat as a legitimate empty diff, must
// NOT run `git reset --hard`, and must classify the outcome distinctly so the
// delegate's on-disk work survives for forensics.
func TestFleetDispatchStagingFailurePreservesWorktree(t *testing.T) {
	env := setupDispatchEnv(t)
	t.Setenv("FLEET_FAKE_MODE", "success_diff")
	spec := env.fakeSpec("d1")

	gitDir := strings.TrimSpace(run(t, env.worktree, "git", "rev-parse", "--git-dir"))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(env.worktree, gitDir)
	}
	lockPath := filepath.Join(gitDir, "index.lock")
	if err := os.WriteFile(lockPath, []byte("held by another process\n"), 0o644); err != nil {
		t.Fatalf("seed index.lock: %v", err)
	}

	res, err := Dispatch(spec)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if err := os.Remove(lockPath); err != nil {
		t.Fatalf("cleanup index.lock: %v", err)
	}

	if res.Outcome.Class != "staging_failed" {
		t.Fatalf("outcome class = %q, want staging_failed", res.Outcome.Class)
	}
	if res.Outcome.Cause == nil || *res.Outcome.Cause == "" {
		t.Fatalf("expected outcome.cause to carry the staging error, got %v", res.Outcome.Cause)
	}
	if res.Applied {
		t.Fatal("staging failure must never be marked applied")
	}
	if !res.Diff.Empty {
		t.Fatal("diff stat is expected Empty:true on staging failure (indeterminate, not authoritative)")
	}
	if _, e := os.Stat(filepath.Join(env.worktree, "delegate_change.txt")); e != nil {
		t.Fatalf("delegate work must survive an aborted staging step: %v", e)
	}
	if strings.TrimSpace(run(t, env.worktree, "git", "status", "--porcelain")) == "" {
		t.Fatal("worktree must NOT be reset --hard when staging itself failed")
	}
	if loadResult(t, filepath.Join(env.dispatchDir("d1"), "result.json")).Outcome.Class != "staging_failed" {
		t.Fatal("result.json class should be staging_failed")
	}
}

// TestFleetDispatchStagingFailureWithForensicCaptureStillSkipsReset covers the
// residual false-safety edge: even when a staging error (diffErr != nil) is
// paired with a forensic ref that DID get captured, reset --hard must still
// never run. write-tree only snapshots whatever made it into the index before
// `git add -A` failed, so a successful capture on this path does not prove the
// working tree is safe to discard -- it may be missing the very files that
// tripped the staging error.
func TestFleetDispatchStagingFailureWithForensicCaptureStillSkipsReset(t *testing.T) {
	env := setupDispatchEnv(t)
	t.Setenv("FLEET_FAKE_MODE", "success_diff_unreadable")
	spec := env.fakeSpec("d1")

	res, err := Dispatch(spec)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	// Cleanup: the unreadable file would otherwise block worktree teardown.
	_ = os.Chmod(filepath.Join(env.worktree, "delegate_unreadable.txt"), 0o644)

	if res.Outcome.Class != "staging_failed" {
		t.Fatalf("outcome class = %q, want staging_failed", res.Outcome.Class)
	}
	if res.ForensicRef == nil {
		t.Fatal("expected forensic capture to succeed on this path (write-tree only reads the index)")
	}
	if res.ResetError != nil {
		t.Fatalf("resetErr = %v, want nil (reset must never even be attempted)", *res.ResetError)
	}
	if _, e := os.Stat(filepath.Join(env.worktree, "delegate_change.txt")); e != nil {
		t.Fatalf("delegate work must survive a staging failure even with a captured forensic ref: %v", e)
	}
	if strings.TrimSpace(run(t, env.worktree, "git", "status", "--porcelain")) == "" {
		t.Fatal("worktree must NOT be reset --hard just because a forensic ref was captured on a staging_failed path")
	}
}

// TestFleetDispatchNonEmptyDiffForensicCaptureFailsPreservesWorktree covers the
// diffErr == nil branch: staging and diffing succeed, the diff is genuinely
// non-empty, but the forensic snapshot itself fails to write (e.g. refs
// directory unwritable). Dispatch must not reset --hard without a forensic
// safety net, leaving the staged, uncommitted delegate work in place.
func TestFleetDispatchNonEmptyDiffForensicCaptureFailsPreservesWorktree(t *testing.T) {
	env := setupDispatchEnv(t)
	commonDir := strings.TrimSpace(run(t, env.worktree, "git", "rev-parse", "--git-common-dir"))
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(env.worktree, commonDir)
	}
	refsDir := filepath.Join(commonDir, "refs", "fleet", env.taskID)
	if err := os.MkdirAll(refsDir, 0o755); err != nil {
		t.Fatalf("seed refs/fleet dir: %v", err)
	}
	if err := os.Chmod(refsDir, 0o555); err != nil {
		t.Fatalf("lock refs/fleet dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(refsDir, 0o755) })

	// diff_then_fail: exit 1 with a real (non-empty) diff, so `applied` is
	// false and the outcome must fall through to the forensicCaptured branch
	// of the reset switch -- which must now fail to capture.
	t.Setenv("FLEET_FAKE_MODE", "diff_then_fail")
	res, err := Dispatch(env.fakeSpec("d1"))
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	if res.Diff.Empty {
		t.Fatal("expected a genuinely non-empty diff for this scenario")
	}
	if res.ForensicRef != nil {
		t.Fatalf("expected forensic capture to fail (refs/fleet dir is read-only), got ref = %v", res.ForensicRef)
	}
	if res.ResetError != nil {
		t.Fatalf("resetErr = %v, want nil (reset must never even be attempted)", *res.ResetError)
	}
	if _, e := os.Stat(filepath.Join(env.worktree, "delegate_partial.txt")); e != nil {
		t.Fatalf("delegate work must survive a failed forensic capture: %v", e)
	}
	if strings.TrimSpace(run(t, env.worktree, "git", "status", "--porcelain")) == "" {
		t.Fatal("worktree must NOT be reset --hard when the forensic capture itself failed")
	}
}

// TestFleetDispatchResetHardFailurePopulatesResetError forces `git reset
// --hard` itself to fail on the one path that is allowed to run it
// (diffErr == nil, forensic ref captured, not applied): the delegate modifies
// an existing tracked file and then makes the worktree directory read-only,
// so staging/diff/forensic-capture (which never write into the working tree)
// still succeed, but the reset's unlink+recreate of that tracked file cannot.
// Dispatch must surface this as res.ResetError rather than silently losing it.
func TestFleetDispatchResetHardFailurePopulatesResetError(t *testing.T) {
	env := setupDispatchEnv(t)
	t.Setenv("FLEET_FAKE_MODE", "diff_then_fail_readonly_wt")
	spec := env.fakeSpec("d1")

	res, err := Dispatch(spec)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	// Restore write access so t.TempDir() cleanup can remove the worktree.
	t.Cleanup(func() { _ = os.Chmod(env.worktree, 0o755) })

	if res.ForensicRef == nil {
		t.Fatal("expected forensic capture to succeed (only the working tree, not the git dir, is read-only)")
	}
	if res.ResetError == nil {
		t.Fatal("expected reset --hard to fail and populate res.ResetError, got nil")
	}
	if !strings.Contains(*res.ResetError, "seed.txt") && !strings.Contains(*res.ResetError, "Permission denied") {
		t.Fatalf("resetErr = %q, want it to reference the permission failure on seed.txt", *res.ResetError)
	}
	if loadResult(t, filepath.Join(env.dispatchDir("d1"), "result.json")).ResetError == nil {
		t.Fatal("result.json must also carry the reset error")
	}
}

func TestFleetDispatchKilledProcessGroup(t *testing.T) {
	env := setupDispatchEnv(t)
	pidFile := filepath.Join(env.taskDir, "grandchild.pid")
	t.Setenv("FLEET_FAKE_MODE", "group_child")
	t.Setenv("FLEET_FAKE_PIDFILE", pidFile)
	spec := env.fakeSpec("d1")
	spec.TimeoutS = 1
	res, err := Dispatch(spec)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !res.Killed || !res.TimedOut || res.Outcome.Class != "reroute" || resCause(res) != "timeout" {
		t.Fatalf("want killed timed_out reroute/timeout, got killed=%v timed=%v %+v", res.Killed, res.TimedOut, res.Outcome)
	}
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("grandchild pid file missing: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("bad grandchild pid: %v", err)
	}
	for i := 0; i < 40; i++ {
		if !dispatchProcessAlive(pid) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("grandchild pid %d survived the process-group kill", pid)
}

func TestFleetDispatchDirtyWorktreeAborts(t *testing.T) {
	env := setupDispatchEnv(t)
	if err := os.WriteFile(filepath.Join(env.worktree, "unrelated.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLEET_FAKE_MODE", "success_diff")
	res, err := Dispatch(env.fakeSpec("d1"))
	if err == nil {
		t.Fatal("dispatch over a dirty worktree must return an error")
	}
	if _, e := os.Stat(filepath.Join(env.worktree, "delegate_change.txt")); e == nil {
		t.Fatal("delegate was spawned despite the dirty precondition")
	}
	if res.ForensicRef != nil || countForensicAttempts(env.worktree, env.taskID) != 0 {
		t.Fatal("dirty abort must create no forensic ref")
	}
	if _, e := os.Stat(filepath.Join(env.dispatchDir("d1"), "result.json")); e != nil {
		t.Fatalf("result.json must be written on dirty abort: %v", e)
	}
}

func TestFleetDispatchLock(t *testing.T) {
	writeLock := func(t *testing.T, env dispatchEnv, body dispatchLockBody) {
		t.Helper()
		dir := filepath.Join(env.taskDir, "dispatch")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(body)
		if err := os.WriteFile(filepath.Join(dir, dispatchLockName), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	now := func() string { return time.Now().UTC().Format(time.RFC3339) }
	t.Run("contention", func(t *testing.T) {
		env := setupDispatchEnv(t)
		writeLock(t, env, dispatchLockBody{PID: os.Getpid(), TTLS: 600, CreatedAt: now()})
		t.Setenv("FLEET_FAKE_MODE", "success_diff")
		if _, err := Dispatch(env.fakeSpec("d1")); err == nil {
			t.Fatal("dispatch must fail under a live lock")
		}
		if _, e := os.Stat(filepath.Join(env.worktree, "delegate_change.txt")); e == nil {
			t.Fatal("delegate spawned despite lock contention")
		}
		if _, e := os.Stat(filepath.Join(env.dispatchDir("d1"), "result.json")); e == nil {
			t.Fatal("no result.json should be written on lock contention")
		}
	})
	reclaim := func(t *testing.T, body dispatchLockBody) {
		env := setupDispatchEnv(t)
		writeLock(t, env, body)
		t.Setenv("FLEET_FAKE_MODE", "success_diff")
		res, err := Dispatch(env.fakeSpec("d1"))
		if err != nil || res.Outcome.Class != "attempt" {
			t.Fatalf("orphaned lock should be reclaimed, got err=%v outcome=%+v", err, res.Outcome)
		}
	}
	t.Run("dead-pid", func(t *testing.T) {
		reclaim(t, dispatchLockBody{PID: deadPID(t), TTLS: 600, CreatedAt: now()})
	})
	t.Run("expired-ttl", func(t *testing.T) {
		reclaim(t, dispatchLockBody{PID: os.Getpid(), TTLS: 1, CreatedAt: time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)})
	})
}

func TestFleetDispatchTraceTelemetry(t *testing.T) {
	closed := map[string]bool{"routing_decision": true, "dispatch_started": true, "dispatch_finished": true,
		"reroute": true, "wrapper_broken": true, "quota_red": true, "blocked_question": true, "blocked_answer": true, "review_family_degraded": true}
	check := func(t *testing.T, taskDir string, want []string, wantAttempts int) {
		events := loadTraceEvents(t, taskDir)
		for _, e := range events {
			if typ, ok := e["type"].(string); ok {
				if !closed[typ] {
					t.Fatalf("event type %q outside the closed vocabulary", typ)
				}
				if e["dispatch_id"] == nil {
					t.Fatalf("dispatch event missing dispatch_id: %v", e)
				}
			}
		}
		types := eventTypesInTrace(events)
		for _, w := range want {
			if !containsStr(types, w) {
				t.Fatalf("missing event %q, got %v", w, types)
			}
		}
		if n := countExecuteAttempts(t, taskDir); n != wantAttempts {
			t.Fatalf("execute attempts = %d, want %d", n, wantAttempts)
		}
	}
	t.Run("attempt", func(t *testing.T) {
		env := setupDispatchEnv(t)
		t.Setenv("FLEET_FAKE_MODE", "success_diff")
		if _, err := Dispatch(env.fakeSpec("d1")); err != nil {
			t.Fatalf("Dispatch: %v", err)
		}
		check(t, env.taskDir, []string{"dispatch_started", "dispatch_finished"}, 1)
	})
	t.Run("reroute-quota", func(t *testing.T) {
		env := setupDispatchEnv(t)
		t.Setenv("FLEET_FAKE_MODE", "quota")
		spec := env.fakeSpec("d1")
		spec.FailureSignatures = []string{"model not supported when using Codex with a ChatGPT account"}
		if _, err := Dispatch(spec); err != nil {
			t.Fatalf("Dispatch: %v", err)
		}
		check(t, env.taskDir, []string{"reroute", "quota_red"}, 0)
	})
	t.Run("blocked", func(t *testing.T) {
		env := setupDispatchEnv(t)
		t.Setenv("FLEET_FAKE_MODE", "blocked")
		res, err := Dispatch(env.fakeSpec("d1"))
		if err != nil {
			t.Fatalf("Dispatch: %v", err)
		}
		if res.Outcome.Class != "blocked" || res.Outcome.Blocked == nil || res.Outcome.Blocked.Severity != "critical" {
			t.Fatalf("want blocked outcome with parsed signal, got %+v", res.Outcome)
		}
		check(t, env.taskDir, []string{"blocked_question"}, 0)
	})
}
func countExecuteAttempts(t *testing.T, taskDir string) int {
	t.Helper()
	payload, err := LoadArtifactPayload(filepath.Join(taskDir, "07-trace.json"))
	if err != nil {
		t.Fatalf("load trace: %v", err)
	}
	attempts, _ := asSlice(payload.(map[string]any)["attempts"])
	n := 0
	for _, a := range attempts {
		if m, ok := a.(map[string]any); ok && m["phase"] == "execute" {
			n++
		}
	}
	return n
}

func deadPID(t *testing.T) int {
	t.Helper()
	c := execCommandFake("noop")
	if err := c.Start(); err != nil {
		t.Fatalf("start throwaway process: %v", err)
	}
	pid := c.Process.Pid
	_ = c.Wait()
	return pid
}

func TestFleetDispatchSpawnFailure(t *testing.T) {
	env := setupDispatchEnv(t)
	spec := env.fakeSpec("d1")
	spec.Binary = "/path/to/nonexistent/binary/to/force/spawn/failure"

	res, err := Dispatch(spec)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	if res.Outcome.Class != "spawn_failed" {
		t.Fatalf("outcome class = %q, want spawn_failed", res.Outcome.Class)
	}
	if res.ExitCode != nil {
		t.Fatalf("exit_code = %v, want nil", res.ExitCode)
	}
	if res.DurationS > 1.0 {
		t.Fatalf("DurationS = %v, want near zero", res.DurationS)
	}

	if n := countExecuteAttempts(t, env.taskDir); n != 0 {
		t.Fatalf("execute attempts = %d, want 0", n)
	}

	loaded := loadResult(t, filepath.Join(env.dispatchDir("d1"), "result.json"))
	if loaded.Outcome.Class != "spawn_failed" {
		t.Fatalf("loaded result class = %q", loaded.Outcome.Class)
	}
	if loaded.Applied {
		t.Fatal("loaded result applied = true")
	}
	if loaded.ForensicRef != nil {
		t.Fatalf("loaded result forensic_ref = %v", *loaded.ForensicRef)
	}
	if loaded.ResetError != nil {
		t.Fatalf("loaded result reset_error = %v", *loaded.ResetError)
	}

	events := loadTraceEvents(t, env.taskDir)
	types := eventTypesInTrace(events)
	if len(types) != 2 || types[0] != "dispatch_started" || types[1] != "dispatch_finished" {
		t.Fatalf("expected exactly [dispatch_started dispatch_finished], got %v", types)
	}
}
