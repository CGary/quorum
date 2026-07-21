package cmd

import (
	"bytes"
	"strings"
	"testing"
	
	"quorum/internal/core"
)

func TestFleetEnable(t *testing.T) {
	root := t.TempDir()
	
	state := core.ControlState{
		Disabled: []core.ControlEntry{
			{Target: "agy"},
		},
	}
	core.SaveFleetControlState(root, state)

	var out, errBuf bytes.Buffer
	code := runFleetEnable(root, "agy", &out, &errBuf)
	if code != 0 {
		t.Errorf("expected zero exit, got %d", code)
	}

	if !strings.Contains(out.String(), "Enabled agy") {
		t.Errorf("expected success message, got %s", out.String())
	}

	st, _ := core.LoadFleetControlState(root)
	if len(st.Disabled) != 0 {
		t.Errorf("expected target to be removed, got %v", st.Disabled)
	}
}
