package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFleetDisable(t *testing.T) {
	root := t.TempDir()
	// core.FleetAgentsPath honors QUORUM_FLEET_AGENTS (2026-09-09). Without this
	// the fixture below is shadowed by whatever catalog the invoking shell
	// exports, and the test silently validates against the real repo instead.
	t.Setenv("QUORUM_FLEET_AGENTS", "")
	content := `
transports:
  agy:
    models: {}
`
	agentsDir := filepath.Join(root, ".agents", "fleet")
	os.MkdirAll(agentsDir, 0755)
	os.WriteFile(filepath.Join(agentsDir, "agents.yaml"), []byte(content), 0644)

	var out, errBuf bytes.Buffer

	// missing reason
	code := runFleetDisable(root, "agy", "", &out, &errBuf)
	if code == 0 {
		t.Errorf("expected non-zero exit for missing reason")
	}
	if !strings.Contains(errBuf.String(), "--reason is required") {
		t.Errorf("expected missing reason error, got %s", errBuf.String())
	}

	// space reason
	errBuf.Reset()
	code = runFleetDisable(root, "agy", "   ", &out, &errBuf)
	if code == 0 {
		t.Errorf("expected non-zero exit for empty reason")
	}
	
	// bogus target
	errBuf.Reset()
	code = runFleetDisable(root, "bogus", "reason", &out, &errBuf)
	if code == 0 {
		t.Errorf("expected non-zero exit for bogus target")
	}
	if !strings.Contains(errBuf.String(), "unknown transport") {
		t.Errorf("expected unknown transport error, got %s", errBuf.String())
	}
	// check file absent
	if _, err := os.Stat(filepath.Join(root, ".ai", "fleet-control.json")); !os.IsNotExist(err) {
		t.Errorf("expected file not to exist after bogus target")
	}

	// success
	errBuf.Reset()
	out.Reset()
	code = runFleetDisable(root, "agy", "reason", &out, &errBuf)
	if code != 0 {
		t.Errorf("expected zero exit for success, got %d", code)
	}
	if !strings.Contains(out.String(), "Disabled agy") {
		t.Errorf("expected success message, got %s", out.String())
	}
}
