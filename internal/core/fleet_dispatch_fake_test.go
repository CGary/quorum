package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

type dispatchEnv struct {
	root, taskID, taskDir, worktree string
}

const dispatchTestTaskID = "FLEET-700"

func setupDispatchEnv(t *testing.T) dispatchEnv {
	t.Helper()
	useSchemas(t)
	root := initGitRepo(t)
	ensureTaskDirs(t, root)
	taskDir := filepath.Join(root, ".ai", "tasks", "active", dispatchTestTaskID)
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	trace := `{"task_id":"` + dispatchTestTaskID + `","summary":"dispatch engine fixture","started_at":"2026-07-12T00:00:00Z","attempts":[],"events":[],"total_cost_usd":0,"violations":[],"context_overflows":[]}`
	if err := os.WriteFile(filepath.Join(taskDir, "07-trace.json"), []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	worktree := filepath.Join(root, "worktrees", dispatchTestTaskID)
	run(t, root, "git", "worktree", "add", worktree, "-b", "ai/"+dispatchTestTaskID, "main")
	return dispatchEnv{root: root, taskID: dispatchTestTaskID, taskDir: taskDir, worktree: worktree}
}

func (e dispatchEnv) fakeSpec(dispatchID string) DispatchSpec {
	return DispatchSpec{
		TaskID: e.taskID, TaskDir: e.taskDir, Agent: "fake", Model: "openai/gpt-5.5-medium",
		DispatchID: dispatchID, BundleHash: "cafebabecafe", Worktree: e.worktree,
		Binary: os.Args[0], StdinPrompt: "do the work", TimeoutS: 30, OutputFormat: "text",
	}
}
func (e dispatchEnv) dispatchDir(dispatchID string) string {
	return filepath.Join(e.taskDir, "dispatch", dispatchID)
}

func execCommandFake(mode string) *exec.Cmd {
	c := exec.Command(os.Args[0])
	c.Env = append(envWithout(os.Environ(), "FLEET_FAKE_MODE"), "FLEET_FAKE_MODE="+mode)
	return c
}
func loadTraceEvents(t *testing.T, taskDir string) []map[string]any {
	t.Helper()
	payload, err := LoadArtifactPayload(filepath.Join(taskDir, "07-trace.json"))
	if err != nil {
		t.Fatalf("load trace: %v", err)
	}
	raw, _ := asSlice(payload.(map[string]any)["events"])
	out := make([]map[string]any, 0, len(raw))
	for _, e := range raw {
		if m, ok := e.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}
func eventTypesInTrace(events []map[string]any) []string {
	var types []string
	for _, e := range events {
		if t, ok := e["type"].(string); ok {
			types = append(types, t)
		}
	}
	return types
}
func containsStr(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
