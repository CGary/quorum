package cmd

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"quorum/internal/core"
)

// captureStderr redirects os.Stderr for the duration of fn and returns
// everything written to it. checkAiderCostGuard (cmd/fleet_dispatch.go) logs
// its cost_exceeded alert via fmt.Fprintf(os.Stderr, ...), so this is the
// only way to assert the alert actually fired rather than merely that the
// pure ParseAiderReportedCost/CostExceedsCeiling functions compute the right
// boolean in isolation.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w
	fn()
	os.Stderr = orig
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read captured stderr: %v", err)
	}
	return string(out)
}

// writeFakeAiderCostScript overwrites setupAiderFakeProject's fake-aider.sh
// so it reports the given session cost, in USD, on its final stdout line
// (aider's real free-text shape: "Cost: $X message, $Y session.").
func writeFakeAiderCostScript(t *testing.T, root, sessionCost string) {
	t.Helper()
	script := filepath.Join(root, "fake-aider.sh")
	body := "#!/bin/sh\n" +
		"set -e\n" +
		"args=\"$*\"\n" +
		"case \"$args\" in\n" +
		"  *--message-file*--no-auto-commits*--no-attribute-co-authored-by*--yes-always*--model*) ;;\n" +
		"  *) echo \"missing required flags: $args\" 1>&2; exit 9 ;;\n" +
		"esac\n" +
		"msgfile=\"\"\n" +
		"prev=\"\"\n" +
		"for a in \"$@\"; do\n" +
		"  if [ \"$prev\" = \"--message-file\" ]; then msgfile=\"$a\"; fi\n" +
		"  prev=\"$a\"\n" +
		"done\n" +
		"if [ -z \"$msgfile\" ] || [ ! -s \"$msgfile\" ]; then\n" +
		"  echo \"message file missing or empty: $msgfile\" 1>&2\n" +
		"  exit 8\n" +
		"fi\n" +
		"printf 'aider changed\\n' >> seed.txt\n" +
		"echo 'Cost: $0.01 message, $" + sessionCost + " session.'\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func gitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func setupFleetDispatchProject(t *testing.T) (string, string) {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
	t.Setenv("QUORUM_SCHEMAS_DIR", filepath.Join(repoRoot, ".agents", "schemas"))
	root := t.TempDir()
	gitCmd(t, root, "init", "-q", "-b", "main", ".")
	gitCmd(t, root, "config", "user.email", "test@example.com")
	gitCmd(t, root, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(root, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, root, "add", "seed.txt")
	gitCmd(t, root, "commit", "-q", "-m", "init")
	taskID := "FLEET-800"
	taskDir := filepath.Join(root, ".ai", "tasks", "active", taskID)
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	trace := `{"task_id":"` + taskID + `","summary":"cmd dispatch fixture","started_at":"2026-07-12T00:00:00Z","attempts":[],"events":[],"total_cost_usd":0,"violations":[],"context_overflows":[]}`
	if err := os.WriteFile(filepath.Join(taskDir, "07-trace.json"), []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, root, "worktree", "add", filepath.Join(root, "worktrees", taskID), "-b", "ai/"+taskID, "main")
	script := filepath.Join(root, "fake-delegate.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'delegate change\\n' > delegate_made_this.txt\necho done\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	agentsDir := filepath.Join(root, ".agents", "fleet")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	agents := "transports:\n  fake:\n    binary: " + script + "\n    argv_template: []\n    output_format: text\n    timeouts:\n      default_s: 30\n    failure_signatures: []\n    active: true\n    models:\n      test/model-a:\n        model_arg: model-a\n  fake-inactive:\n    binary: " + script + "\n    argv_template: []\n    output_format: text\n    timeouts:\n      default_s: 30\n    failure_signatures: []\n    active: false\n    models:\n      test/model-a:\n        model_arg: model-a\n"
	if err := os.WriteFile(filepath.Join(agentsDir, "agents.yaml"), []byte(agents), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, taskID
}

func TestFleetDispatchCommandHappyPath(t *testing.T) {
	root, taskID := setupFleetDispatchProject(t)
	resultPath, err := runFleetDispatch(core.NewTaskStore(root), fleetDispatchRequest{
		TaskID: taskID, Agent: "fake", Model: "test/model-a", DispatchID: "abc123", TimeoutS: 30,
	})
	if err != nil {
		t.Fatalf("runFleetDispatch: %v", err)
	}
	raw, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatalf("read result.json: %v", err)
	}
	var res core.DispatchResult
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("unmarshal result.json: %v", err)
	}
	if res.Outcome.Class != "attempt" || !res.Applied {
		t.Fatalf("want applied attempt, got class=%s applied=%v", res.Outcome.Class, res.Applied)
	}
	if _, e := os.Stat(filepath.Join(root, "worktrees", taskID, "delegate_made_this.txt")); e != nil {
		t.Fatalf("delegate diff not present in worktree: %v", e)
	}
}

func TestFleetDispatchCommandInactiveTransport(t *testing.T) {
	root, taskID := setupFleetDispatchProject(t)
	_, err := runFleetDispatch(core.NewTaskStore(root), fleetDispatchRequest{
		TaskID: taskID, Agent: "fake-inactive", Model: "test/model-a", DispatchID: "abc123", TimeoutS: 30,
	})
	if err == nil || !strings.Contains(err.Error(), "inactive") {
		t.Fatalf("want inactive-transport error, got %v", err)
	}
	if _, e := os.Stat(filepath.Join(root, "worktrees", taskID, "delegate_made_this.txt")); e == nil {
		t.Fatalf("delegate binary must not execute for an inactive transport")
	}
}

func TestFleetDispatchCommandUnknownAgent(t *testing.T) {
	root, taskID := setupFleetDispatchProject(t)
	_, err := runFleetDispatch(core.NewTaskStore(root), fleetDispatchRequest{
		TaskID: taskID, Agent: "ghost", Model: "test/model-a", DispatchID: "abc123",
	})
	if err == nil || !strings.Contains(err.Error(), "unknown fleet transport") {
		t.Fatalf("want unknown-transport error, got %v", err)
	}
}

// setupPrintTimeoutFakeProject is an agy-shaped fake transport (argv_template
// references {print_timeout} right after --print-timeout, like the real agy
// block). The fake binary records its full argv, one token per line, to
// args.txt so tests assert the actual SUBSTITUTED argv (FLEET-019 AC-2/AC-3).
func setupPrintTimeoutFakeProject(t *testing.T) (string, string) {
	t.Helper()
	root, taskID := setupFleetDispatchProject(t)
	script := filepath.Join(root, "fake-agy.sh")
	body := "#!/bin/sh\nprintf 'delegate change\\n' > delegate_made_this.txt\nprintf '%s\\n' \"$@\" > args.txt\necho done\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	agentsPath := filepath.Join(root, ".agents", "fleet", "agents.yaml")
	raw, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatal(err)
	}
	extra := "  agy-fake:\n    binary: " + script + "\n    argv_template:\n      - --model\n      - \"{model_arg}\"\n      - --print-timeout\n      - \"{print_timeout}\"\n      - --print\n      - \"{prompt}\"\n    output_format: text\n    timeouts:\n      default_s: 300\n    failure_signatures: []\n    active: true\n    models:\n      test/model-a:\n        model_arg: model-a\n"
	if err := os.WriteFile(agentsPath, append(raw, []byte(extra)...), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, taskID
}

// argAfter returns the argv token right after flag, or "" if absent/last.
func argAfter(argv []string, flag string) string {
	for i, tok := range argv {
		if tok == flag && i+1 < len(argv) {
			return argv[i+1]
		}
	}
	return ""
}

// TestFleetDispatchPrintTimeout is FLEET-019 AC-2/AC-3: dispatch's
// substituted argv must carry "--print-timeout" set to the SAME effective
// timeout as the wrapper's hard-kill (explicit timeout_s, or the transport
// default when absent) -- not agy's own hardcoded 5m0s.
func TestFleetDispatchPrintTimeout(t *testing.T) {
	cases := []struct {
		name     string
		timeoutS int
		want     string
	}{
		{"explicit-900s", 900, "15m0s"},
		{"default-300s", 0, "5m0s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, taskID := setupPrintTimeoutFakeProject(t)
			if _, err := runFleetDispatch(core.NewTaskStore(root), fleetDispatchRequest{
				TaskID: taskID, Agent: "agy-fake", Model: "test/model-a", DispatchID: "pt", TimeoutS: tc.timeoutS,
			}); err != nil {
				t.Fatalf("runFleetDispatch: %v", err)
			}
			raw, err := os.ReadFile(filepath.Join(root, "worktrees", taskID, "args.txt"))
			if err != nil {
				t.Fatalf("read args.txt: %v", err)
			}
			argv := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
			if got := argAfter(argv, "--print-timeout"); got != tc.want {
				t.Fatalf("want --print-timeout %s in substituted argv, got argv=%v", tc.want, argv)
			}
		})
	}
}

// TestFleetPrintTimeoutDoesNotLeakIntoCodexOrAider is FLEET-019 AC-5: adding
// print_timeout to the shared vars map is additive. The REAL codex/aider
// argv_templates never declare {print_timeout}, so substitution with/without
// it must render byte-for-byte identical argv.
func TestFleetPrintTimeoutDoesNotLeakIntoCodexOrAider(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
	base := map[string]string{
		"model_arg": "gpt-5.5", "reasoning_effort": "none",
		"worktree": "/tmp/worktree", "out": "/tmp/out.jsonl", "prompt_file": "/tmp/msg.txt", "files": "seed.txt",
	}
	withPT := map[string]string{"print_timeout": formatPrintTimeout(900)}
	for k, v := range base {
		withPT[k] = v
	}
	for _, agent := range []string{"codex", "aider"} {
		transport, err := loadFleetTransport(repoRoot, agent)
		if err != nil {
			t.Fatalf("loadFleetTransport(%s): %v", agent, err)
		}
		before := substituteFleetArgv(transport.ArgvTemplate, base)
		after := substituteFleetArgv(transport.ArgvTemplate, withPT)
		if !reflect.DeepEqual(before, after) {
			t.Fatalf("%s argv changed by adding print_timeout var: before=%v after=%v", agent, before, after)
		}
	}
}

// setupAiderFakeProject extends setupFleetDispatchProject with an
// aider-shaped fake transport (prompt_file/{files} wiring, FLEET-017) and a
// 02-contract.yaml declaring touch:["seed.txt"] so {files} has a concrete
// source. The fake binary is a POSIX sh script that fails loudly (non-zero
// exit) unless the mandatory aider flags and a non-empty --message-file are
// present in its argv -- an indirect but real assertion on the argv the
// wrapper builds.
func setupAiderFakeProject(t *testing.T) (string, string) {
	t.Helper()
	root, taskID := setupFleetDispatchProject(t)
	taskDir := filepath.Join(root, ".ai", "tasks", "active", taskID)
	contract := "task_id: " + taskID + "\ntouch:\n  - seed.txt\nforbid:\n  files: []\n  behaviors: []\ngoal: fixture\nread: []\nsummary: fixture\nverify:\n  commands: []\n"
	if err := os.WriteFile(filepath.Join(taskDir, "02-contract.yaml"), []byte(contract), 0o644); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "fake-aider.sh")
	body := `#!/bin/sh
set -e
args="$*"
case "$args" in
  *--message-file*--no-auto-commits*--no-attribute-co-authored-by*--yes-always*--model*) ;;
  *) echo "missing required flags: $args" 1>&2; exit 9 ;;
esac
msgfile=""
prev=""
for a in "$@"; do
  if [ "$prev" = "--message-file" ]; then msgfile="$a"; fi
  prev="$a"
done
if [ -z "$msgfile" ] || [ ! -s "$msgfile" ]; then
  echo "message file missing or empty: $msgfile" 1>&2
  exit 8
fi
printf 'aider changed\n' >> seed.txt
echo 'Cost: $0.00 message, $0.00 session.'
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	agentsPath := filepath.Join(root, ".agents", "fleet", "agents.yaml")
	raw, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatal(err)
	}
	extra := "  aider-fake:\n    binary: " + script + "\n    argv_template:\n      - --message-file\n      - \"{prompt_file}\"\n      - --no-auto-commits\n      - --no-attribute-co-authored-by\n      - --yes-always\n      - --model\n      - \"{model_arg}\"\n      - \"{files}\"\n    input_channel: prompt_file\n    output_format: text\n    timeouts:\n      default_s: 30\n    failure_signatures: []\n    active: true\n    models:\n      test/model-a:\n        model_arg: openrouter/openrouter/free\n        max_cost_per_call_usd: 0.5\n"
	if err := os.WriteFile(agentsPath, append(raw, []byte(extra)...), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, taskID
}

func TestFleetDispatchAiderPromptFileHappyPath(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key-not-real")
	root, taskID := setupAiderFakeProject(t)
	bundlePath := filepath.Join(t.TempDir(), "bundle.md")
	if err := os.WriteFile(bundlePath, []byte("do the mechanical edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	resultPath, err := runFleetDispatch(core.NewTaskStore(root), fleetDispatchRequest{
		TaskID: taskID, Agent: "aider-fake", Model: "test/model-a", DispatchID: "aider1", TimeoutS: 30,
		BundlePath: bundlePath,
	})
	if err != nil {
		t.Fatalf("runFleetDispatch: %v", err)
	}
	raw, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatalf("read result.json: %v", err)
	}
	var res core.DispatchResult
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("unmarshal result.json: %v", err)
	}
	if res.Outcome.Class != "attempt" || !res.Applied {
		notes, _ := os.ReadFile(filepath.Join(root, ".ai", "tasks", "active", taskID, res.NotesPath))
		t.Fatalf("want applied attempt, got class=%s applied=%v diff=%+v exit=%v notes=%s", res.Outcome.Class, res.Applied, res.Diff, res.ExitCode, string(notes))
	}
	if _, e := os.Stat(filepath.Join(root, ".ai", "tasks", "active", taskID, "dispatch", "aider1", "message.txt")); e != nil {
		t.Fatalf("aider message file not written under taskDir/dispatch: %v", e)
	}
}

func TestFleetDispatchAiderMissingKeyFailsPreflightNoisily(t *testing.T) {
	os.Unsetenv("OPENROUTER_API_KEY")
	root, taskID := setupAiderFakeProject(t)
	_, err := runFleetDispatch(core.NewTaskStore(root), fleetDispatchRequest{
		TaskID: taskID, Agent: "aider-fake", Model: "test/model-a", DispatchID: "aider2", TimeoutS: 30,
	})
	if err == nil || !strings.Contains(err.Error(), "OPENROUTER_API_KEY") {
		t.Fatalf("want noisy OPENROUTER_API_KEY preflight error, got %v", err)
	}
	if _, e := os.Stat(filepath.Join(root, "worktrees", taskID, "seed.txt")); e == nil {
		if content, _ := os.ReadFile(filepath.Join(root, "worktrees", taskID, "seed.txt")); strings.Contains(string(content), "aider changed") {
			t.Fatal("delegate must never run when the preflight key check fails")
		}
	}
}

func TestFleetDispatchAiderBrokenArgvClassifiesWrapperBrokenNotDispatched(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key-not-real")
	root, taskID := setupFleetDispatchProject(t)
	taskDir := filepath.Join(root, ".ai", "tasks", "active", taskID)
	contract := "task_id: " + taskID + "\ntouch:\n  - seed.txt\nforbid:\n  files: []\n  behaviors: []\ngoal: fixture\nread: []\nsummary: fixture\nverify:\n  commands: []\n"
	if err := os.WriteFile(filepath.Join(taskDir, "02-contract.yaml"), []byte(contract), 0o644); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "fake-aider-broken.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho should never run\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	agentsPath := filepath.Join(root, ".agents", "fleet", "agents.yaml")
	raw, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatal(err)
	}
	// Missing --yes-always: ValidateAiderArgv must reject this before exec.
	extra := "  aider-broken:\n    binary: " + script + "\n    argv_template:\n      - --message-file\n      - \"{prompt_file}\"\n      - --no-auto-commits\n      - --no-attribute-co-authored-by\n      - --model\n      - \"{model_arg}\"\n      - \"{files}\"\n    input_channel: prompt_file\n    output_format: text\n    timeouts:\n      default_s: 30\n    failure_signatures: []\n    active: true\n    models:\n      test/model-a:\n        model_arg: openrouter/openrouter/free\n"
	if err := os.WriteFile(agentsPath, append(raw, []byte(extra)...), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = runFleetDispatch(core.NewTaskStore(root), fleetDispatchRequest{
		TaskID: taskID, Agent: "aider-broken", Model: "test/model-a", DispatchID: "aider3", TimeoutS: 30,
	})
	if err == nil || !strings.Contains(err.Error(), "--yes-always") {
		t.Fatalf("want ValidateAiderArgv rejection naming --yes-always, got %v", err)
	}
}

// TestFleetDispatchAiderCostExceedsCeilingAlertsOnStderr is the AC-7
// integration proof the q-review revise finding demanded: a fake aider that
// reports a session cost above the model's max_cost_per_call_usd ceiling
// (0.5, set on test/model-a in setupAiderFakeProject) must make
// runFleetDispatch actually surface a cost_exceeded alert on stderr -- not
// merely prove the pure ParseAiderReportedCost/CostExceedsCeiling functions
// compute the right boolean in unit-test isolation.
func TestFleetDispatchAiderCostExceedsCeilingAlertsOnStderr(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key-not-real")
	root, taskID := setupAiderFakeProject(t)
	writeFakeAiderCostScript(t, root, "0.75") // > 0.5 ceiling
	bundlePath := filepath.Join(t.TempDir(), "bundle.md")
	if err := os.WriteFile(bundlePath, []byte("do the mechanical edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	var (
		resultPath string
		runErr     error
	)
	stderr := captureStderr(t, func() {
		resultPath, runErr = runFleetDispatch(core.NewTaskStore(root), fleetDispatchRequest{
			TaskID: taskID, Agent: "aider-fake", Model: "test/model-a", DispatchID: "aider-cost-over", TimeoutS: 30,
			BundlePath: bundlePath,
		})
	})
	if runErr != nil {
		t.Fatalf("runFleetDispatch: %v", runErr)
	}
	if resultPath == "" {
		t.Fatal("want a result path even when the cost ceiling is exceeded (alert-only, never blocking)")
	}
	if !strings.Contains(stderr, "cost_exceeded") {
		t.Fatalf("want a cost_exceeded alert on stderr for a 0.75 session cost over a 0.5 ceiling, got stderr=%q", stderr)
	}
}

// TestFleetDispatchAiderCostUnderCeilingDoesNotAlert is the negative case:
// a session cost below the ceiling must never emit a cost_exceeded alert.
func TestFleetDispatchAiderCostUnderCeilingDoesNotAlert(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key-not-real")
	root, taskID := setupAiderFakeProject(t)
	writeFakeAiderCostScript(t, root, "0.10") // < 0.5 ceiling
	bundlePath := filepath.Join(t.TempDir(), "bundle.md")
	if err := os.WriteFile(bundlePath, []byte("do the mechanical edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	var runErr error
	stderr := captureStderr(t, func() {
		_, runErr = runFleetDispatch(core.NewTaskStore(root), fleetDispatchRequest{
			TaskID: taskID, Agent: "aider-fake", Model: "test/model-a", DispatchID: "aider-cost-under", TimeoutS: 30,
			BundlePath: bundlePath,
		})
	})
	if runErr != nil {
		t.Fatalf("runFleetDispatch: %v", runErr)
	}
	if strings.Contains(stderr, "cost_exceeded") {
		t.Fatalf("want no cost_exceeded alert for a 0.10 session cost under a 0.5 ceiling, got stderr=%q", stderr)
	}
}

// TestFleetCwdVarDoesNotLeakIntoAgyAiderCodex is FLEET-020 AC-4: adding the
// new "cwd" key to the shared vars map is additive. The REAL agy/aider/codex
// argv_templates never declare {cwd}, so substitution with/without it must
// render byte-for-byte identical argv (same idiom as
// TestFleetPrintTimeoutDoesNotLeakIntoCodexOrAider for {print_timeout}).
func TestFleetCwdVarDoesNotLeakIntoAgyAiderCodex(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
	base := map[string]string{
		"model_arg": "gpt-5.5", "reasoning_effort": "none", "print_timeout": "5m0s",
		"worktree": "/tmp/worktree", "out": "/tmp/out.jsonl", "prompt_file": "/tmp/msg.txt", "files": "seed.txt",
	}
	withCwd := map[string]string{"cwd": "/tmp/some-cwd"}
	for k, v := range base {
		withCwd[k] = v
	}
	for _, agent := range []string{"codex", "agy", "aider"} {
		transport, err := loadFleetTransport(repoRoot, agent)
		if err != nil {
			t.Fatalf("loadFleetTransport(%s): %v", agent, err)
		}
		before := substituteFleetArgv(transport.ArgvTemplate, base)
		after := substituteFleetArgv(transport.ArgvTemplate, withCwd)
		if !reflect.DeepEqual(before, after) {
			t.Fatalf("%s argv changed by adding cwd var: before=%v after=%v", agent, before, after)
		}
	}
}

// TestRealAgyAiderTransportsUnaffectedByEnvAndStdinEmptyFields is FLEET-020
// AC-4: loading the real repo agents.yaml, agy/aider/codex must have zero-value
// Env (nil/empty) and StdinEmpty (false), proving the new optional schema
// fields are no-ops for transports that omit them.
func TestRealAgyAiderTransportsUnaffectedByEnvAndStdinEmptyFields(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
	for _, agent := range []string{"codex", "agy", "aider", "claude"} {
		transport, err := loadFleetTransport(repoRoot, agent)
		if err != nil {
			t.Fatalf("loadFleetTransport(%s): %v", agent, err)
		}
		if len(transport.Env) != 0 {
			t.Fatalf("%s: want no env vars declared, got %v", agent, transport.Env)
		}
		if transport.StdinEmpty {
			t.Fatalf("%s: want stdin_empty absent (false), got true", agent)
		}
	}
}

// TestRealOpencodeTransportLoadsExpectedShape is FLEET-020 AC-1: the real
// opencode block in .agents/fleet/agents.yaml loads with the validated
// headless recipe.
func TestRealOpencodeTransportLoadsExpectedShape(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
	transport, err := loadFleetTransport(repoRoot, "opencode")
	if err != nil {
		t.Fatalf("loadFleetTransport(opencode): %v", err)
	}
	if transport.Binary != "opencode" {
		t.Fatalf("want binary opencode, got %q", transport.Binary)
	}
	if !transport.Active {
		t.Fatal("want opencode active:true")
	}
	if transport.Env["OPENCODE_DISABLE_AUTOUPDATE"] != "true" {
		t.Fatalf("want env OPENCODE_DISABLE_AUTOUPDATE=true, got %v", transport.Env)
	}
	if !transport.StdinEmpty {
		t.Fatal("want stdin_empty true for opencode")
	}
	model, ok := transport.Models["openrouter/free"]
	if !ok {
		t.Fatalf("want model openrouter/free declared, got %v", transport.Models)
	}
	if stringField(model, "model_arg") != "openrouter/openrouter/free" {
		t.Fatalf("want model_arg openrouter/openrouter/free, got %v", model)
	}
	wantArgv := []string{"run", "{prompt}", "-m", "{model_arg}", "--format", "json", "--dir", "{cwd}", "--auto"}
	if !reflect.DeepEqual(transport.ArgvTemplate, wantArgv) {
		t.Fatalf("want argv_template %v, got %v", wantArgv, transport.ArgvTemplate)
	}
}

// setupFleetOpencodeFakeProject extends setupFleetDispatchProject with an
// opencode-shaped fake transport: prompt travels as a trailing argv token
// ({prompt}), {cwd} substitutes to the worktree, env carries a marker var,
// and stdin_empty is true. The fake binary records its argv and its stdin
// content to files so tests can assert both.
func setupFleetOpencodeFakeProject(t *testing.T) (string, string) {
	t.Helper()
	root, taskID := setupFleetDispatchProject(t)
	script := filepath.Join(root, "fake-opencode.sh")
	body := "#!/bin/sh\nprintf '%s\\n' \"$@\" > args.txt\ncat > stdin.txt\nprintf '%s' \"$FLEET_TEST_ENV_MARKER\" > env.txt\nprintf 'delegate change\\n' > delegate_made_this.txt\necho done\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	agentsPath := filepath.Join(root, ".agents", "fleet", "agents.yaml")
	raw, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatal(err)
	}
	extra := "  opencode-fake:\n" +
		"    binary: " + script + "\n" +
		"    env:\n" +
		"      FLEET_TEST_ENV_MARKER: marker-value\n" +
		"    argv_template:\n" +
		"      - run\n" +
		"      - \"{prompt}\"\n" +
		"      - -m\n" +
		"      - \"{model_arg}\"\n" +
		"      - --dir\n" +
		"      - \"{cwd}\"\n" +
		"    input_channel: prompt_arg\n" +
		"    output_format: json\n" +
		"    stdin_empty: true\n" +
		"    timeouts:\n" +
		"      default_s: 30\n" +
		"    failure_signatures: []\n" +
		"    active: true\n" +
		"    models:\n" +
		"      test/model-a:\n" +
		"        model_arg: model-a\n"
	if err := os.WriteFile(agentsPath, append(raw, []byte(extra)...), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, taskID
}

// TestFleetDispatchOpencodeCwdEnvAndStdinEmpty is FLEET-020 AC-1/AC-4: the
// {cwd} placeholder resolves to the worktree, transport.Env is applied to the
// process environment before exec (observed by the fake binary), and
// stdin_empty forces an empty stdin even though the prompt still arrives via
// argv.
func TestFleetDispatchOpencodeCwdEnvAndStdinEmpty(t *testing.T) {
	os.Unsetenv("FLEET_TEST_ENV_MARKER")
	t.Cleanup(func() { os.Unsetenv("FLEET_TEST_ENV_MARKER") })
	root, taskID := setupFleetOpencodeFakeProject(t)
	bundlePath := filepath.Join(t.TempDir(), "bundle.md")
	if err := os.WriteFile(bundlePath, []byte("do the mechanical edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runFleetDispatch(core.NewTaskStore(root), fleetDispatchRequest{
		TaskID: taskID, Agent: "opencode-fake", Model: "test/model-a", DispatchID: "oc1", TimeoutS: 30,
		BundlePath: bundlePath,
	}); err != nil {
		t.Fatalf("runFleetDispatch: %v", err)
	}
	worktree := filepath.Join(root, "worktrees", taskID)
	argvRaw, err := os.ReadFile(filepath.Join(worktree, "args.txt"))
	if err != nil {
		t.Fatalf("read args.txt: %v", err)
	}
	argv := strings.Split(strings.TrimRight(string(argvRaw), "\n"), "\n")
	if got := argAfter(argv, "--dir"); got != worktree {
		t.Fatalf("want --dir %s (cwd substituted to worktree), got argv=%v", worktree, argv)
	}
	envRaw, err := os.ReadFile(filepath.Join(worktree, "env.txt"))
	if err != nil {
		t.Fatalf("read env.txt: %v", err)
	}
	if string(envRaw) != "marker-value" {
		t.Fatalf("want transport.Env applied before exec, got env.txt=%q", envRaw)
	}
	stdinRaw, err := os.ReadFile(filepath.Join(worktree, "stdin.txt"))
	if err != nil {
		t.Fatalf("read stdin.txt: %v", err)
	}
	if len(stdinRaw) != 0 {
		t.Fatalf("want empty stdin for stdin_empty:true transport, got stdin.txt=%q", stdinRaw)
	}
}

// TestQuorumFleetAgentsEnvOverride is FLEET-020 AC-3: QUORUM_FLEET_AGENTS,
// when set, overrides the agents.yaml path loadFleetTransport reads; when
// unset, it falls back to <projectRoot>/.agents/fleet/agents.yaml (mirroring
// internal/core/schema.go's SchemasDir env-first-then-fallback precedent).
func TestQuorumFleetAgentsEnvOverride(t *testing.T) {
	root, _ := setupFleetDispatchProject(t)

	t.Run("unset falls back to project root agents.yaml", func(t *testing.T) {
		transport, err := loadFleetTransport(root, "fake")
		if err != nil {
			t.Fatalf("loadFleetTransport: %v", err)
		}
		if transport.Binary == "" {
			t.Fatal("want the project-root fake transport to load")
		}
	})

	t.Run("set overrides the path", func(t *testing.T) {
		altDir := t.TempDir()
		altAgentsDir := filepath.Join(altDir, "somewhere-else")
		if err := os.MkdirAll(altAgentsDir, 0o755); err != nil {
			t.Fatal(err)
		}
		altPath := filepath.Join(altAgentsDir, "agents.yaml")
		alt := "transports:\n  only-here:\n    binary: /bin/true\n    argv_template: []\n    output_format: text\n    timeouts:\n      default_s: 30\n    failure_signatures: []\n    active: true\n    models: {}\n"
		if err := os.WriteFile(altPath, []byte(alt), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Setenv("QUORUM_FLEET_AGENTS", altPath)
		// A project root that has NO .agents/fleet/agents.yaml of its own:
		// the override must still resolve "only-here", proving the env var
		// wins over the project-root default rather than merely supplementing it.
		emptyRoot := t.TempDir()
		if _, err := loadFleetTransport(emptyRoot, "only-here"); err != nil {
			t.Fatalf("loadFleetTransport with QUORUM_FLEET_AGENTS set: %v", err)
		}
		if _, err := loadFleetTransport(emptyRoot, "fake"); err == nil {
			t.Fatal("want the project-root's own (nonexistent) agents.yaml to NOT be consulted when QUORUM_FLEET_AGENTS is set")
		}
	})
}

func TestFleetDispatchBundleHashFromManifest(t *testing.T) {
	t.Run("happy path - valid manifest", func(t *testing.T) {
		root, taskID := setupFleetDispatchProject(t)
		dispatchID := "happy123"
		dispatchDir := filepath.Join(root, ".ai", "tasks", "active", taskID, "dispatch", dispatchID)
		if err := os.MkdirAll(dispatchDir, 0755); err != nil {
			t.Fatal(err)
		}

		promptPath := filepath.Join(dispatchDir, "prompt.md")
		if err := os.WriteFile(promptPath, []byte("test prompt"), 0644); err != nil {
			t.Fatal(err)
		}

		manifestJSON := `{"bundle_hash": "abc123def456hash"}`
		manifestPath := filepath.Join(dispatchDir, "manifest.json")
		if err := os.WriteFile(manifestPath, []byte(manifestJSON), 0644); err != nil {
			t.Fatal(err)
		}

		_, err := runFleetDispatch(core.NewTaskStore(root), fleetDispatchRequest{
			TaskID: taskID, Agent: "fake", Model: "test/model-a", DispatchID: dispatchID, TimeoutS: 30,
			BundlePath: promptPath,
		})
		if err != nil {
			t.Fatalf("runFleetDispatch: %v", err)
		}

		tracePath := filepath.Join(root, ".ai", "tasks", "active", taskID, "07-trace.json")
		payload, err := core.LoadArtifactPayload(tracePath)
		if err != nil {
			t.Fatalf("LoadArtifactPayload: %v", err)
		}
		traceMap, ok := payload.(map[string]any)
		if !ok {
			t.Fatal("trace payload is not map")
		}
		events, ok := traceMap["events"].([]any)
		if !ok {
			t.Fatal("events is not a slice")
		}

		found := false
		for _, ev := range events {
			m, ok := ev.(map[string]any)
			if !ok {
				continue
			}
			if m["type"] == "dispatch_started" && m["dispatch_id"] == dispatchID {
				found = true
				if m["bundle_hash"] != "abc123def456hash" {
					t.Errorf("want bundle_hash 'abc123def456hash', got %v", m["bundle_hash"])
				}
			}
		}
		if !found {
			t.Errorf("dispatch_started event for %s not found in trace", dispatchID)
		}
	})

	// "degrades gracefully" is a table-driven consolidation (AC-3/AC-4) of what
	// were three separate near-identical subtests: missing manifest.json,
	// empty BundlePath, and malformed/incomplete manifest JSON. All four cases
	// below share the same scaffold and only vary how (or whether) the
	// manifest is written and what BundlePath is passed; every case must
	// still produce a dispatch_started event with an empty bundle_hash and
	// runFleetDispatch must never error. This does not remove coverage --
	// each original scenario is still exercised as its own t.Run case.
	t.Run("degrades gracefully", func(t *testing.T) {
		cases := []struct {
			name            string
			writeManifest   bool
			manifestBody    string
			emptyBundlePath bool
		}{
			{name: "missing manifest", writeManifest: false},
			{name: "empty BundlePath", writeManifest: false, emptyBundlePath: true},
			{name: "malformed json", writeManifest: true, manifestBody: `{"bundle_hash":`},
			{name: "missing key", writeManifest: true, manifestBody: `{"other_key": "val"}`},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				root, taskID := setupFleetDispatchProject(t)
				dispatchID := "degrade-" + strings.ReplaceAll(tc.name, " ", "-")
				dispatchDir := filepath.Join(root, ".ai", "tasks", "active", taskID, "dispatch", dispatchID)
				if err := os.MkdirAll(dispatchDir, 0755); err != nil {
					t.Fatal(err)
				}

				bundlePath := ""
				if !tc.emptyBundlePath {
					promptPath := filepath.Join(dispatchDir, "prompt.md")
					if err := os.WriteFile(promptPath, []byte("test prompt"), 0644); err != nil {
						t.Fatal(err)
					}
					bundlePath = promptPath
				}
				if tc.writeManifest {
					manifestPath := filepath.Join(dispatchDir, "manifest.json")
					if err := os.WriteFile(manifestPath, []byte(tc.manifestBody), 0644); err != nil {
						t.Fatal(err)
					}
				}

				_, err := runFleetDispatch(core.NewTaskStore(root), fleetDispatchRequest{
					TaskID: taskID, Agent: "fake", Model: "test/model-a", DispatchID: dispatchID, TimeoutS: 30,
					BundlePath: bundlePath,
				})
				if err != nil {
					t.Fatalf("runFleetDispatch: %v", err)
				}

				tracePath := filepath.Join(root, ".ai", "tasks", "active", taskID, "07-trace.json")
				payload, err := core.LoadArtifactPayload(tracePath)
				if err != nil {
					t.Fatalf("LoadArtifactPayload: %v", err)
				}
				traceMap, ok := payload.(map[string]any)
				if !ok {
					t.Fatal("trace payload is not map")
				}
				events := traceMap["events"].([]any)
				found := false
				for _, ev := range events {
					m, ok := ev.(map[string]any)
					if !ok {
						continue
					}
					if m["type"] == "dispatch_started" && m["dispatch_id"] == dispatchID {
						found = true
						if m["bundle_hash"] != "" {
							t.Errorf("want empty bundle_hash, got %v", m["bundle_hash"])
						}
					}
				}
				if !found {
					t.Errorf("dispatch_started event for %s not found in trace", dispatchID)
				}
			})
		}
	})
}
