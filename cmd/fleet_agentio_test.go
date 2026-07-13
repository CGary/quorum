package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestFleetAgentEnvelopeSuccessShape(t *testing.T) {
	var out, errW bytes.Buffer
	emit := fleetEmit{JSON: true}
	emit.success(&out, &errW, fleetSuccessEnvelope{
		OK: true, Command: "fleet.run", Summary: "ran", Data: map[string]any{"exit_code": 0},
		NextActions: []fleetNextAction{},
	})
	// Exactly one JSON object on stdout, nothing on stderr.
	var m map[string]any
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatalf("stdout not a single JSON object: %v\n%s", err, out.String())
	}
	for _, k := range []string{"ok", "command", "summary", "data", "next_actions"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("success envelope missing key %q: %v", k, m)
		}
	}
	if errW.Len() != 0 {
		t.Fatalf("stderr must stay empty for a clean success: %q", errW.String())
	}
	if strings.ContainsAny(out.String(), "🚀✅") {
		t.Fatal("no emoji/banner allowed on stdout under --json")
	}
}

func TestFleetAgentErrorConstructorAndShape(t *testing.T) {
	env := fleetAgentError("fleet.run", errCodeInvalidEnum, "--model must be one of: a, b", "model", "z", true, "quorum fleet run --model a --json")
	if env.OK || env.Error.Code != "INVALID_ENUM" || env.Error.Field != "model" || env.Error.Received != "z" || !env.Retryable {
		t.Fatalf("bad error envelope: %+v", env)
	}
	if env.SuggestedFix == nil || env.SuggestedFix.Command == "" {
		t.Fatal("suggested_fix should be set when a fix command is given")
	}
	var out, errW bytes.Buffer
	fleetEmit{JSON: true}.failure(&out, &errW, env)
	var m map[string]any
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatalf("error stdout not a single JSON object: %v", err)
	}
	if m["ok"] != false || m["retryable"] != true {
		t.Fatalf("error envelope stdout shape wrong: %v", m)
	}
}

func TestFleetEmitPlainKeepsStdoutClean(t *testing.T) {
	var out, errW bytes.Buffer
	fleetEmit{Plain: true}.success(&out, &errW, fleetSuccessEnvelope{OK: true, Command: "fleet.run", Summary: "done"})
	if strings.TrimSpace(out.String()) == "" {
		t.Fatal("--plain success should still print something to stdout")
	}
	if json.Valid(out.Bytes()) {
		t.Fatal("--plain output should not be JSON")
	}
}
