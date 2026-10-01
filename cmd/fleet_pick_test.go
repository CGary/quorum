package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// writePickPolicy writes a Jev router policy with two difficulty levels and
// three candidates (A, B, C) whose provider is openrouter_decisions pointing
// at the supplied URL and whose API key comes from TEST_JEV_KEY.
func writePickPolicy(t *testing.T, root, url string) string {
	t.Helper()
	policy := "version: 1\n" +
		"provider: openrouter_decisions\n" +
		"providers:\n" +
		"  openrouter_decisions:\n" +
		"    url: \"" + url + "\"\n" +
		"    model: jev-model\n" +
		"    api_key_env: TEST_JEV_KEY\n" +
		"timeout_s: 30\n" +
		"max_state_tokens: 100000\n" +
		"transport: opencode_go\n" +
		"instructions: pick the best model for the task\n" +
		"difficulty_instructions: pick the difficulty\n" +
		"candidates:\n" +
		"  - key: A\n" +
		"    model: opencode-go/model-a\n" +
		"    what: handles small fast tasks\n" +
		"    max_difficulty: hard\n" +
		"  - key: B\n" +
		"    model: opencode-go/model-b\n" +
		"    what: handles large reasoning tasks\n" +
		"    max_difficulty: hard\n" +
		"  - key: C\n" +
		"    model: opencode-go/model-c\n" +
		"    what: handles everything\n" +
		"    max_difficulty: hard\n" +
		"difficulty:\n" +
		"  - key: easy\n" +
		"    description: simple change\n" +
		"    claude_model: haiku\n" +
		"  - key: hard\n" +
		"    description: complex change\n" +
		"    claude_model: opus\n" +
		"default_difficulty: easy\n" +
		"fallback_pair: [A, B]\n"
	p := filepath.Join(root, "jev-router.yaml")
	if err := os.WriteFile(p, []byte(policy), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// writePickAgentsAll writes an agents.yaml whose opencode_go transport declares
// all three candidate models active (A, B and C), so the pick exercises the
// three-live-candidate degenerate-backup path.
func writePickAgentsAll(t *testing.T, root string) string {
	t.Helper()
	agents := "transports:\n" +
		"  opencode_go:\n" +
		"    active: true\n" +
		"    models:\n" +
		"      opencode-go/model-a: {}\n" +
		"      opencode-go/model-b: {}\n" +
		"      opencode-go/model-c: {}\n"
	p := filepath.Join(root, "agents.yaml")
	if err := os.WriteFile(p, []byte(agents), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// writePickAgents writes an agents.yaml whose opencode_go transport declares
// only two of the three candidate models active (A and B; C is absent).
func writePickAgents(t *testing.T, root string) string {
	t.Helper()
	agents := "transports:\n" +
		"  opencode_go:\n" +
		"    active: true\n" +
		"    models:\n" +
		"      opencode-go/model-a: {}\n" +
		"      opencode-go/model-b: {}\n"
	p := filepath.Join(root, "agents.yaml")
	if err := os.WriteFile(p, []byte(agents), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// writePickPolicyBackup writes a policy identical to writePickPolicy but with a
// paired backup (key B-bak, model backupModel) attached to candidate B.
func writePickPolicyBackup(t *testing.T, root, url, backupModel string) string {
	t.Helper()
	policy := "version: 1\n" +
		"provider: openrouter_decisions\n" +
		"providers:\n" +
		"  openrouter_decisions:\n" +
		"    url: \"" + url + "\"\n" +
		"    model: jev-model\n" +
		"    api_key_env: TEST_JEV_KEY\n" +
		"timeout_s: 30\n" +
		"max_state_tokens: 100000\n" +
		"transport: opencode_go\n" +
		"instructions: pick the best model for the task\n" +
		"difficulty_instructions: pick the difficulty\n" +
		"candidates:\n" +
		"  - key: A\n" +
		"    model: opencode-go/model-a\n" +
		"    what: handles small fast tasks\n" +
		"    max_difficulty: hard\n" +
		"  - key: B\n" +
		"    model: opencode-go/model-b\n" +
		"    what: handles large reasoning tasks\n" +
		"    max_difficulty: hard\n" +
		"    backup:\n" +
		"      key: B-bak\n" +
		"      model: " + backupModel + "\n" +
		"  - key: C\n" +
		"    model: opencode-go/model-c\n" +
		"    what: handles everything\n" +
		"    max_difficulty: hard\n" +
		"difficulty:\n" +
		"  - key: easy\n" +
		"    description: simple change\n" +
		"    claude_model: haiku\n" +
		"  - key: hard\n" +
		"    description: complex change\n" +
		"    claude_model: opus\n" +
		"default_difficulty: easy\n" +
		"fallback_pair: [A, B]\n"
	p := filepath.Join(root, "jev-router.yaml")
	if err := os.WriteFile(p, []byte(policy), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// writePickAgentsBackup writes an agents.yaml whose opencode_go transport
// declares all three candidate models plus the paired backup model
// (opencode-go/model-b2), so candidate B's paired backup stays live.
func writePickAgentsBackup(t *testing.T, root string) string {
	t.Helper()
	agents := "transports:\n" +
		"  opencode_go:\n" +
		"    active: true\n" +
		"    models:\n" +
		"      opencode-go/model-a: {}\n" +
		"      opencode-go/model-b: {}\n" +
		"      opencode-go/model-c: {}\n" +
		"      opencode-go/model-b2: {}\n"
	p := filepath.Join(root, "agents.yaml")
	if err := os.WriteFile(p, []byte(agents), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// pickTestGetenv returns an injected env reader mapping the policy/catalog/test
// key; every other variable (including QUORUM_JEV_URL) resolves empty.
func pickTestGetenv(policyPath, agentsPath, key string) func(string) string {
	return func(k string) string {
		switch k {
		case "QUORUM_JEV_POLICY":
			return policyPath
		case "QUORUM_FLEET_AGENTS":
			return agentsPath
		case "TEST_JEV_KEY":
			return key
		}
		return ""
	}
}

// writePickInput writes the --input JSON contract for a prompt.
func writePickInput(t *testing.T, dir, prompt string) string {
	t.Helper()
	b, err := json.Marshal(map[string]string{"prompt": prompt})
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "input.json")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// setupPickServerAndFiles creates a fake Jev server, the policy pointing at it,
// the agents catalog, and an input file, returning the input path and a getenv
// wired to all three.
func setupPickServerAndFiles(t *testing.T, handler http.HandlerFunc) (inputPath string, getenv func(string) string) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	root := t.TempDir()
	policyPath := writePickPolicy(t, root, srv.URL)
	agentsPath := writePickAgents(t, root)
	inputPath = writePickInput(t, root, "implement feature X")
	return inputPath, pickTestGetenv(policyPath, agentsPath, "secret")
}

// setupPickServerAndFilesAll is setupPickServerAndFiles but with all three
// candidates live in the catalog.
func setupPickServerAndFilesAll(t *testing.T, handler http.HandlerFunc) (inputPath string, getenv func(string) string) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	root := t.TempDir()
	policyPath := writePickPolicy(t, root, srv.URL)
	agentsPath := writePickAgentsAll(t, root)
	inputPath = writePickInput(t, root, "implement feature X")
	return inputPath, pickTestGetenv(policyPath, agentsPath, "secret")
}

// pickRequestCriteriaKeys decodes a Jev request body and returns the sorted
// target_model criteria keys, so a handler can answer differently per request.
func pickRequestCriteriaKeys(t *testing.T, r *http.Request) []string {
	t.Helper()
	var body struct {
		Questions struct {
			TargetModel struct {
				Criteria map[string]json.RawMessage `json:"criteria"`
			} `json:"target_model"`
		} `json:"questions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatalf("cannot decode request body: %v", err)
	}
	keys := make([]string, 0, len(body.Questions.TargetModel.Criteria))
	for k := range body.Questions.TargetModel.Criteria {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// writePickPolicyCeiling writes a Jev router policy with three ordered
// difficulty levels (trivial < moderate < hard) and three candidates (a ceiling
// moderate, b/c ceiling hard) so a hard answer excludes candidate a.
func writePickPolicyCeiling(t *testing.T, root, url string) string {
	t.Helper()
	policy := "version: 1\n" +
		"provider: openrouter_decisions\n" +
		"providers:\n" +
		"  openrouter_decisions:\n" +
		"    url: \"" + url + "\"\n" +
		"    model: jev-model\n" +
		"    api_key_env: TEST_JEV_KEY\n" +
		"timeout_s: 30\n" +
		"max_state_tokens: 100000\n" +
		"transport: opencode_go\n" +
		"instructions: pick the best model for the task\n" +
		"difficulty_instructions: pick the difficulty\n" +
		"candidates:\n" +
		"  - key: a\n" +
		"    model: opencode-go/model-a\n" +
		"    what: handles small fast tasks\n" +
		"    max_difficulty: moderate\n" +
		"  - key: b\n" +
		"    model: opencode-go/model-b\n" +
		"    what: handles large reasoning tasks\n" +
		"    max_difficulty: hard\n" +
		"  - key: c\n" +
		"    model: opencode-go/model-c\n" +
		"    what: handles everything\n" +
		"    max_difficulty: hard\n" +
		"difficulty:\n" +
		"  - key: trivial\n" +
		"    description: simple change\n" +
		"    claude_model: haiku\n" +
		"  - key: moderate\n" +
		"    description: medium change\n" +
		"    claude_model: sonnet\n" +
		"  - key: hard\n" +
		"    description: complex change\n" +
		"    claude_model: opus\n" +
		"default_difficulty: trivial\n" +
		"fallback_pair: [a, b]\n"
	p := filepath.Join(root, "jev-router.yaml")
	if err := os.WriteFile(p, []byte(policy), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFleetPickHappyPath(t *testing.T) {
	var sawAuth string
	inputPath, getenv := setupPickServerAndFiles(t, func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"model":"jev-model","answers":{"target_model":{"type":"choice","choice":"B","probabilities":{"A":0.3,"B":0.7}},"difficulty":{"type":"choice","choice":"hard"}}}`)
	})

	var out, errW bytes.Buffer
	code := runFleetPick(fleetPickParams{
		Input: inputPath, JSON: true, ProjectRoot: t.TempDir(), Getenv: getenv,
	}, &out, &errW)
	if code != 0 {
		t.Fatalf("want exit 0, got %d\nstdout=%s\nstderr=%s", code, out.String(), errW.String())
	}
	env := decodeEnvelope(t, out.Bytes())
	if env["ok"] != true || env["command"] != "fleet.pick" {
		t.Fatalf("bad success envelope: %v", env)
	}
	data := env["data"].(map[string]any)
	if data["reason"] != "jev" {
		t.Fatalf("want reason jev, got %v", data["reason"])
	}
	primary := data["primary"].(map[string]any)
	if primary["key"] != "B" {
		t.Fatalf("want primary B, got %v", primary)
	}
	backup := data["backup"].(map[string]any)
	if backup["key"] != "A" {
		t.Fatalf("want backup A, got %v", backup)
	}
	dropped := data["dropped"].([]any)
	if len(dropped) != 1 {
		t.Fatalf("want 1 dropped candidate, got %d", len(dropped))
	}
	d := dropped[0].(map[string]any)
	if d["key"] != "C" || d["reason"] != "not_in_catalog" {
		t.Fatalf("want dropped C not_in_catalog, got %v", d)
	}
	if sawAuth != "Bearer secret" {
		t.Fatalf("want Authorization header Bearer secret, got %q", sawAuth)
	}
}

func TestFleetPickTieChoiceWins(t *testing.T) {
	inputPath, getenv := setupPickServerAndFiles(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"model":"jev-model","answers":{"target_model":{"type":"choice","choice":"B","probabilities":{"A":0.5,"B":0.5}}}}`)
	})

	var out, errW bytes.Buffer
	code := runFleetPick(fleetPickParams{
		Input: inputPath, JSON: true, ProjectRoot: t.TempDir(), Getenv: getenv,
	}, &out, &errW)
	if code != 0 {
		t.Fatalf("want exit 0, got %d\nstdout=%s\nstderr=%s", code, out.String(), errW.String())
	}
	data := decodeEnvelope(t, out.Bytes())["data"].(map[string]any)
	primary := data["primary"].(map[string]any)
	if primary["key"] != "B" {
		t.Fatalf("want the classifier choice (B, second in policy order) to win the tie, got primary %v", primary)
	}
}

func TestFleetPickServerErrorFailsOpen(t *testing.T) {
	inputPath, getenv := setupPickServerAndFiles(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"boom"}}`, http.StatusInternalServerError)
	})

	var out, errW bytes.Buffer
	code := runFleetPick(fleetPickParams{
		Input: inputPath, JSON: true, ProjectRoot: t.TempDir(), Getenv: getenv,
	}, &out, &errW)
	if code != 0 {
		t.Fatalf("server 500 must fail open (exit 0), got %d\nstdout=%s\nstderr=%s", code, out.String(), errW.String())
	}
	env := decodeEnvelope(t, out.Bytes())
	if env["ok"] != true {
		t.Fatalf("want ok:true on fail-open, got %v", env)
	}
	data := env["data"].(map[string]any)
	if data["reason"] != "error" {
		t.Fatalf("want reason error, got %v", data["reason"])
	}
	primary := data["primary"].(map[string]any)
	backup := data["backup"].(map[string]any)
	if primary["key"] != "A" || backup["key"] != "B" {
		t.Fatalf("want fallback pair A/B, got primary=%v backup=%v", primary, backup)
	}
}

func TestFleetPickMissingKeyFailsOpen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("Jev server must not be reached when the API key env is missing")
	}))
	defer srv.Close()
	root := t.TempDir()
	policyPath := writePickPolicy(t, root, srv.URL)
	agentsPath := writePickAgents(t, root)
	inputPath := writePickInput(t, root, "implement feature X")

	var out, errW bytes.Buffer
	code := runFleetPick(fleetPickParams{
		Input: inputPath, JSON: true, ProjectRoot: root,
		Getenv: pickTestGetenv(policyPath, agentsPath, ""),
	}, &out, &errW)
	if code != 0 {
		t.Fatalf("missing key must fail open (exit 0), got %d\nstdout=%s\nstderr=%s", code, out.String(), errW.String())
	}
	env := decodeEnvelope(t, out.Bytes())
	if env["ok"] != true {
		t.Fatalf("want ok:true on fail-open, got %v", env)
	}
	data := env["data"].(map[string]any)
	if data["reason"] != "error" {
		t.Fatalf("want reason error, got %v", data["reason"])
	}
	errObj := data["error"].(map[string]any)
	msg, _ := errObj["message"].(string)
	if !strings.Contains(msg, "TEST_JEV_KEY") {
		t.Fatalf("want the error message to name TEST_JEV_KEY, got %q", msg)
	}
}

func TestFleetPickDryRunNoHTTP(t *testing.T) {
	calls := 0
	inputPath, getenv := setupPickServerAndFiles(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprint(w, `{"model":"jev-model","answers":{"target_model":{"type":"choice","choice":"A"}}}`)
	})

	var out, errW bytes.Buffer
	code := runFleetPick(fleetPickParams{
		Input: inputPath, DryRun: true, JSON: true, ProjectRoot: t.TempDir(), Getenv: getenv,
	}, &out, &errW)
	if code != 0 {
		t.Fatalf("--dry-run must exit 0, got %d\nstdout=%s\nstderr=%s", code, out.String(), errW.String())
	}
	if calls != 0 {
		t.Fatalf("--dry-run must make no HTTP call, got %d", calls)
	}
	data := decodeEnvelope(t, out.Bytes())["data"].(map[string]any)
	req := data["request"].(map[string]any)
	questions := req["questions"].(map[string]any)
	if _, ok := questions["target_model"]; !ok {
		t.Fatalf("want request.questions.target_model, got %v", questions)
	}
	if _, ok := questions["difficulty"]; !ok {
		t.Fatalf("want request.questions.difficulty, got %v", questions)
	}
}

func TestFleetPickMissingInput(t *testing.T) {
	root := t.TempDir()
	var out, errW bytes.Buffer
	code := runFleetPick(fleetPickParams{JSON: true, ProjectRoot: root}, &out, &errW)
	if code == 0 {
		t.Fatal("missing --input must exit non-zero")
	}
	e := decodeEnvelope(t, out.Bytes())["error"].(map[string]any)
	if e["code"] != "MISSING_REQUIRED_FLAG" || e["field"] != "input" {
		t.Fatalf("bad MISSING_REQUIRED_FLAG envelope: %v", e)
	}
}

func TestFleetPickEmptyPrompt(t *testing.T) {
	root := t.TempDir()
	inputPath := writePickInput(t, root, "")
	var out, errW bytes.Buffer
	code := runFleetPick(fleetPickParams{Input: inputPath, JSON: true, ProjectRoot: root}, &out, &errW)
	if code == 0 {
		t.Fatal("empty prompt must exit non-zero")
	}
	e := decodeEnvelope(t, out.Bytes())["error"].(map[string]any)
	if e["code"] != "INVALID_ARGUMENT" || e["field"] != "prompt" {
		t.Fatalf("bad INVALID_ARGUMENT envelope: %v", e)
	}
}

func TestFleetPickBadProvider(t *testing.T) {
	root := t.TempDir()
	policyPath := writePickPolicy(t, root, "http://unused")
	agentsPath := writePickAgents(t, root)
	inputPath := writePickInput(t, root, "implement feature X")
	var out, errW bytes.Buffer
	code := runFleetPick(fleetPickParams{
		Input: inputPath, Provider: "bogus", JSON: true, ProjectRoot: root,
		Getenv: pickTestGetenv(policyPath, agentsPath, "secret"),
	}, &out, &errW)
	if code == 0 {
		t.Fatal("bad --provider must exit non-zero")
	}
	e := decodeEnvelope(t, out.Bytes())["error"].(map[string]any)
	if e["code"] != "INVALID_ENUM" || e["field"] != "provider" {
		t.Fatalf("bad INVALID_ENUM envelope: %v", e)
	}
}

func TestFleetPickOneHotTriggersSecondCall(t *testing.T) {
	var requests int
	var secondCriteria []string
	inputPath, getenv := setupPickServerAndFilesAll(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		keys := pickRequestCriteriaKeys(t, r)
		w.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			fmt.Fprint(w, `{"model":"jev-model","answers":{"target_model":{"type":"choice","choice":"B","probabilities":{"A":0.0,"B":1.0,"C":0.0}}}}`)
			return
		}
		secondCriteria = keys
		fmt.Fprint(w, `{"model":"jev-model","answers":{"target_model":{"type":"choice","choice":"C","probabilities":{"A":0.0,"C":1.0}}}}`)
	})

	var out, errW bytes.Buffer
	code := runFleetPick(fleetPickParams{
		Input: inputPath, JSON: true, ProjectRoot: t.TempDir(), Getenv: getenv,
	}, &out, &errW)
	if code != 0 {
		t.Fatalf("want exit 0, got %d\nstdout=%s\nstderr=%s", code, out.String(), errW.String())
	}
	if requests != 2 {
		t.Fatalf("want exactly 2 requests, got %d", requests)
	}
	if len(secondCriteria) != 2 {
		t.Fatalf("want 2 candidates in second request, got %v", secondCriteria)
	}
	for _, k := range secondCriteria {
		if k == "B" {
			t.Fatalf("second request must not offer primary B, criteria=%v", secondCriteria)
		}
	}
	data := decodeEnvelope(t, out.Bytes())["data"].(map[string]any)
	if data["backup_source"] != "second_call" {
		t.Fatalf("want backup_source second_call, got %v", data["backup_source"])
	}
	backup := data["backup"].(map[string]any)
	if backup["key"] != "C" {
		t.Fatalf("want backup C from second call, got %v", backup)
	}
	if _, ok := data["second_call"]; !ok {
		t.Fatalf("want second_call detail present, got %v", data)
	}
}

func TestFleetPickDistinctProbabilitiesSingleCall(t *testing.T) {
	var requests int
	inputPath, getenv := setupPickServerAndFilesAll(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"model":"jev-model","answers":{"target_model":{"type":"choice","choice":"A","probabilities":{"A":0.6,"B":0.3,"C":0.1}}}}`)
	})

	var out, errW bytes.Buffer
	code := runFleetPick(fleetPickParams{
		Input: inputPath, JSON: true, ProjectRoot: t.TempDir(), Getenv: getenv,
	}, &out, &errW)
	if code != 0 {
		t.Fatalf("want exit 0, got %d\nstdout=%s\nstderr=%s", code, out.String(), errW.String())
	}
	if requests != 1 {
		t.Fatalf("want exactly 1 request, got %d", requests)
	}
	data := decodeEnvelope(t, out.Bytes())["data"].(map[string]any)
	if data["backup_source"] != "rank2" {
		t.Fatalf("want backup_source rank2, got %v", data["backup_source"])
	}
	primary := data["primary"].(map[string]any)
	if primary["key"] != "A" {
		t.Fatalf("want primary A, got %v", primary)
	}
	backup := data["backup"].(map[string]any)
	if backup["key"] != "B" {
		t.Fatalf("want backup B, got %v", backup)
	}
}

func TestFleetPickPairedBackupSingleCall(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"model":"jev-model","answers":{"target_model":{"type":"choice","choice":"B","probabilities":{"A":0.0,"B":1.0,"C":0.0}}}}`)
	}))
	defer srv.Close()
	root := t.TempDir()
	policyPath := writePickPolicyBackup(t, root, srv.URL, "opencode-go/model-b2")
	agentsPath := writePickAgentsBackup(t, root)
	inputPath := writePickInput(t, root, "implement feature X")

	var out, errW bytes.Buffer
	code := runFleetPick(fleetPickParams{
		Input: inputPath, JSON: true, ProjectRoot: root,
		Getenv: pickTestGetenv(policyPath, agentsPath, "secret"),
	}, &out, &errW)
	if code != 0 {
		t.Fatalf("want exit 0, got %d\nstdout=%s\nstderr=%s", code, out.String(), errW.String())
	}
	if requests != 1 {
		t.Fatalf("want exactly 1 request (paired backup never re-calls), got %d", requests)
	}
	data := decodeEnvelope(t, out.Bytes())["data"].(map[string]any)
	if data["backup_source"] != "paired" {
		t.Fatalf("want backup_source paired, got %v", data["backup_source"])
	}
	backup := data["backup"].(map[string]any)
	if backup["key"] != "B-bak" {
		t.Fatalf("want paired backup key B-bak, got %v", backup)
	}
	if _, ok := data["second_call"]; ok {
		t.Fatalf("paired backup must not produce second_call, got %v", data)
	}
}

func TestFleetPickDeadBackupTriggersSecondCall(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			fmt.Fprint(w, `{"model":"jev-model","answers":{"target_model":{"type":"choice","choice":"B","probabilities":{"A":0.0,"B":1.0,"C":0.0}}}}`)
			return
		}
		fmt.Fprint(w, `{"model":"jev-model","answers":{"target_model":{"type":"choice","choice":"C","probabilities":{"A":0.0,"C":1.0}}}}`)
	}))
	defer srv.Close()
	root := t.TempDir()
	policyPath := writePickPolicyBackup(t, root, srv.URL, "opencode-go/model-b2")
	// Only A/B/C in the catalog: candidate B's paired backup model is absent, so
	// it is dropped and the decision degrades to the normal rank-2 degenerate
	// path, which triggers the second call.
	agentsPath := writePickAgentsAll(t, root)
	inputPath := writePickInput(t, root, "implement feature X")

	var out, errW bytes.Buffer
	code := runFleetPick(fleetPickParams{
		Input: inputPath, JSON: true, ProjectRoot: root,
		Getenv: pickTestGetenv(policyPath, agentsPath, "secret"),
	}, &out, &errW)
	if code != 0 {
		t.Fatalf("want exit 0, got %d\nstdout=%s\nstderr=%s", code, out.String(), errW.String())
	}
	if requests != 2 {
		t.Fatalf("want exactly 2 requests, got %d", requests)
	}
	data := decodeEnvelope(t, out.Bytes())["data"].(map[string]any)
	if data["backup_source"] != "second_call" {
		t.Fatalf("want backup_source second_call, got %v", data["backup_source"])
	}
	backup := data["backup"].(map[string]any)
	if backup["key"] != "C" {
		t.Fatalf("want backup C from second call, got %v", backup)
	}
}

func TestFleetPickOneHotSecondCall500FallsBackToTiebreak(t *testing.T) {
	var requests int
	inputPath, getenv := setupPickServerAndFilesAll(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"model":"jev-model","answers":{"target_model":{"type":"choice","choice":"B","probabilities":{"A":0.0,"B":1.0,"C":0.0}}}}`)
			return
		}
		http.Error(w, `{"error":{"message":"boom"}}`, http.StatusInternalServerError)
	})

	var out, errW bytes.Buffer
	code := runFleetPick(fleetPickParams{
		Input: inputPath, JSON: true, ProjectRoot: t.TempDir(), Getenv: getenv,
	}, &out, &errW)
	if code != 0 {
		t.Fatalf("want exit 0, got %d\nstdout=%s\nstderr=%s", code, out.String(), errW.String())
	}
	env := decodeEnvelope(t, out.Bytes())
	if env["ok"] != true {
		t.Fatalf("want ok:true, got %v", env)
	}
	data := env["data"].(map[string]any)
	if data["backup_source"] != "rank2_tiebreak" {
		t.Fatalf("want backup_source rank2_tiebreak, got %v", data["backup_source"])
	}
	backup := data["backup"].(map[string]any)
	if backup["key"] != "A" {
		t.Fatalf("want original backup A from rank-2 tie-break, got %v", backup)
	}
	if _, ok := data["second_call"]; !ok {
		t.Fatalf("want second_call detail present even on error, got %v", data)
	}
}

func TestFleetPickCeilingApplied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"model":"jev-model","answers":{"target_model":{"type":"choice","choice":"a","probabilities":{"a":0.6,"b":0.3,"c":0.1}},"difficulty":{"type":"choice","choice":"hard"}}}`)
	}))
	defer srv.Close()
	root := t.TempDir()
	policyPath := writePickPolicyCeiling(t, root, srv.URL)
	agentsPath := writePickAgentsAll(t, root)
	inputPath := writePickInput(t, root, "implement feature X")

	var out, errW bytes.Buffer
	code := runFleetPick(fleetPickParams{
		Input: inputPath, JSON: true, ProjectRoot: root,
		Getenv: pickTestGetenv(policyPath, agentsPath, "secret"),
	}, &out, &errW)
	if code != 0 {
		t.Fatalf("want exit 0, got %d\nstdout=%s\nstderr=%s", code, out.String(), errW.String())
	}
	data := decodeEnvelope(t, out.Bytes())["data"].(map[string]any)
	primary := data["primary"].(map[string]any)
	if primary["key"] != "b" {
		t.Fatalf("want primary b (ceiling excluded a), got %v", primary)
	}
	if data["ceiling_applied"] != true {
		t.Fatalf("want ceiling_applied true, got %v", data["ceiling_applied"])
	}
}

// writePickPolicyFallback writes a policy whose primary provider
// (openrouter_decisions) points at primaryURL and whose fallback provider
// (typesafe) points at fallbackURL, with distinct API key env vars so a test can
// leave one empty.
func writePickPolicyFallback(t *testing.T, root, primaryURL, fallbackURL string) string {
	t.Helper()
	policy := "version: 1\n" +
		"provider: openrouter_decisions\n" +
		"fallback_provider: typesafe\n" +
		"providers:\n" +
		"  openrouter_decisions:\n" +
		"    url: \"" + primaryURL + "\"\n" +
		"    model: jev-model\n" +
		"    api_key_env: TEST_JEV_KEY\n" +
		"  typesafe:\n" +
		"    url: \"" + fallbackURL + "\"\n" +
		"    model: ts-model\n" +
		"    api_key_env: TEST_JEV_KEY2\n" +
		"timeout_s: 30\n" +
		"max_state_tokens: 100000\n" +
		"transport: opencode_go\n" +
		"instructions: pick the best model for the task\n" +
		"difficulty_instructions: pick the difficulty\n" +
		"candidates:\n" +
		"  - key: A\n" +
		"    model: opencode-go/model-a\n" +
		"    what: handles small fast tasks\n" +
		"    max_difficulty: hard\n" +
		"  - key: B\n" +
		"    model: opencode-go/model-b\n" +
		"    what: handles large reasoning tasks\n" +
		"    max_difficulty: hard\n" +
		"  - key: C\n" +
		"    model: opencode-go/model-c\n" +
		"    what: handles everything\n" +
		"    max_difficulty: hard\n" +
		"difficulty:\n" +
		"  - key: easy\n" +
		"    description: simple change\n" +
		"    claude_model: haiku\n" +
		"  - key: hard\n" +
		"    description: complex change\n" +
		"    claude_model: opus\n" +
		"default_difficulty: easy\n" +
		"fallback_pair: [A, B]\n"
	p := filepath.Join(root, "jev-router.yaml")
	if err := os.WriteFile(p, []byte(policy), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// pickTestGetenvTwo wires the policy/catalog paths plus two independent Jev API
// key env vars (TEST_JEV_KEY for the primary, TEST_JEV_KEY2 for the fallback).
func pickTestGetenvTwo(policyPath, agentsPath, primaryKey, fallbackKey string) func(string) string {
	return func(k string) string {
		switch k {
		case "QUORUM_JEV_POLICY":
			return policyPath
		case "QUORUM_FLEET_AGENTS":
			return agentsPath
		case "TEST_JEV_KEY":
			return primaryKey
		case "TEST_JEV_KEY2":
			return fallbackKey
		}
		return ""
	}
}

func TestFleetPickFallbackProviderUsed(t *testing.T) {
	primarySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"boom"}}`, http.StatusInternalServerError)
	}))
	t.Cleanup(primarySrv.Close)
	fallbackSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"model":"ts-model","answers":{"target_model":{"type":"choice","choice":"B","probabilities":{"A":0.3,"B":0.7}},"difficulty":{"type":"choice","choice":"easy"}}}`)
	}))
	t.Cleanup(fallbackSrv.Close)

	root := t.TempDir()
	policyPath := writePickPolicyFallback(t, root, primarySrv.URL, fallbackSrv.URL)
	agentsPath := writePickAgents(t, root)
	inputPath := writePickInput(t, root, "implement feature X")

	var out, errW bytes.Buffer
	code := runFleetPick(fleetPickParams{
		Input: inputPath, JSON: true, ProjectRoot: root,
		Getenv: pickTestGetenvTwo(policyPath, agentsPath, "secret", "secret2"),
	}, &out, &errW)
	if code != 0 {
		t.Fatalf("want exit 0, got %d\nstdout=%s\nstderr=%s", code, out.String(), errW.String())
	}
	data := decodeEnvelope(t, out.Bytes())["data"].(map[string]any)
	if data["reason"] != "jev" {
		t.Fatalf("want reason jev, got %v", data["reason"])
	}
	if data["provider"] != "typesafe" {
		t.Fatalf("want data.provider typesafe, got %v", data["provider"])
	}
	attempts := data["provider_attempts"].([]any)
	if len(attempts) != 1 {
		t.Fatalf("want 1 provider attempt, got %d", len(attempts))
	}
	a := attempts[0].(map[string]any)
	if a["provider"] != "openrouter_decisions" || a["status"] != float64(500) {
		t.Fatalf("want primary attempt openrouter_decisions/500, got %v", a)
	}
}

func TestFleetPickFallbackNotUsedOn422(t *testing.T) {
	var fallbackCalls int
	primarySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"bad request"}}`, http.StatusUnprocessableEntity)
	}))
	t.Cleanup(primarySrv.Close)
	fallbackSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackCalls++
		fmt.Fprint(w, `{}`)
	}))
	t.Cleanup(fallbackSrv.Close)

	root := t.TempDir()
	policyPath := writePickPolicyFallback(t, root, primarySrv.URL, fallbackSrv.URL)
	agentsPath := writePickAgents(t, root)
	inputPath := writePickInput(t, root, "implement feature X")

	var out, errW bytes.Buffer
	code := runFleetPick(fleetPickParams{
		Input: inputPath, JSON: true, ProjectRoot: root,
		Getenv: pickTestGetenvTwo(policyPath, agentsPath, "secret", "secret2"),
	}, &out, &errW)
	if code != 0 {
		t.Fatalf("422 must fail open (exit 0), got %d\nstdout=%s\nstderr=%s", code, out.String(), errW.String())
	}
	if fallbackCalls != 0 {
		t.Fatalf("fallback must receive zero requests on a 422, got %d", fallbackCalls)
	}
	data := decodeEnvelope(t, out.Bytes())["data"].(map[string]any)
	if data["reason"] != "error" {
		t.Fatalf("want reason error, got %v", data["reason"])
	}
}

func TestFleetPickFallbackOnMissingPrimaryKey(t *testing.T) {
	primarySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("primary must not be reached when its key env is missing")
	}))
	t.Cleanup(primarySrv.Close)
	fallbackSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"model":"ts-model","answers":{"target_model":{"type":"choice","choice":"A"}}}`)
	}))
	t.Cleanup(fallbackSrv.Close)

	root := t.TempDir()
	policyPath := writePickPolicyFallback(t, root, primarySrv.URL, fallbackSrv.URL)
	agentsPath := writePickAgents(t, root)
	inputPath := writePickInput(t, root, "implement feature X")

	var out, errW bytes.Buffer
	code := runFleetPick(fleetPickParams{
		Input: inputPath, JSON: true, ProjectRoot: root,
		Getenv: pickTestGetenvTwo(policyPath, agentsPath, "", "secret2"),
	}, &out, &errW)
	if code != 0 {
		t.Fatalf("want exit 0, got %d\nstdout=%s\nstderr=%s", code, out.String(), errW.String())
	}
	data := decodeEnvelope(t, out.Bytes())["data"].(map[string]any)
	if data["provider"] != "typesafe" {
		t.Fatalf("want data.provider typesafe, got %v", data["provider"])
	}
	attempts := data["provider_attempts"].([]any)
	if len(attempts) != 1 {
		t.Fatalf("want 1 provider attempt, got %d", len(attempts))
	}
	msg, _ := attempts[0].(map[string]any)["message"].(string)
	if !strings.Contains(msg, "TEST_JEV_KEY") {
		t.Fatalf("want the attempt message to name TEST_JEV_KEY, got %q", msg)
	}
}

func TestFleetPickFallbackBothFail(t *testing.T) {
	primarySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"boom"}}`, http.StatusInternalServerError)
	}))
	t.Cleanup(primarySrv.Close)
	fallbackSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"boom"}}`, http.StatusBadGateway)
	}))
	t.Cleanup(fallbackSrv.Close)

	root := t.TempDir()
	policyPath := writePickPolicyFallback(t, root, primarySrv.URL, fallbackSrv.URL)
	agentsPath := writePickAgents(t, root)
	inputPath := writePickInput(t, root, "implement feature X")

	var out, errW bytes.Buffer
	code := runFleetPick(fleetPickParams{
		Input: inputPath, JSON: true, ProjectRoot: root,
		Getenv: pickTestGetenvTwo(policyPath, agentsPath, "secret", "secret2"),
	}, &out, &errW)
	if code != 0 {
		t.Fatalf("want exit 0, got %d\nstdout=%s\nstderr=%s", code, out.String(), errW.String())
	}
	data := decodeEnvelope(t, out.Bytes())["data"].(map[string]any)
	if data["reason"] != "error" {
		t.Fatalf("want reason error, got %v", data["reason"])
	}
	attempts := data["provider_attempts"].([]any)
	if len(attempts) != 2 {
		t.Fatalf("want 2 provider attempts, got %d", len(attempts))
	}
}

func TestFleetPickNoFallbackProviderAttemptsEmpty(t *testing.T) {
	inputPath, getenv := setupPickServerAndFiles(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"model":"jev-model","answers":{"target_model":{"type":"choice","choice":"A"}}}`)
	})

	var out, errW bytes.Buffer
	code := runFleetPick(fleetPickParams{
		Input: inputPath, JSON: true, ProjectRoot: t.TempDir(), Getenv: getenv,
	}, &out, &errW)
	if code != 0 {
		t.Fatalf("want exit 0, got %d\nstdout=%s\nstderr=%s", code, out.String(), errW.String())
	}
	data := decodeEnvelope(t, out.Bytes())["data"].(map[string]any)
	if data["reason"] != "jev" {
		t.Fatalf("want reason jev, got %v", data["reason"])
	}
	if _, ok := data["provider_attempts"]; ok {
		t.Fatalf("provider_attempts must be empty when no fallback is configured, got %v", data["provider_attempts"])
	}
}

func TestFleetPickUpstreamProviderEchoed(t *testing.T) {
	inputPath, getenv := setupPickServerAndFiles(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"model":"jev-model","provider":"TypeSafe","answers":{"target_model":{"type":"choice","choice":"A"}}}`)
	})

	var out, errW bytes.Buffer
	code := runFleetPick(fleetPickParams{
		Input: inputPath, JSON: true, ProjectRoot: t.TempDir(), Getenv: getenv,
	}, &out, &errW)
	if code != 0 {
		t.Fatalf("want exit 0, got %d\nstdout=%s\nstderr=%s", code, out.String(), errW.String())
	}
	data := decodeEnvelope(t, out.Bytes())["data"].(map[string]any)
	if data["provider"] != "openrouter_decisions" {
		t.Fatalf("want data.provider openrouter_decisions, got %v", data["provider"])
	}
	if data["upstream_provider"] != "TypeSafe" {
		t.Fatalf("want upstream_provider TypeSafe, got %v", data["upstream_provider"])
	}
}
