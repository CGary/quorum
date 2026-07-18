package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"quorum/internal/core"
)

// fleetRouteRepoRoot resolves the repository root from this test file's own
// location, mirroring the "real file" precedent in cmd/fleet_dispatch_test.go.
func fleetRouteRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, raw, 0o644); err != nil {
		t.Fatalf("write %s: %v", dst, err)
	}
}

// setupTempPolicyRoot builds a temp project root holding byte-copies of the
// three REAL on-disk .agents policy files, and points schema resolution at the
// real schemas. buildRoutePolicyFromDisk then reads genuine policy content
// end-to-end without a live git repo.
func setupTempPolicyRoot(t *testing.T) string {
	t.Helper()
	repo := fleetRouteRepoRoot(t)
	root := t.TempDir()
	copyFile(t, filepath.Join(repo, ".agents", "config.yaml"), filepath.Join(root, ".agents", "config.yaml"))
	copyFile(t, filepath.Join(repo, ".agents", "policies", "routing.yaml"), filepath.Join(root, ".agents", "policies", "routing.yaml"))
	copyFile(t, filepath.Join(repo, ".agents", "fleet", "agents.yaml"), filepath.Join(root, ".agents", "fleet", "agents.yaml"))
	t.Setenv("QUORUM_SCHEMAS_DIR", filepath.Join(repo, ".agents", "schemas"))
	// Ensure no env override leaks the repo agents.yaml into buildRoutePolicyFromDisk.
	t.Setenv("QUORUM_FLEET_AGENTS", "")
	os.Unsetenv("QUORUM_FLEET_AGENTS")
	return root
}

func runRoute(t *testing.T, root, requestJSON string) (int, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	code := runFleetRoute(root, strings.NewReader(requestJSON), &out, &errBuf)
	return code, out.String(), errBuf.String()
}

func TestFleetRouteStdinDecodeErrors(t *testing.T) {
	root := setupTempPolicyRoot(t)
	for _, tc := range []struct{ name, in string }{
		{"empty", ""},
		{"malformed_json", "{not json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, _, errOut := runRoute(t, root, tc.in)
			if code == 0 {
				t.Fatalf("want non-zero exit for %s stdin", tc.name)
			}
			if errOut == "" {
				t.Error("want a stderr message, got none")
			}
		})
	}
}

// TestFleetRouteHashDeterminism is AC-2: a byte-level edit to routing.yaml
// changes only the RoutingYAML hash; ConfigYAML/AgentsYAML stay identical.
func TestFleetRouteHashDeterminism(t *testing.T) {
	root := setupTempPolicyRoot(t)
	reqJSON := `{"phase":"implement","risk":"low","complexity_band":"S"}`

	hashesOf := func(stdout string) core.PolicyHashes {
		var res core.RouteResult
		if err := json.Unmarshal([]byte(stdout), &res); err != nil {
			t.Fatalf("unmarshal result: %v (stdout=%s)", err, stdout)
		}
		return res.InputsSnapshot.Hashes
	}

	code1, out1, err1 := runRoute(t, root, reqJSON)
	if code1 != 0 {
		t.Fatalf("first run exit %d: %s", code1, err1)
	}
	before := hashesOf(out1)

	// Append one byte (a trailing comment) to routing.yaml.
	routingPath := filepath.Join(root, ".agents", "policies", "routing.yaml")
	raw, _ := os.ReadFile(routingPath)
	if err := os.WriteFile(routingPath, append(raw, []byte("\n# byte edit\n")...), 0o644); err != nil {
		t.Fatal(err)
	}

	code2, out2, err2 := runRoute(t, root, reqJSON)
	if code2 != 0 {
		t.Fatalf("second run exit %d: %s", code2, err2)
	}
	after := hashesOf(out2)

	if before.RoutingYAML == after.RoutingYAML {
		t.Error("RoutingYAML hash must change after a byte-level edit")
	}
	if before.ConfigYAML != after.ConfigYAML {
		t.Error("ConfigYAML hash must stay stable across the routing.yaml edit")
	}
	if before.AgentsYAML != after.AgentsYAML {
		t.Error("AgentsYAML hash must stay stable across the routing.yaml edit")
	}
}

func TestFleetRouteControlFileMissingAndMalformed(t *testing.T) {
	reqJSON := `{"phase":"implement","risk":"low","complexity_band":"S"}`

	t.Run("missing_is_graceful", func(t *testing.T) {
		root := setupTempPolicyRoot(t)
		code, out, errOut := runRoute(t, root, reqJSON)
		if code != 0 {
			t.Fatalf("missing control file must not error, exit %d: %s", code, errOut)
		}
		if !strings.Contains(out, "candidate") {
			t.Errorf("want a RouteResult on stdout, got %q", out)
		}
	})

	t.Run("malformed_fails_fast", func(t *testing.T) {
		root := setupTempPolicyRoot(t)
		ctrl := filepath.Join(root, ".ai", "fleet-control.json")
		if err := os.MkdirAll(filepath.Dir(ctrl), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(ctrl, []byte("{bad json"), 0o644); err != nil {
			t.Fatal(err)
		}
		code, _, errOut := runRoute(t, root, reqJSON)
		if code == 0 {
			t.Fatal("malformed control file must fail with non-zero exit")
		}
		if !strings.Contains(errOut, "fleet-control.json") {
			t.Errorf("want stderr to name the control file, got %q", errOut)
		}
	})
}

// seedActiveTask writes a minimal valid 07-trace.json for a synthetic active
// task and returns the task id. The dir name equals the id (exact-dir-name
// resolution, mirroring cmd/fleet_dispatch_test.go).
func seedActiveTask(t *testing.T, root, taskID string) {
	t.Helper()
	taskDir := filepath.Join(root, ".ai", "tasks", "active", taskID)
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	trace := `{"task_id":"` + taskID + `","summary":"fleet route fixture","started_at":"2026-07-12T00:00:00Z","attempts":[],"events":[],"total_cost_usd":0,"violations":[],"context_overflows":[]}`
	if err := os.WriteFile(filepath.Join(taskDir, "07-trace.json"), []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readTraceEvents(t *testing.T, path string) []any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal trace: %v", err)
	}
	evs, _ := payload["events"].([]any)
	return evs
}

// TestFleetRouteTraceAppend is AC-3/AC-7: with task_id, exactly one
// routing_decision event lands in 07-trace.json through the append-only path,
// carrying the full inputs_snapshot; without task_id the trace is untouched;
// re-running is not a double-append side effect of computing a decision.
func TestFleetRouteTraceAppend(t *testing.T) {
	root := setupTempPolicyRoot(t)
	taskID := "FLEET-900"
	seedActiveTask(t, root, taskID)
	tracePath := filepath.Join(root, ".ai", "tasks", "active", taskID, "07-trace.json")

	if got := len(readTraceEvents(t, tracePath)); got != 0 {
		t.Fatalf("fixture must start with zero events, got %d", got)
	}

	code, out, errOut := runRoute(t, root, `{"task_id":"`+taskID+`","phase":"implement","risk":"low","complexity_band":"S","incumbent_family":"google","dispatch_id":"disp-xyz"}`)
	if code != 0 {
		t.Fatalf("run exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, "candidate") {
		t.Errorf("want RouteResult on stdout, got %q", out)
	}

	events := readTraceEvents(t, tracePath)
	if len(events) != 1 {
		t.Fatalf("want exactly one appended event, got %d", len(events))
	}
	ev, ok := events[0].(map[string]any)
	if !ok {
		t.Fatalf("event is not an object: %T", events[0])
	}
	if ev["type"] != "routing_decision" {
		t.Errorf("event type: got %v want routing_decision", ev["type"])
	}
	if ev["dispatch_id"] != "disp-xyz" {
		t.Errorf("dispatch_id passthrough: got %v want disp-xyz", ev["dispatch_id"])
	}
	snap, ok := ev["inputs_snapshot"].(map[string]any)
	if !ok {
		t.Fatalf("inputs_snapshot missing/not object: %v", ev["inputs_snapshot"])
	}
	if snap["incumbent_family"] != "google" {
		t.Errorf("inputs_snapshot.incumbent_family: got %v want google", snap["incumbent_family"])
	}
	if snap["router_version"] == "" || snap["router_version"] == nil {
		t.Error("inputs_snapshot must carry a router_version")
	}

	// Re-invoke WITHOUT task_id: the trace must not change.
	code2, _, errOut2 := runRoute(t, root, `{"phase":"implement","risk":"low","complexity_band":"S"}`)
	if code2 != 0 {
		t.Fatalf("no-task run exit %d: %s", code2, errOut2)
	}
	if got := len(readTraceEvents(t, tracePath)); got != 1 {
		t.Fatalf("no-task run must not append; events went from 1 to %d", got)
	}
}

// TestFleetRouteRealPolicyFilesG1Cell is AC-5: against the REAL repo policy
// files, phase=implement/risk=low/band=S resolves to agy + the level-0 primary
// declared in config.yaml, and none of the G1 cell-set model-name literals
// (read from config, never embedded here) appears in any cmd/ or internal/ .go
// source.
func TestFleetRouteRealPolicyFilesG1Cell(t *testing.T) {
	repo := fleetRouteRepoRoot(t)
	primary, forbidden := g1CellModelNames(t, repo)

	code, out, errOut := runRoute(t, repo, `{"phase":"implement","risk":"low","complexity_band":"S"}`)
	if code != 0 {
		t.Fatalf("run against real files exit %d: %s", code, errOut)
	}
	var res core.RouteResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if res.Candidate == nil {
		t.Fatalf("want a candidate, got blocked=%q reasons=%v", res.Blocked, res.Reasons)
	}
	if res.Candidate.Agent != "agy" {
		t.Errorf("agent: got %q want agy", res.Candidate.Agent)
	}
	if res.Candidate.Model != primary {
		t.Errorf("model: got %q want level-0 primary %q", res.Candidate.Model, primary)
	}

	for _, dir := range []string{"cmd", "internal"} {
		root := filepath.Join(repo, dir)
		err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(p, ".go") {
				return err
			}
			src, rerr := os.ReadFile(p)
			if rerr != nil {
				return rerr
			}
			text := string(src)
			for _, name := range forbidden {
				if strings.Contains(text, name) {
					t.Errorf("%s must not hardcode G1 model name %q", p, name)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
}

// g1CellModelNames reads config.yaml.levels[0] and returns its primary model
// plus the full G1 cell-set model list (primary + secondaries), so tests never
// embed those literals (which the source scan forbids under cmd/ and internal/).
func g1CellModelNames(t *testing.T, repo string) (string, []string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repo, ".agents", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Levels map[int]core.LevelModels `yaml:"levels"`
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	l0 := cfg.Levels[0]
	names := append([]string{l0.Primary}, l0.Secondary...)
	return l0.Primary, names
}

// TestFleetRouteAgentsSchemaValidatesRealFile is AC-6: the real agents.yaml
// validates against the real agents.schema.json (exercised via the existing
// fleet-preflight schema path), and every model entry retains model_arg while
// gaining a provider drawn from the closed enum.
func TestFleetRouteAgentsSchemaValidatesRealFile(t *testing.T) {
	repo := fleetRouteRepoRoot(t)
	agentsRaw, err := os.ReadFile(filepath.Join(repo, ".agents", "fleet", "agents.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	schemaRaw, err := os.ReadFile(filepath.Join(repo, ".agents", "schemas", "agents.schema.json"))
	if err != nil {
		t.Fatal(err)
	}

	result, err := core.RunFleetPreflight(agentsRaw, schemaRaw, nil)
	if err != nil {
		t.Fatalf("RunFleetPreflight: %v", err)
	}
	for _, e := range result.Errors {
		if strings.Contains(e, "schema") {
			t.Fatalf("real agents.yaml failed schema validation: %s", e)
		}
	}

	validProviders := map[string]bool{
		"google": true, "openrouter-nvidia": true, "openrouter-poolside": true,
		"openrouter-cohere": true, "anthropic": true, "openai": true,
	}
	var parsed struct {
		Transports map[string]struct {
			Models map[string]map[string]any `yaml:"models"`
		} `yaml:"transports"`
	}
	if err := yaml.Unmarshal(agentsRaw, &parsed); err != nil {
		t.Fatal(err)
	}
	seen := 0
	for tname, tr := range parsed.Transports {
		for mk, mv := range tr.Models {
			seen++
			if s, _ := mv["model_arg"].(string); s == "" {
				t.Errorf("%s/%s lost its model_arg", tname, mk)
			}
			prov, _ := mv["provider"].(string)
			if !validProviders[prov] {
				t.Errorf("%s/%s provider %q not in the closed enum", tname, mk, prov)
			}
		}
	}
	if seen == 0 {
		t.Fatal("expected at least one model entry to spot-check")
	}
}
