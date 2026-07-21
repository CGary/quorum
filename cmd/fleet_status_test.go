package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"quorum/internal/core"
)

func TestFleetStatus(t *testing.T) {
	root := t.TempDir()
	
	state := core.ControlState{
		Disabled: []core.ControlEntry{
			{Target: "agy", Reason: "rate limited", By: "human", At: time.Now().UTC().Format(time.RFC3339)},
		},
	}
	core.SaveFleetControlState(root, state)

	var out, errBuf bytes.Buffer
	
	// plain text
	code := runFleetStatus(root, false, &out, &errBuf)
	if code != 0 {
		t.Errorf("expected zero exit, got %d", code)
	}
	if !strings.Contains(out.String(), "Target: agy") {
		t.Errorf("expected plain text status output, got %s", out.String())
	}

	// json
	out.Reset()
	code = runFleetStatus(root, true, &out, &errBuf)
	if code != 0 {
		t.Errorf("expected zero exit, got %d", code)
	}
	
	var report core.FleetStatusReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(report.Disabled) != 1 || report.Disabled[0].Target != "agy" {
		t.Errorf("expected valid JSON report, got %v", report)
	}
}
