package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestFleetControlRoundTrip(t *testing.T) {
	root := t.TempDir()
	state := ControlState{
		Disabled: []ControlEntry{
			{Target: "agy", Reason: "rate limited", By: "human", At: time.Now().UTC().Format(time.RFC3339)},
		},
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	err := SaveFleetControlState(root, state)
	if err != nil {
		t.Fatalf("SaveFleetControlState failed: %v", err)
	}

	// check no temp files
	aiDir := filepath.Join(root, ".ai")
	files, _ := os.ReadDir(aiDir)
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".tmp" {
			t.Errorf("found leftover tmp file: %s", f.Name())
		}
	}

	loaded, err := LoadFleetControlState(root)
	if err != nil {
		t.Fatalf("LoadFleetControlState failed: %v", err)
	}

	if len(loaded.Disabled) != 1 || loaded.Disabled[0].Target != "agy" {
		t.Fatalf("unexpected loaded state: %+v", loaded)
	}
}

func setupTempAgentsYaml(t *testing.T, root string) {
	t.Helper()
	// Isolate from any QUORUM_FLEET_AGENTS exported in the invoking shell:
	// FleetAgentsPath honors it, and it would shadow this temp catalog.
	t.Setenv("QUORUM_FLEET_AGENTS", "")
	content := `
transports:
  agy:
    models:
      fake/model-low:
        provider: google
  codex:
    models:
      openai/gpt-5.5-none:
        provider: openai
`
	agentsDir := filepath.Join(root, ".agents", "fleet")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentsDir, "agents.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestFleetControlValidateFleetTarget(t *testing.T) {
	root := t.TempDir()
	setupTempAgentsYaml(t, root)

	tests := []struct {
		target string
		err    bool
	}{
		{"agy", false},
		{"agy/fake/model-low", false},
		{"codex", false},
		{"bogus-agent", true},
		{"agy/bogus-model", true},
	}

	for _, tt := range tests {
		err := ValidateFleetTarget(root, tt.target)
		if (err != nil) != tt.err {
			t.Errorf("ValidateFleetTarget(%q) err = %v, want err %v", tt.target, err, tt.err)
		}
	}
}

func TestFleetControlDisableAndEnable(t *testing.T) {
	root := t.TempDir()
	setupTempAgentsYaml(t, root)

	_, err := DisableFleetTarget(root, "agy", "rate limited", "human")
	if err != nil {
		t.Fatalf("DisableFleetTarget failed: %v", err)
	}

	state, _ := LoadFleetControlState(root)
	if len(state.Disabled) != 1 || state.Disabled[0].Target != "agy" || state.Disabled[0].Reason != "rate limited" {
		t.Fatalf("unexpected state after disable: %+v", state)
	}

	// upsert replace
	_, err = DisableFleetTarget(root, "agy", "new reason", "human")
	if err != nil {
		t.Fatalf("DisableFleetTarget failed: %v", err)
	}

	state, _ = LoadFleetControlState(root)
	if len(state.Disabled) != 1 || state.Disabled[0].Target != "agy" || state.Disabled[0].Reason != "new reason" {
		t.Fatalf("unexpected state after disable upsert: %+v", state)
	}

	// enable
	_, err = EnableFleetTarget(root, "agy")
	if err != nil {
		t.Fatalf("EnableFleetTarget failed: %v", err)
	}

	state, _ = LoadFleetControlState(root)
	if len(state.Disabled) != 0 {
		t.Fatalf("unexpected state after enable: %+v", state)
	}

	// idempotent enable
	_, err = EnableFleetTarget(root, "agy")
	if err != nil {
		t.Fatalf("EnableFleetTarget failed: %v", err)
	}
}

func TestFleetControlConcurrency(t *testing.T) {
	root := t.TempDir()
	t.Setenv("QUORUM_FLEET_AGENTS", "") // see setupTempAgentsYaml
	// write enough dummy models for 20 goroutines
	agentsDir := filepath.Join(root, ".agents", "fleet")
	os.MkdirAll(agentsDir, 0755)
	os.WriteFile(filepath.Join(agentsDir, "agents.yaml"), []byte("transports:\n  agy:\n    models:\n"), 0644)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			DisableFleetTarget(root, "agy", "reason", "human")
		}(i)
	}
	wg.Wait()

	// must be valid json
	b, err := os.ReadFile(FleetControlPath(root))
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	var state ControlState
	if err := json.Unmarshal(b, &state); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
}

func TestFleetControlSideEffectIsolation(t *testing.T) {
	root := t.TempDir()
	setupTempAgentsYaml(t, root)

	taskDir := filepath.Join(root, ".ai", "tasks", "active", "T-1")
	os.MkdirAll(taskDir, 0755)
	os.WriteFile(filepath.Join(taskDir, "07-trace.json"), []byte("{}"), 0644)

	DisableFleetTarget(root, "agy", "reason", "human")
	EnableFleetTarget(root, "agy")

	b, err := os.ReadFile(filepath.Join(taskDir, "07-trace.json"))
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if string(b) != "{}" {
		t.Fatalf("file changed!")
	}
}

// TestFleetAgentsPathEnvOverride pins that control-state validation resolves
// agents.yaml the same way dispatch does: QUORUM_FLEET_AGENTS first, project
// root second. A consumer project sharing Quorum's catalog through the env var
// has no .agents/fleet/ of its own, and without this the kill-switch CLI failed
// there while route/dispatch worked.
func TestFleetAgentsPathEnvOverride(t *testing.T) {
	root := t.TempDir()
	setupTempAgentsYaml(t, root)

	if got, want := FleetAgentsPath(root), filepath.Join(root, ".agents", "fleet", "agents.yaml"); got != want {
		t.Errorf("FleetAgentsPath without env = %q, want %q", got, want)
	}

	// A project root with no catalog of its own, pointed at the shared one.
	consumer := t.TempDir()
	if err := ValidateFleetTarget(consumer, "agy"); err == nil {
		t.Fatal("want an error validating against a project root with no agents.yaml")
	}

	t.Setenv("QUORUM_FLEET_AGENTS", filepath.Join(root, ".agents", "fleet", "agents.yaml"))
	if got, want := FleetAgentsPath(consumer), filepath.Join(root, ".agents", "fleet", "agents.yaml"); got != want {
		t.Errorf("FleetAgentsPath with env = %q, want %q", got, want)
	}
	if err := ValidateFleetTarget(consumer, "agy"); err != nil {
		t.Errorf("ValidateFleetTarget with QUORUM_FLEET_AGENTS set: %v", err)
	}
	if err := ValidateFleetTarget(consumer, "bogus-agent"); err == nil {
		t.Error("want an unknown transport to still be rejected via the env-resolved catalog")
	}
}
