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
