//go:build sqlite_fts5 && sqlite_vec

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestFormatJSON(t *testing.T) {
	data := map[string]string{"foo": "bar"}
	got, err := FormatJSON(data)
	if err != nil {
		t.Fatalf("FormatJSON failed: %v", err)
	}

	expected := `{
  "foo": "bar"
}`
	if strings.TrimSpace(got) != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

func TestColorFunctions(t *testing.T) {
	noColorFlag = true
	defer func() { noColorFlag = false }()

	if Green("test") != "test" {
		t.Errorf("Green() with no color want %q, got %q", "test", Green("test"))
	}
	if Red("test") != "test" {
		t.Errorf("Red() with no color want %q, got %q", "test", Red("test"))
	}
	if Yellow("test") != "test" {
		t.Errorf("Yellow() with no color want %q, got %q", "test", Yellow("test"))
	}
}

func TestWriteSuccessEnvelope(t *testing.T) {
	var buf bytes.Buffer
	env := SuccessEnvelope{
		OK:      true,
		Command: "store",
		Summary: "Memory stored successfully. ID: 1",
		Data: map[string]any{
			"memory_id": int64(1),
			"status":    "stored",
		},
		NextActions: []NextAction{},
	}

	WriteSuccessEnvelope(&buf, env)
	output := buf.String()

	var raw map[string]any
	if err := json.Unmarshal([]byte(output), &raw); err != nil {
		t.Fatalf("failed to parse SuccessEnvelope JSON: %v, output: %s", err, output)
	}

	if raw["ok"] != true {
		t.Errorf("expected ok: true, got %v", raw["ok"])
	}
	if raw["command"] != "store" {
		t.Errorf("expected command 'store', got %v", raw["command"])
	}
	if raw["summary"] != "Memory stored successfully. ID: 1" {
		t.Errorf("got summary %v", raw["summary"])
	}
	if !strings.Contains(output, `"next_actions":[]`) {
		t.Errorf("expected next_actions to serialize as [], got %s", output)
	}
}

func TestWriteErrorEnvelope(t *testing.T) {
	var buf bytes.Buffer
	env := NewErrorEnvelope("explore", errInvalidEnum, "invalid direction 'sideways'", "direction", "sideways", false, "hsme-cli explore entity --direction both")

	WriteErrorEnvelope(&buf, env)
	output := buf.String()

	var raw map[string]any
	if err := json.Unmarshal([]byte(output), &raw); err != nil {
		t.Fatalf("failed to parse ErrorEnvelope JSON: %v, output: %s", err, output)
	}

	if raw["ok"] != false {
		t.Errorf("expected ok: false, got %v", raw["ok"])
	}
	if raw["command"] != "explore" {
		t.Errorf("expected command 'explore', got %v", raw["command"])
	}
	errBody, ok := raw["error"].(map[string]any)
	if !ok {
		t.Fatalf("missing error object in envelope: %v", raw)
	}
	if errBody["code"] != errInvalidEnum {
		t.Errorf("got code %v, want %v", errBody["code"], errInvalidEnum)
	}
	if errBody["field"] != "direction" {
		t.Errorf("got field %v, want 'direction'", errBody["field"])
	}
	if errBody["received"] != "sideways" {
		t.Errorf("got received %v, want 'sideways'", errBody["received"])
	}
	if raw["retryable"] != false {
		t.Errorf("expected retryable false, got %v", raw["retryable"])
	}
	fix, ok := raw["suggested_fix"].(map[string]any)
	if !ok || fix["command"] != "hsme-cli explore entity --direction both" {
		t.Errorf("got suggested_fix %v", raw["suggested_fix"])
	}
}

func TestWriteHumanError(t *testing.T) {
	var buf bytes.Buffer
	env := NewErrorEnvelope("store", errMissingRequired, "missing required flag: --source-type", "source-type", "", true, "hsme-cli store --source-type <type> < notes.md")

	WriteHumanError(&buf, env)
	out := buf.String()

	if !strings.Contains(out, "error: missing required flag: --source-type") {
		t.Errorf("expected error line, got: %s", out)
	}
	if !strings.Contains(out, "try: hsme-cli store --source-type <type> < notes.md") {
		t.Errorf("expected try line, got: %s", out)
	}
}

func TestSchemaFunctions(t *testing.T) {
	schemas := []struct {
		fn          func() map[string]any
		wantCommand string
	}{
		{storeSchema, "store"},
		{searchFuzzySchema, "search-fuzzy"},
		{searchExactSchema, "search-exact"},
		{exploreSchema, "explore"},
		{statusSchema, "status"},
		{adminRetryFailedSchema, "admin.retry-failed"},
		{adminBackupSchema, "admin.backup"},
		{adminRestoreSchema, "admin.restore"},
		{importQuorumSchema, "import-quorum"},
	}

	for _, s := range schemas {
		t.Run(s.wantCommand, func(t *testing.T) {
			m := s.fn()
			if m["command"] != s.wantCommand {
				t.Errorf("got command %q, want %q", m["command"], s.wantCommand)
			}
			if _, ok := m["description"].(string); !ok {
				t.Errorf("missing or non-string description in schema %s", s.wantCommand)
			}
			if _, ok := m["input"].(map[string]any); !ok {
				t.Errorf("missing input map in schema %s", s.wantCommand)
			}
			if _, ok := m["output"].(map[string]any); !ok {
				t.Errorf("missing output map in schema %s", s.wantCommand)
			}
			if _, ok := m["errors"].([]string); !ok {
				t.Errorf("missing errors slice in schema %s", s.wantCommand)
			}
		})
	}
}

func TestFieldsProjection(t *testing.T) {
	type item struct {
		ID    int    `json:"id"`
		Name  string `json:"name"`
		Extra string `json:"extra"`
	}

	items := []item{
		{ID: 1, Name: "alpha", Extra: "secret1"},
		{ID: 2, Name: "beta", Extra: "secret2"},
	}

	// 1. parseFields
	fields := parseFields("id, name")
	if len(fields) != 2 || fields[0] != "id" || fields[1] != "name" {
		t.Errorf("parseFields got %v", fields)
	}

	if parseFields("") != nil {
		t.Errorf("expected nil for empty fields string")
	}

	// 2. projectSlice
	projected := projectSlice(items, fields)
	if len(projected) != 2 {
		t.Fatalf("expected 2 projected items, got %d", len(projected))
	}

	for i, p := range projected {
		if _, ok := p["extra"]; ok {
			t.Errorf("item %d contains unprojected key 'extra'", i)
		}
		if _, ok := p["id"]; !ok {
			t.Errorf("item %d missing projected key 'id'", i)
		}
		if _, ok := p["name"]; !ok {
			t.Errorf("item %d missing projected key 'name'", i)
		}
	}
}

func TestExitCodeForError(t *testing.T) {
	usageCodes := []string{errMissingRequired, errInvalidEnum, errInvalidArgument, errValidationFailed}
	for _, c := range usageCodes {
		if code := exitCodeForError(c); code != exitUsage {
			t.Errorf("code %s: got exit code %d, want %d", c, code, exitUsage)
		}
	}

	runtimeCodes := []string{errFileNotFound, errPermissionDenied, errConflict, errTimeout, errNetworkError, errInternal}
	for _, c := range runtimeCodes {
		if code := exitCodeForError(c); code != exitRuntime {
			t.Errorf("code %s: got exit code %d, want %d", c, code, exitRuntime)
		}
	}
}
