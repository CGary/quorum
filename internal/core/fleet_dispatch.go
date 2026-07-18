package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Generic fleet dispatch engine (FLEET-006-a): runs a delegated CLI in a task
// worktree with process/git invariants, always writes result.json, classifies
// per ADR 0011, and records trace telemetry. Pure-Go syscalls (CGO-free). The
// codex adapter is FLEET-006-b.
const (
	dispatchLockName           = "dispatch.lock"
	dispatchGracefulKillWindow = 2 * time.Second
	dispatchDefaultTimeoutS    = 600
	dispatchLockMarginS        = 60
	dispatchNotesMaxLen        = 2000
)

// DispatchSpec is the Value Object for one dispatch. Delegate command, argv, and
// failure signatures arrive as DATA (no model name hardcoded in Go).
type DispatchSpec struct {
	TaskID            string
	TaskDir           string // .ai/tasks/<loc>/<task>; dispatch/ lives here, OUTSIDE the worktree
	Agent             string
	Model             string // canonical name, data only
	DispatchID        string
	BundleHash        string
	Worktree          string
	Binary            string
	Argv              []string
	StdinPrompt       string
	TimeoutS          int
	FailureSignatures []string
	OutputFormat      string
}

// Dispatch runs one delegated dispatch and returns the normalized result; an
// error means a lock-contention or dirty-precondition abort.
func Dispatch(spec DispatchSpec) (DispatchResult, error) {
	if spec.TaskDir == "" || spec.Worktree == "" || spec.DispatchID == "" {
		return DispatchResult{}, fmt.Errorf("dispatch spec requires TaskDir, Worktree and DispatchID")
	}
	dispatchRoot := filepath.Join(spec.TaskDir, "dispatch")
	dispatchDir := filepath.Join(dispatchRoot, spec.DispatchID)
	resultPath := filepath.Join(dispatchDir, "result.json")
	notesRel := filepath.Join("dispatch", spec.DispatchID, "notes.txt")
	notesPath := filepath.Join(spec.TaskDir, notesRel)
	release, err := acquireTaskLock(dispatchRoot, spec.TimeoutS)
	if err != nil {
		return DispatchResult{}, err
	}
	defer release()
	baselineOut, err := gitOutput(spec.Worktree, "rev-parse", "HEAD")
	if err != nil {
		return DispatchResult{}, fmt.Errorf("cannot resolve worktree baseline: %w", err)
	}
	baseline := strings.TrimSpace(baselineOut)
	startedAt := time.Now().UTC()
	if IsWorktreeDirty(spec.Worktree) {
		res := newBaseResult(spec, startedAt, baseline, notesRel)
		res.EndedAt = time.Now().UTC().Format(time.RFC3339)
		res.Outcome = DispatchOutcome{Class: "reroute"}
		res.Diff = DispatchDiffStat{Empty: true}
		_ = normalizeResult(resultPath, res)
		return res, fmt.Errorf("dispatch aborted: worktree %s is dirty before dispatch", spec.Worktree)
	}
	if err := os.MkdirAll(dispatchDir, 0o755); err != nil {
		return DispatchResult{}, err
	}
	run := runDelegate(spec)
	_ = os.WriteFile(notesPath, []byte(run.output), 0o644)
	endedAt := time.Now().UTC()
	notesTrim := strings.TrimSpace(run.output)
	diffStat, diffErr := stageAndDiffStat(spec.Worktree, baseline)
	var outcome classifiedOutcome
	if diffErr != nil {
		// Staging or diff computation failed (disk full, index.lock, permissions,
		// etc). diffStat.Empty is INDETERMINATE here, not a legitimate empty diff:
		// never treat it as one, and never reset --hard on this path unless a
		// forensic snapshot below actually preserves whatever got staged first.
		msg := diffErr.Error()
		outcome = classifiedOutcome{class: "staging_failed", cause: &msg}
	} else {
		var blockedSig *BlockedSignal
		if bs, perr := ParseBlockedSignal(notesTrim); perr == nil {
			blockedSig = bs
		}
		outcome = classifyOutcome(classifyInput{
			exitCode: run.exitCode, timedOut: run.timedOut, killed: run.killed,
			diffEmpty: diffStat.Empty, notesEmpty: notesTrim == "", blocked: blockedSig,
			quotaMatched:  matchesAnySignature(run.output, spec.FailureSignatures),
			outputParseOK: outputParses(spec.OutputFormat, run.output),
		})
	}
	applied := diffErr == nil && outcome.class == "attempt" && run.exitCode == 0 && !run.timedOut && !run.killed && !diffStat.Empty
	var forensicRef *string
	forensicCaptured := false
	if diffErr != nil || !diffStat.Empty {
		if ref, ferr := captureForensicRef(spec, baseline); ferr == nil {
			forensicRef = &ref
			forensicCaptured = true
		}
	}
	diffConfirmedEmpty := diffErr == nil && diffStat.Empty
	var resetErr error
	switch {
	case applied:
		resetErr = gitRun(spec.Worktree, "reset", "--mixed", baseline)
	case diffConfirmedEmpty || forensicCaptured:
		resetErr = gitRun(spec.Worktree, "reset", "--hard", baseline)
	default:
		// Diff state is unknown and no forensic snapshot could be captured
		// either: leave the worktree untouched rather than risk destroying
		// unrecoverable delegate work with reset --hard.
	}
	var resetErrMsg *string
	if resetErr != nil {
		msg := resetErr.Error()
		resetErrMsg = &msg
	}
	res := newBaseResult(spec, startedAt, baseline, notesRel)
	res.EndedAt = endedAt.Format(time.RFC3339)
	res.DurationS = endedAt.Sub(startedAt).Seconds()
	if run.hasExit {
		ec := run.exitCode
		res.ExitCode = &ec
	}
	res.TimedOut, res.Killed = run.timedOut, run.killed
	res.Outcome = DispatchOutcome{Class: outcome.class, Noop: outcome.noop, Cause: outcome.cause, Blocked: outcome.blocked}
	res.Diff, res.ForensicRef, res.Applied = diffStat, forensicRef, applied
	res.ResetError = resetErrMsg
	res.TraceEvents = traceEventTypes(outcome)
	if err := normalizeResult(resultPath, res); err != nil {
		return res, err
	}
	if terr := appendDispatchTrace(spec, res, outcome, notesTrim); terr != nil {
		return res, terr
	}
	return res, nil
}

type runResult struct {
	output   string
	exitCode int
	hasExit  bool
	timedOut bool
	killed   bool
}

// RunDelegateInput is the policy-free data-only Value Object for one delegated
// process run. It carries no task/worktree/git/forensic concern: the delegate
// binary, argv, working directory, prompt, timeout, and the DATA needed to
// classify the output (failure signatures, declared output format) all arrive
// as plain data so a caller outside the SDC lifecycle can reuse the exact
// process-group timeout machinery without pulling in dispatch policy.
type RunDelegateInput struct {
	Binary            string
	Argv              []string
	Cwd               string
	StdinPrompt       string
	TimeoutS          int
	FailureSignatures []string
	OutputFormat      string
}

// RunDelegateResult holds the observed outcome of one delegated process run.
// QuotaMatched and OutputParseOK are pure signal computations over Output; the
// caller decides what they mean (RunDelegate itself takes no action on them).
type RunDelegateResult struct {
	Output        string
	ExitCode      int
	HasExit       bool
	TimedOut      bool
	Killed        bool
	QuotaMatched  bool
	OutputParseOK bool
}

// RunDelegate runs the delegate in its own process group in in.Cwd; on timeout
// it signals the NEGATIVE pid SIGTERM then SIGKILL so a child ignoring SIGTERM
// still dies. It is policy-free and performs no git/task/trace side effect: it
// only starts the process, enforces the timeout, and returns the captured
// output plus the QuotaMatched/OutputParseOK signals derived from it.
func RunDelegate(in RunDelegateInput) RunDelegateResult {
	timeout := time.Duration(in.TimeoutS) * time.Second
	if timeout <= 0 {
		timeout = time.Duration(dispatchDefaultTimeoutS) * time.Second
	}
	cmd := exec.Command(in.Binary, in.Argv...)
	cmd.Dir = in.Cwd
	cmd.Stdin = strings.NewReader(in.StdinPrompt)
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		out := fmt.Sprintf("dispatch could not start delegate: %v", err)
		return RunDelegateResult{
			Output: out, ExitCode: -1,
			QuotaMatched:  matchesAnySignature(out, in.FailureSignatures),
			OutputParseOK: outputParses(in.OutputFormat, out),
		}
	}
	pgid := cmd.Process.Pid // Setpgid makes the child its own group leader
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	var (
		werr             error
		timedOut, killed bool
	)
	select {
	case werr = <-done:
	case <-timer.C:
		timedOut = true
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
		graceExpired := false
		select {
		case werr = <-done:
		case <-time.After(dispatchGracefulKillWindow):
			graceExpired = true
		}
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		if graceExpired {
			killed = true
			werr = <-done
		}
	}
	output := buf.String()
	return RunDelegateResult{
		Output: output, ExitCode: exitCodeFrom(werr), HasExit: true,
		TimedOut: timedOut, Killed: killed,
		QuotaMatched:  matchesAnySignature(output, in.FailureSignatures),
		OutputParseOK: outputParses(in.OutputFormat, output),
	}
}

// runDelegate is a thin behavior-preserving adapter: it builds a RunDelegateInput
// from the dispatch spec (Cwd=spec.Worktree) and maps the result back to the
// internal runResult so core.Dispatch stays byte-for-byte unchanged. Dispatch
// recomputes QuotaMatched/OutputParseOK from run.output, so those fields are
// intentionally dropped here.
func runDelegate(spec DispatchSpec) runResult {
	r := RunDelegate(RunDelegateInput{
		Binary: spec.Binary, Argv: spec.Argv, Cwd: spec.Worktree,
		StdinPrompt: spec.StdinPrompt, TimeoutS: spec.TimeoutS,
		FailureSignatures: spec.FailureSignatures, OutputFormat: spec.OutputFormat,
	})
	return runResult{
		output: r.Output, exitCode: r.ExitCode, hasExit: r.HasExit,
		timedOut: r.TimedOut, killed: r.Killed,
	}
}
func exitCodeFrom(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

type classifyInput struct {
	exitCode      int
	timedOut      bool
	killed        bool
	diffEmpty     bool
	notesEmpty    bool
	blocked       *BlockedSignal
	quotaMatched  bool
	outputParseOK bool
}
type classifiedOutcome struct {
	class   string
	noop    bool
	cause   *string
	blocked *BlockedSignal
}

// classifyOutcome is a pure signal-based classifier with deterministic ADR 0011
// precedence. cause and noop are DATA, never class values.
func classifyOutcome(in classifyInput) classifiedOutcome {
	cause := func(c string) classifiedOutcome { return classifiedOutcome{class: "reroute", cause: &c} }
	switch {
	case in.blocked != nil && in.diffEmpty: // parks the task; consumes nothing
		return classifiedOutcome{class: "blocked", blocked: in.blocked}
	case !in.diffEmpty: // a diff is always an attempt, regardless of exit code
		return classifiedOutcome{class: "attempt"}
	case in.exitCode == 0 && in.notesEmpty: // exit 0 + empty diff + empty notes
		return classifiedOutcome{class: "attempt", noop: true}
	case in.quotaMatched:
		return cause("quota")
	case in.timedOut || in.killed:
		return cause("timeout")
	case !in.outputParseOK:
		return cause("wrapper_broken")
	default: // other empty-diff exit: a genuine failed attempt (model incapacity)
		return classifiedOutcome{class: "attempt"}
	}
}

// traceEventTypes lists the closed-vocabulary event types emitted, in append order.
func traceEventTypes(o classifiedOutcome) []string {
	events := []string{"dispatch_started"}
	switch o.class {
	case "reroute":
		events = append(events, "reroute")
		if o.cause != nil && *o.cause == "quota" {
			events = append(events, "quota_red")
		}
		if o.cause != nil && *o.cause == "wrapper_broken" {
			events = append(events, "wrapper_broken")
		}
	case "blocked":
		events = append(events, "blocked_question")
	}
	return append(events, "dispatch_finished")
}
func matchesAnySignature(output string, signatures []string) bool {
	for _, sig := range signatures {
		if sig = strings.TrimSpace(sig); sig != "" && strings.Contains(output, sig) {
			return true
		}
	}
	return false
}

// outputParses reports whether the output is parseable for its declared format;
// text/unknown is never "unparseable", jsonl/json that fails to parse is broken.
func outputParses(format, output string) bool {
	trimmed := strings.TrimSpace(output)
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "jsonl":
		for _, line := range strings.Split(trimmed, "\n") {
			if line = strings.TrimSpace(line); line != "" && !json.Valid([]byte(line)) {
				return false
			}
		}
		return true
	case "json":
		return trimmed == "" || json.Valid([]byte(trimmed))
	default:
		return true
	}
}

// stageAndDiffStat stages every change (add -A) and computes the numstat of the
// staged tree against the baseline commit.
func stageAndDiffStat(worktree, baseline string) (DispatchDiffStat, error) {
	if err := gitRun(worktree, "add", "-A"); err != nil {
		return DispatchDiffStat{Empty: true}, err
	}
	out, err := gitOutput(worktree, "diff", "--cached", "--numstat", baseline)
	if err != nil {
		return DispatchDiffStat{Empty: true}, err
	}
	stat := DispatchDiffStat{Empty: true}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		stat.Empty = false
		stat.FilesChanged++
		if fields := strings.Fields(line); len(fields) >= 2 {
			if n, aerr := strconv.Atoi(fields[0]); aerr == nil {
				stat.Insertions += n
			}
			if n, aerr := strconv.Atoi(fields[1]); aerr == nil {
				stat.Deletions += n
			}
		}
	}
	return stat, nil
}

// captureForensicRef snapshots the staged tree as a commit parented on baseline
// and points refs/fleet/<task>/attempt-N-<model> at it WITHOUT moving the branch.
func captureForensicRef(spec DispatchSpec, baseline string) (string, error) {
	tree, err := gitOutput(spec.Worktree, "write-tree")
	if err != nil {
		return "", err
	}
	msg := fmt.Sprintf("fleet forensic snapshot %s %s", spec.TaskID, spec.DispatchID)
	snap, err := gitOutput(spec.Worktree, "commit-tree", strings.TrimSpace(tree), "-p", baseline, "-m", msg)
	if err != nil {
		return "", err
	}
	n := countForensicAttempts(spec.Worktree, spec.TaskID) + 1
	ref := fmt.Sprintf("refs/fleet/%s/attempt-%d-%s", spec.TaskID, n, sanitizeModelForRef(spec.Model))
	if err := gitRun(spec.Worktree, "update-ref", ref, strings.TrimSpace(snap)); err != nil {
		return "", err
	}
	return ref, nil
}
func countForensicAttempts(worktree, taskID string) int {
	out, err := gitOutput(worktree, "for-each-ref", "--format=%(refname)", "refs/fleet/"+taskID+"/")
	if err != nil {
		return 0
	}
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}
func sanitizeModelForRef(model string) string {
	if r := strings.ReplaceAll(model, "/", "-"); strings.TrimSpace(r) != "" {
		return r
	}
	return "model"
}
func newBaseResult(spec DispatchSpec, startedAt time.Time, baseline, notesRel string) DispatchResult {
	return DispatchResult{
		SchemaVersion: FleetDispatchResultSchemaVersion, DispatchID: spec.DispatchID,
		TaskID: spec.TaskID, Agent: spec.Agent, Model: spec.Model, Phase: "execute",
		StartedAt: startedAt.Format(time.RFC3339), BaselineCommit: baseline,
		Usage: DispatchUsage{Source: "none"}, NotesPath: notesRel, TraceEvents: []string{},
	}
}

type dispatchLockBody struct {
	PID       int    `json:"pid"`
	TTLS      int    `json:"ttl_s"`
	CreatedAt string `json:"created_at"`
}

// acquireTaskLock creates a per-task lock via O_CREATE|O_EXCL. An existing lock
// is reclaimed only when orphaned (recorded pid dead or ttl expired); otherwise
// it returns an explicit contention error.
func acquireTaskLock(dispatchRoot string, timeoutS int) (func(), error) {
	if err := os.MkdirAll(dispatchRoot, 0o755); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(dispatchRoot, dispatchLockName)
	ttl := timeoutS + dispatchLockMarginS
	if timeoutS <= 0 {
		ttl = dispatchDefaultTimeoutS + dispatchLockMarginS
	}
	tryCreate := func() (bool, error) {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			if os.IsExist(err) {
				return false, nil
			}
			return false, err
		}
		body, _ := json.Marshal(dispatchLockBody{PID: os.Getpid(), TTLS: ttl, CreatedAt: time.Now().UTC().Format(time.RFC3339)})
		_, werr := f.Write(body)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		return werr == nil, werr
	}
	created, err := tryCreate()
	if err != nil {
		return nil, err
	}
	if !created && dispatchLockOrphaned(lockPath) {
		_ = os.Remove(lockPath)
		if created, err = tryCreate(); err != nil {
			return nil, err
		}
	}
	if !created {
		return nil, fmt.Errorf("dispatch lock contention: %s is held by a live dispatch", lockPath)
	}
	return func() { _ = os.Remove(lockPath) }, nil
}
func dispatchLockOrphaned(lockPath string) bool {
	raw, err := os.ReadFile(lockPath)
	if err != nil {
		return true
	}
	var lk dispatchLockBody
	if err := json.Unmarshal(raw, &lk); err != nil {
		return true
	}
	if !dispatchProcessAlive(lk.PID) {
		return true
	}
	if lk.TTLS > 0 && lk.CreatedAt != "" {
		if created, perr := time.Parse(time.RFC3339, lk.CreatedAt); perr == nil && time.Since(created) > time.Duration(lk.TTLS)*time.Second {
			return true
		}
	}
	return false
}

// dispatchProcessAlive probes liveness with signal 0, the serve.go precedent.
func dispatchProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

// appendDispatchTrace appends telemetry to 07-trace.json via the validated
// append-only SaveArtifact path (ADR 0011 closed vocabulary); an attempt appends
// one phase=execute entry, reroute and blocked append none.
func appendDispatchTrace(spec DispatchSpec, res DispatchResult, outcome classifiedOutcome, notes string) error {
	tracePath := filepath.Join(spec.TaskDir, "07-trace.json")
	payload, err := LoadArtifactPayload(tracePath)
	if err != nil {
		return fmt.Errorf("cannot load trace for dispatch telemetry: %w", err)
	}
	trace, ok := payload.(map[string]any)
	if !ok {
		return fmt.Errorf("trace payload is not an object")
	}
	ts := res.EndedAt
	var causeVal any
	if outcome.cause != nil {
		causeVal = *outcome.cause
	}
	events, _ := asSlice(trace["events"])
	events = append(events, map[string]any{"type": "dispatch_started", "ts": res.StartedAt, "dispatch_id": spec.DispatchID, "bundle_hash": spec.BundleHash})
	switch outcome.class {
	case "reroute":
		events = append(events, map[string]any{"type": "reroute", "ts": ts, "dispatch_id": spec.DispatchID, "cause": causeVal})
		if outcome.cause != nil && *outcome.cause == "quota" {
			events = append(events, map[string]any{"type": "quota_red", "ts": ts, "dispatch_id": spec.DispatchID})
		}
		if outcome.cause != nil && *outcome.cause == "wrapper_broken" {
			events = append(events, map[string]any{"type": "wrapper_broken", "ts": ts, "dispatch_id": spec.DispatchID})
		}
	case "blocked":
		ev := map[string]any{"type": "blocked_question", "ts": ts, "dispatch_id": spec.DispatchID}
		if outcome.blocked != nil {
			ev["path"], ev["reason"], ev["severity"] = outcome.blocked.Path, outcome.blocked.Reason, outcome.blocked.Severity
		}
		events = append(events, ev)
	}
	var forensic any
	if res.ForensicRef != nil {
		forensic = *res.ForensicRef
	}
	events = append(events, map[string]any{
		"type": "dispatch_finished", "ts": ts, "dispatch_id": spec.DispatchID,
		"outcome_class": outcome.class, "cause": causeVal, "noop": outcome.noop,
		"diff_stat":    map[string]any{"empty": res.Diff.Empty, "files_changed": res.Diff.FilesChanged, "insertions": res.Diff.Insertions, "deletions": res.Diff.Deletions},
		"forensic_ref": forensic, "usage_source": res.Usage.Source, "applied": res.Applied,
	})
	trace["events"] = events
	if outcome.class == "attempt" {
		result := "failed"
		if res.Applied {
			result = "passed"
		}
		entry := map[string]any{"phase": "execute", "result": result, "model": spec.Model, "duration_s": res.DurationS}
		if notes != "" {
			entry["notes"] = truncate(notes, dispatchNotesMaxLen)
		}
		attempts, _ := asSlice(trace["attempts"])
		trace["attempts"] = append(attempts, entry)
	}
	_, err = TaskStore{}.SaveArtifact(&TaskDirMatch{Path: spec.TaskDir, Location: "active"}, "07-trace.json", trace)
	return err
}
func gitRun(dir string, args ...string) error {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return nil
}
func gitOutput(dir string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
