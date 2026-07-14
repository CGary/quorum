package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"quorum/internal/core"
)

func TestFleetSmokeCommandHappyPath(t *testing.T) {
	root, taskID := setupFleetDispatchProject(t)
	resultPath, err := runFleetSmoke(core.NewTaskStore(root), "fake", taskID)
	if err != nil {
		t.Fatalf("runFleetSmoke: %v", err)
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
	if res.Model != "test/model-a" {
		t.Fatalf("want deterministic model selection test/model-a, got %q", res.Model)
	}
	if _, e := os.Stat(filepath.Join(root, "worktrees", taskID, "delegate_made_this.txt")); e != nil {
		t.Fatalf("delegate diff not present in worktree: %v", e)
	}
}

// TestFleetSmokePrintTimeoutMatchesTransportDefault is FLEET-019 AC-4 (smoke
// half): smoke's effective timeout is always transport.Timeouts.DefaultS
// (300s here), so the substituted argv must carry "--print-timeout" "5m0s".
func TestFleetSmokePrintTimeoutMatchesTransportDefault(t *testing.T) {
	root, taskID := setupPrintTimeoutFakeProject(t)
	if _, err := runFleetSmoke(core.NewTaskStore(root), "agy-fake", taskID); err != nil {
		t.Fatalf("runFleetSmoke: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "worktrees", taskID, "args.txt"))
	if err != nil {
		t.Fatalf("read args.txt: %v", err)
	}
	argv := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if got := argAfter(argv, "--print-timeout"); got != "5m0s" {
		t.Fatalf("want --print-timeout 5m0s in substituted argv, got argv=%v", argv)
	}
}

func TestFleetSmokeCommandUnknownAgent(t *testing.T) {
	root, taskID := setupFleetDispatchProject(t)
	_, err := runFleetSmoke(core.NewTaskStore(root), "ghost", taskID)
	if err == nil || !strings.Contains(err.Error(), "unknown fleet transport") {
		t.Fatalf("want unknown-transport error, got %v", err)
	}
}

func TestFleetSmokeCommandUnknownTask(t *testing.T) {
	root, _ := setupFleetDispatchProject(t)
	_, err := runFleetSmoke(core.NewTaskStore(root), "fake", "FLEET-999")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("want task-not-found error, got %v", err)
	}
}

func TestFleetSmokeCommandRequiresAgentAndTaskID(t *testing.T) {
	root, taskID := setupFleetDispatchProject(t)
	if _, err := runFleetSmoke(core.NewTaskStore(root), "", taskID); err == nil {
		t.Fatal("want error when agent is empty")
	}
	if _, err := runFleetSmoke(core.NewTaskStore(root), "fake", ""); err == nil {
		t.Fatal("want error when task_id is empty")
	}
}

func TestFleetSmokeCommandRegisteredManualOnly(t *testing.T) {
	// AC-4/D8: the smoke subcommand must exist and be reachable only via
	// explicit CLI invocation -- this test only asserts registration under
	// 'quorum fleet', never that it is wired into any automated path.
	found := false
	for _, c := range fleetCmd.Commands() {
		if c.Name() == "smoke" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("want 'smoke' registered as a subcommand of 'quorum fleet'")
	}
}
