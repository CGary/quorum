package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"quorum/internal/core"
)

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
	extra := "  aider-fake:\n    binary: " + script + "\n    argv_template:\n      - --message-file\n      - \"{prompt_file}\"\n      - --no-auto-commits\n      - --no-attribute-co-authored-by\n      - --yes-always\n      - --model\n      - \"{model_arg}\"\n      - \"{files}\"\n    input_channel: prompt_file\n    output_format: text\n    timeouts:\n      default_s: 30\n    failure_signatures: []\n    active: true\n    models:\n      test/model-a:\n        model_arg: openrouter/openrouter/free\n"
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
