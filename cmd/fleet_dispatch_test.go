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
	agents := "transports:\n  fake:\n    binary: " + script + "\n    argv_template: []\n    output_format: text\n    timeouts:\n      default_s: 30\n    failure_signatures: []\n    models:\n      test/model-a:\n        model_arg: model-a\n"
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

func TestFleetDispatchCommandUnknownAgent(t *testing.T) {
	root, taskID := setupFleetDispatchProject(t)
	_, err := runFleetDispatch(core.NewTaskStore(root), fleetDispatchRequest{
		TaskID: taskID, Agent: "ghost", Model: "test/model-a", DispatchID: "abc123",
	})
	if err == nil || !strings.Contains(err.Error(), "unknown fleet transport") {
		t.Fatalf("want unknown-transport error, got %v", err)
	}
}

// setupFleetDispatchCodexProject mirrors setupFleetDispatchProject but wires a
// fake codex transport: a shell script standing in for the real `codex`
// binary (self-exec/fake-binary pattern per FLEET-006-a precedent, adapted
// here to a POSIX shell script since cmd's existing fixtures already use
// scripts rather than a Go self-exec harness). The script finds the `-o`
// argv token codex uses for its last-message file, emits a JSONL usage event
// on stdout, and writes the last-message file, so tests can drive the real
// runFleetDispatch -> core.Dispatch -> codex-enrichment path end to end.
func setupFleetDispatchCodexProject(t *testing.T, scriptBody string) (string, string) {
	t.Helper()
	root, taskID := setupFleetDispatchProject(t)
	scriptPath := filepath.Join(root, "fake-codex.sh")
	if err := os.WriteFile(scriptPath, []byte(scriptBody), 0o755); err != nil {
		t.Fatal(err)
	}
	agents := "transports:\n  codex:\n    binary: " + scriptPath + "\n" +
		"    argv_template: [\"-o\", \"{out}\"]\n" +
		"    output_format: jsonl\n" +
		"    timeouts:\n      default_s: 30\n" +
		"    failure_signatures:\n      - You've hit your usage limit\n" +
		"    models:\n      test/codex-model:\n        model_arg: codex-model\n"
	if err := os.WriteFile(filepath.Join(root, ".agents", "fleet", "agents.yaml"), []byte(agents), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, taskID
}

// TestFleetDispatchCommandCodexUsageEnrichment covers AC-7's end-to-end
// enrichment scenario: a fake codex binary emits a JSONL usage event and
// writes a last-message file at the {out} path; result.json must come back
// with usage.source=cli_reported and the parsed counts, applied via
// ApplyDispatchResultUsage after core.Dispatch returns, with every other
// result.json field left exactly as the unmodified engine wrote it.
func TestFleetDispatchCommandCodexUsageEnrichment(t *testing.T) {
	script := "#!/bin/sh\n" +
		"out=\"\"\nprev=\"\"\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ \"$prev\" = \"-o\" ]; then out=\"$arg\"; fi\n" +
		"  prev=\"$arg\"\n" +
		"done\n" +
		"printf 'delegate change\\n' > delegate_made_this.txt\n" +
		"echo '{\"type\":\"agent_message\",\"input_tokens\":120,\"output_tokens\":34}'\n" +
		"printf 'final answer from codex\\n' > \"$out\"\n"
	root, taskID := setupFleetDispatchCodexProject(t, script)
	resultPath, err := runFleetDispatch(core.NewTaskStore(root), fleetDispatchRequest{
		TaskID: taskID, Agent: "codex", Model: "test/codex-model", DispatchID: "codexrun", TimeoutS: 30,
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
	if res.Usage.Source != "cli_reported" {
		t.Fatalf("want usage.source=cli_reported, got %q", res.Usage.Source)
	}
	if res.Usage.TokensIn == nil || *res.Usage.TokensIn != 120 {
		t.Fatalf("want tokens_in=120, got %+v", res.Usage.TokensIn)
	}
	if res.Usage.TokensOut == nil || *res.Usage.TokensOut != 34 {
		t.Fatalf("want tokens_out=34, got %+v", res.Usage.TokensOut)
	}
	if res.Outcome.Class != "attempt" || !res.Applied {
		t.Fatalf("want an unaltered applied attempt outcome, got class=%s applied=%v", res.Outcome.Class, res.Applied)
	}
}

// TestFleetDispatchCommandCodexQuotaSignatureReroute covers AC-7's data-only
// quota/auth classification scenario: adding a codex quota phrase to
// agents.yaml's failure_signatures is enough, through the UNMODIFIED engine,
// to classify a nonzero-exit/empty-diff run as outcome.class=reroute,
// cause=quota -- proving the addition is pure data.
func TestFleetDispatchCommandCodexQuotaSignatureReroute(t *testing.T) {
	script := "#!/bin/sh\necho \"You've hit your usage limit\" 1>&2\nexit 1\n"
	root, taskID := setupFleetDispatchCodexProject(t, script)
	resultPath, err := runFleetDispatch(core.NewTaskStore(root), fleetDispatchRequest{
		TaskID: taskID, Agent: "codex", Model: "test/codex-model", DispatchID: "codexquota", TimeoutS: 30,
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
	if res.Outcome.Class != "reroute" || res.Outcome.Cause == nil || *res.Outcome.Cause != "quota" {
		t.Fatalf("want reroute/quota outcome, got class=%s cause=%v", res.Outcome.Class, res.Outcome.Cause)
	}
}
