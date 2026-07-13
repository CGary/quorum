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
