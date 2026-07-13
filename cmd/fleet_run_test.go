package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupFleetRunProject builds a minimal project root with a fake agy-like
// transport and an executable fake binary. It creates NO git repo, NO task
// dir, and NO worktree: 'fleet run' is non-lifecycle, so its dependencies are
// only the transport map and the delegate binary.
func setupFleetRunProject(t *testing.T) (root, marker string) {
	t.Helper()
	root = t.TempDir()
	// A marker path the fake binary touches so a test can prove the delegate
	// (a) ran and (b) ran with cwd = --cwd.
	marker = "ran.txt"
	script := filepath.Join(root, "fake-agent.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'delegate ok\\n'\nprintf 'x' > "+marker+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	agentsDir := filepath.Join(root, ".agents", "fleet")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	agents := "transports:\n" +
		"  fake:\n" +
		"    binary: " + script + "\n" +
		"    argv_template:\n" +
		"      - --model\n" +
		"      - \"{model_arg}\"\n" +
		"      - \"{prompt}\"\n" +
		"    output_format: text\n" +
		"    timeouts:\n" +
		"      default_s: 30\n" +
		"    failure_signatures: []\n" +
		"    active: true\n" +
		"    models:\n" +
		"      anthropic/claude-sonnet-4-6:\n" +
		"        model_arg: Claude Sonnet 4.6 (Thinking)\n"
	if err := os.WriteFile(filepath.Join(agentsDir, "agents.yaml"), []byte(agents), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, marker
}

func writePromptFile(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "prompt.txt")
	if err := os.WriteFile(p, []byte("say hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func decodeEnvelope(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, b)
	}
	return m
}

func TestFleetRunHappyPathInCwd(t *testing.T) {
	root, marker := setupFleetRunProject(t)
	cwd := t.TempDir()
	var out, errW bytes.Buffer
	code := runFleetRun(fleetRunParams{
		Agent: "fake", Model: "anthropic/claude-sonnet-4-6", Cwd: cwd,
		Input: writePromptFile(t, root), JSON: true, ProjectRoot: root,
	}, &out, &errW)
	if code != 0 {
		t.Fatalf("want exit 0, got %d\nstdout=%s\nstderr=%s", code, out.String(), errW.String())
	}
	env := decodeEnvelope(t, out.Bytes())
	if env["ok"] != true || env["command"] != "fleet.run" {
		t.Fatalf("bad success envelope: %v", env)
	}
	if _, err := os.Stat(filepath.Join(cwd, marker)); err != nil {
		t.Fatalf("delegate did not run in --cwd: %v", err)
	}
	// Non-lifecycle: no lifecycle side effects in the project root.
	for _, p := range []string{".ai", "worktrees", ".git"} {
		if _, err := os.Stat(filepath.Join(root, p)); err == nil {
			t.Fatalf("fleet run created lifecycle artifact %s", p)
		}
	}
}

func TestFleetRunUnknownModelInvalidEnum(t *testing.T) {
	root, marker := setupFleetRunProject(t)
	cwd := t.TempDir()
	var out, errW bytes.Buffer
	code := runFleetRun(fleetRunParams{
		Agent: "fake", Model: "anthropic/nope", Cwd: cwd,
		Input: writePromptFile(t, root), JSON: true, ProjectRoot: root,
	}, &out, &errW)
	if code == 0 {
		t.Fatal("unknown model must exit non-zero")
	}
	env := decodeEnvelope(t, out.Bytes())
	e, _ := env["error"].(map[string]any)
	if env["ok"] != false || e["code"] != "INVALID_ENUM" || e["field"] != "model" || e["received"] != "anthropic/nope" {
		t.Fatalf("bad INVALID_ENUM envelope: %v", env)
	}
	if !strings.Contains(e["message"].(string), "anthropic/claude-sonnet-4-6") {
		t.Fatalf("message must list valid models: %v", e["message"])
	}
	if _, err := os.Stat(filepath.Join(cwd, marker)); err == nil {
		t.Fatal("no delegate must run on INVALID_ENUM")
	}
}

func TestFleetRunMissingRequiredFlag(t *testing.T) {
	root, _ := setupFleetRunProject(t)
	var out, errW bytes.Buffer
	code := runFleetRun(fleetRunParams{
		Agent: "fake", Cwd: t.TempDir(), Input: writePromptFile(t, root), JSON: true, ProjectRoot: root,
	}, &out, &errW)
	if code == 0 {
		t.Fatal("missing --model must exit non-zero")
	}
	e, _ := decodeEnvelope(t, out.Bytes())["error"].(map[string]any)
	if e["code"] != "MISSING_REQUIRED_FLAG" || e["field"] != "model" {
		t.Fatalf("bad MISSING_REQUIRED_FLAG envelope: %v", e)
	}
}

func TestFleetRunFileNotFound(t *testing.T) {
	root, _ := setupFleetRunProject(t)
	var out, errW bytes.Buffer
	code := runFleetRun(fleetRunParams{
		Agent: "fake", Model: "anthropic/claude-sonnet-4-6",
		Cwd: filepath.Join(root, "no-such-dir"), Input: writePromptFile(t, root), JSON: true, ProjectRoot: root,
	}, &out, &errW)
	if code == 0 {
		t.Fatal("missing --cwd path must exit non-zero")
	}
	e, _ := decodeEnvelope(t, out.Bytes())["error"].(map[string]any)
	if e["code"] != "FILE_NOT_FOUND" || e["field"] != "cwd" {
		t.Fatalf("bad FILE_NOT_FOUND envelope: %v", e)
	}
}

func TestFleetRunSchema(t *testing.T) {
	root, marker := setupFleetRunProject(t)
	cwd := t.TempDir()
	var out, errW bytes.Buffer
	code := runFleetRun(fleetRunParams{
		Agent: "fake", Schema: true, Cwd: cwd, ProjectRoot: root,
	}, &out, &errW)
	if code != 0 {
		t.Fatalf("--schema must exit 0, got %d", code)
	}
	if !strings.Contains(out.String(), "anthropic/claude-sonnet-4-6") || !strings.Contains(out.String(), "\"input\"") {
		t.Fatalf("--schema must print the model enum and input contract:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(cwd, marker)); err == nil {
		t.Fatal("--schema must not run a delegate")
	}
}

func TestFleetRunTimeout(t *testing.T) {
	root := t.TempDir()
	script := filepath.Join(root, "slow.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	agentsDir := filepath.Join(root, ".agents", "fleet")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	agents := "transports:\n  fake:\n    binary: " + script + "\n    argv_template: []\n    output_format: text\n    timeouts:\n      default_s: 30\n    failure_signatures: []\n    active: true\n    models:\n      anthropic/claude-sonnet-4-6:\n        model_arg: x\n"
	if err := os.WriteFile(filepath.Join(agentsDir, "agents.yaml"), []byte(agents), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errW bytes.Buffer
	code := runFleetRun(fleetRunParams{
		Agent: "fake", Model: "anthropic/claude-sonnet-4-6", Cwd: t.TempDir(),
		Input: writePromptFile(t, root), TimeoutS: 1, JSON: true, ProjectRoot: root,
	}, &out, &errW)
	if code == 0 {
		t.Fatal("timed-out delegate must exit non-zero")
	}
	e, _ := decodeEnvelope(t, out.Bytes())["error"].(map[string]any)
	if e["code"] != "TIMEOUT" {
		t.Fatalf("bad TIMEOUT envelope: %v", e)
	}
}

func TestFleetRunDryRun(t *testing.T) {
	root, marker := setupFleetRunProject(t)
	cwd := t.TempDir()
	var out, errW bytes.Buffer
	code := runFleetRun(fleetRunParams{
		Agent: "fake", Model: "anthropic/claude-sonnet-4-6", Cwd: cwd,
		Input: writePromptFile(t, root), DryRun: true, JSON: true, ProjectRoot: root,
	}, &out, &errW)
	if code != 0 {
		t.Fatalf("--dry-run must exit 0, got %d", code)
	}
	if _, err := os.Stat(filepath.Join(cwd, marker)); err == nil {
		t.Fatal("--dry-run must not run a delegate")
	}
}
