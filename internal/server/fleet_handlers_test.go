package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"quorum/internal/core"
)

func setupFleetTestProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".ai", "tasks", "active"), 0755); err != nil {
		t.Fatalf("mkdir active: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".ai", "tasks", "done"), 0755); err != nil {
		t.Fatalf("mkdir done: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".ai", "tasks", "failed"), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	return root
}

func writeTaskTrace(t *testing.T, root, location, taskID, traceJSON string) {
	t.Helper()
	dir := filepath.Join(root, ".ai", "tasks", location, taskID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir task dir: %v", err)
	}
	specContent := "task_id: " + taskID + "\nsummary: \"fixture task\"\n"
	if err := os.WriteFile(filepath.Join(dir, "00-spec.yaml"), []byte(specContent), 0644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "07-trace.json"), []byte(traceJSON), 0644); err != nil {
		t.Fatalf("write trace: %v", err)
	}
}

// realFleet023ATrace mirrors the actual .ai/tasks/done/FLEET-023-a/07-trace.json
// shape: one routing_decision + one dispatch_started/dispatch_finished pair
// with outcome_class=attempt, plus one attempts[] entry, no tokens/cost.
const realFleet023ATrace = `{
  "attempts": [
    {"duration_s": 193.313877636, "model": "google/gemini-3.1-pro-low", "phase": "execute", "result": "passed"}
  ],
  "context_overflows": [],
  "events": [
    {"type": "risk_level_calculated", "level": "medium"},
    {"type": "routing_decision", "ts": "2026-07-20T21:27:46Z", "dispatch_id": "", "candidate": {"agent": "agy", "level": 1, "model": "google/gemini-3.1-pro-low"}},
    {"type": "dispatch_started", "ts": "2026-07-21T00:15:27Z", "dispatch_id": "41638ede1a3f", "bundle_hash": "41638ede1a3f791"},
    {"type": "dispatch_finished", "ts": "2026-07-21T00:18:40Z", "dispatch_id": "41638ede1a3f", "outcome_class": "attempt", "applied": true, "noop": false}
  ],
  "execution_mode": "worktree_edit",
  "started_at": "2026-07-20T21:24:58Z",
  "summary": "Fleet kill-switch core + CLI.",
  "task_id": "FLEET-023-a",
  "total_cost_usd": 0,
  "violations": []
}`

const rerouteOnlyTrace = `{
  "attempts": [],
  "context_overflows": [],
  "events": [
    {"type": "routing_decision", "ts": "2026-07-20T10:00:00Z", "dispatch_id": "", "candidate": {"agent": "agy", "level": 0, "model": "testvendor/test-model-x"}},
    {"type": "dispatch_started", "ts": "2026-07-20T10:01:00Z", "dispatch_id": "aaa111", "bundle_hash": "aaa"},
    {"type": "reroute", "ts": "2026-07-20T10:02:00Z", "dispatch_id": "aaa111", "cause": "quota"},
    {"type": "dispatch_finished", "ts": "2026-07-20T10:02:00Z", "dispatch_id": "aaa111", "outcome_class": "reroute", "applied": false, "noop": false}
  ],
  "execution_mode": "worktree_edit",
  "started_at": "2026-07-20T09:59:00Z",
  "summary": "Reroute-only fixture.",
  "task_id": "FLEET-900",
  "total_cost_usd": 0,
  "violations": []
}`

// blockedQuestionTerminalTrace mirrors appendDispatchTrace's real ordering
// for a blocked outcome (internal/core/fleet_dispatch.go): dispatch_started,
// blocked_question, dispatch_finished(outcome_class=blocked) -- so the
// dispatch itself is no longer in-flight, but the question remains
// unanswered (no later blocked_answer).
func blockedQuestionTerminalTrace(now time.Time) string {
	blockedTS := now.Add(-30 * time.Second).UTC().Format(time.RFC3339)
	return `{
  "attempts": [],
  "context_overflows": [],
  "events": [
    {"type": "dispatch_started", "ts": "2026-07-20T10:01:00Z", "dispatch_id": "bbb222", "bundle_hash": "bbb"},
    {"type": "blocked_question", "ts": "` + blockedTS + `", "dispatch_id": "bbb222", "path": "internal/core/foo.go", "reason": "missing helper", "severity": "critical"},
    {"type": "dispatch_finished", "ts": "` + blockedTS + `", "dispatch_id": "bbb222", "outcome_class": "blocked", "applied": false, "noop": false}
  ],
  "execution_mode": "worktree_edit",
  "started_at": "2026-07-20T09:59:00Z",
  "summary": "Blocked question terminal fixture.",
  "task_id": "FLEET-901",
  "total_cost_usd": 0,
  "violations": []
}`
}

const blockedThenAnsweredTrace = `{
  "attempts": [],
  "context_overflows": [],
  "events": [
    {"type": "dispatch_started", "ts": "2026-07-20T10:01:00Z", "dispatch_id": "ccc333", "bundle_hash": "ccc"},
    {"type": "blocked_question", "ts": "2026-07-20T10:02:00Z", "dispatch_id": "ccc333", "path": "x.go", "reason": "why", "severity": "minor"},
    {"type": "dispatch_finished", "ts": "2026-07-20T10:02:00Z", "dispatch_id": "ccc333", "outcome_class": "blocked", "applied": false, "noop": false},
    {"type": "blocked_answer", "ts": "2026-07-20T10:03:00Z", "dispatch_id": "ccc333"}
  ],
  "execution_mode": "worktree_edit",
  "started_at": "2026-07-20T09:59:00Z",
  "summary": "Blocked then answered fixture.",
  "task_id": "FLEET-902",
  "total_cost_usd": 0,
  "violations": []
}`

func dispatchStartedTerminalTrace(now time.Time) string {
	return `{
  "attempts": [],
  "context_overflows": [],
  "events": [
    {"type": "dispatch_started", "ts": "` + now.Add(-90*time.Second).UTC().Format(time.RFC3339) + `", "dispatch_id": "ddd444", "bundle_hash": "ddd"}
  ],
  "execution_mode": "worktree_edit",
  "started_at": "2026-07-20T09:59:00Z",
  "summary": "In-flight fixture.",
  "task_id": "FLEET-903",
  "total_cost_usd": 0,
  "violations": []
}`
}

const malformedTrace = `{not valid json`

// --- buildTaskDispatchView unit tests -------------------------------------

func loadTracePayload(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("unmarshal fixture trace: %v", err)
	}
	return m
}

func TestFleetBuildTaskDispatchView_RealFixtureAttempt(t *testing.T) {
	trace := loadTracePayload(t, realFleet023ATrace)
	dispatches, inFlight, blocked := buildTaskDispatchView("FLEET-023-a", trace, time.Now().UTC())

	if inFlight != nil {
		t.Fatalf("expected no in-flight record, got %+v", inFlight)
	}
	if blocked != nil {
		t.Fatalf("expected no blocked record, got %+v", blocked)
	}
	if len(dispatches) != 1 {
		t.Fatalf("expected 1 dispatch record, got %d: %+v", len(dispatches), dispatches)
	}
	d := dispatches[0]
	if d.TaskID != "FLEET-023-a" || d.Phase != "execute" || d.OutcomeClass != "attempt" {
		t.Fatalf("unexpected dispatch record: %+v", d)
	}
	if d.Model != "google/gemini-3.1-pro-low" {
		t.Fatalf("expected model from attempts[] to win, got %q", d.Model)
	}
	if d.Result != "passed" {
		t.Fatalf("expected result=passed, got %q", d.Result)
	}
	if d.DurationS == nil || *d.DurationS != 193.313877636 {
		t.Fatalf("expected duration_s from attempts[], got %v", d.DurationS)
	}
	if d.TokensIn != nil || d.TokensOut != nil || d.CostUSD != nil {
		t.Fatalf("expected omitted tokens/cost (usage.source=none), got in=%v out=%v cost=%v", d.TokensIn, d.TokensOut, d.CostUSD)
	}
}

func TestFleetBuildTaskDispatchView_RerouteOnly(t *testing.T) {
	trace := loadTracePayload(t, rerouteOnlyTrace)
	dispatches, inFlight, blocked := buildTaskDispatchView("FLEET-900", trace, time.Now().UTC())

	if inFlight != nil || blocked != nil {
		t.Fatalf("expected no in-flight/blocked, got inFlight=%+v blocked=%+v", inFlight, blocked)
	}
	if len(dispatches) != 1 {
		t.Fatalf("expected 1 dispatch record, got %d", len(dispatches))
	}
	d := dispatches[0]
	if d.OutcomeClass != "reroute" {
		t.Fatalf("expected outcome_class=reroute, got %q", d.OutcomeClass)
	}
	if d.Result != "" || d.DurationS != nil || d.TokensIn != nil {
		t.Fatalf("expected no fabricated attempt fields for reroute, got %+v", d)
	}
}

func TestFleetBuildTaskDispatchView_BlockedQuestionTerminal(t *testing.T) {
	now := time.Now().UTC()
	trace := loadTracePayload(t, blockedQuestionTerminalTrace(now))
	dispatches, inFlight, blocked := buildTaskDispatchView("FLEET-901", trace, now)

	if len(dispatches) != 1 || dispatches[0].OutcomeClass != "blocked" {
		t.Fatalf("expected 1 dispatch record with outcome_class=blocked, got %+v", dispatches)
	}
	if inFlight != nil {
		t.Fatalf("expected no in-flight record once the dispatch finished, got %+v", inFlight)
	}
	if blocked == nil {
		t.Fatal("expected a blocked record")
	}
	if blocked.Path != "internal/core/foo.go" || blocked.Severity != "critical" {
		t.Fatalf("unexpected blocked record: %+v", blocked)
	}
	if blocked.AgeSeconds < 20 || blocked.AgeSeconds > 60 {
		t.Fatalf("expected age_seconds around 30, got %v", blocked.AgeSeconds)
	}
}

func TestFleetBuildTaskDispatchView_BlockedThenAnswered(t *testing.T) {
	trace := loadTracePayload(t, blockedThenAnsweredTrace)
	dispatches, inFlight, blocked := buildTaskDispatchView("FLEET-902", trace, time.Now().UTC())

	if blocked != nil {
		t.Fatalf("expected blocked_answer to clear pending blocked, got %+v", blocked)
	}
	if inFlight != nil {
		t.Fatalf("expected no in-flight, got %+v", inFlight)
	}
	_ = dispatches
}

func TestFleetBuildTaskDispatchView_DispatchStartedTerminalIsInFlight(t *testing.T) {
	now := time.Now().UTC()
	trace := loadTracePayload(t, dispatchStartedTerminalTrace(now))
	dispatches, inFlight, blocked := buildTaskDispatchView("FLEET-903", trace, now)

	if len(dispatches) != 0 {
		t.Fatalf("expected no dispatch records for a still-open dispatch, got %+v", dispatches)
	}
	if blocked != nil {
		t.Fatalf("expected no blocked record, got %+v", blocked)
	}
	if inFlight == nil {
		t.Fatal("expected an in-flight record")
	}
	if inFlight.DispatchID != "ddd444" {
		t.Fatalf("unexpected in-flight dispatch id: %+v", inFlight)
	}
	if inFlight.AgeSeconds < 60 || inFlight.AgeSeconds > 150 {
		t.Fatalf("expected age_seconds around 90, got %v", inFlight.AgeSeconds)
	}
}

func TestFleetBuildTaskDispatchView_MalformedEventsDoesNotPanic(t *testing.T) {
	trace := map[string]any{
		"events":   []any{"not-an-object", 42, nil, map[string]any{"type": "dispatch_started", "dispatch_id": "x", "ts": "not-a-time"}},
		"attempts": []any{"also-not-an-object"},
	}
	dispatches, inFlight, blocked := buildTaskDispatchView("FLEET-904", trace, time.Now().UTC())
	if len(dispatches) != 0 {
		t.Fatalf("expected no dispatch records, got %+v", dispatches)
	}
	if blocked != nil {
		t.Fatalf("expected no blocked record, got %+v", blocked)
	}
	if inFlight == nil {
		t.Fatal("expected an in-flight record for the unmatched dispatch_started")
	}
}

// --- HTTP handler tests -----------------------------------------------------

func TestFleetPageHandler(t *testing.T) {
	srv := &Server{projectRoot: "/tmp/does-not-matter"}

	req := httptest.NewRequest(http.MethodGet, "/fleet", nil)
	w := httptest.NewRecorder()
	srv.fleetPageHandler(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %v", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("expected html content type, got %q", ct)
	}
	body := w.Body.String()
	if len(body) == 0 {
		t.Fatal("expected non-empty body")
	}
	if containsAny(body, []string{"htmx", "WebSocket", "new WebSocket"}) {
		t.Fatalf("fleet.html body must not reference htmx or WebSocket: %s", body)
	}
}

func TestFleetPageHandler_MethodNotAllowed(t *testing.T) {
	srv := &Server{projectRoot: "/tmp/does-not-matter"}
	req := httptest.NewRequest(http.MethodPost, "/fleet", nil)
	w := httptest.NewRecorder()
	srv.fleetPageHandler(w, req)
	if w.Result().StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %v", w.Result().StatusCode)
	}
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if len(sub) > 0 && (len(s) >= len(sub)) {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}

func TestFleetStatusHandler_ZeroValueWhenNoControlFile(t *testing.T) {
	root := setupFleetTestProject(t)
	srv := &Server{projectRoot: root}

	req := httptest.NewRequest(http.MethodGet, "/api/fleet/status", nil)
	w := httptest.NewRecorder()
	srv.fleetStatusHandler(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %v", res.StatusCode)
	}
	var report core.FleetStatusReport
	if err := json.NewDecoder(res.Body).Decode(&report); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(report.Disabled) != 0 {
		t.Fatalf("expected empty disabled list, got %+v", report.Disabled)
	}
	if report.UpdatedAt != "" {
		t.Fatalf("expected empty updated_at, got %q", report.UpdatedAt)
	}
}

func TestFleetStatusHandler_DisabledEntry(t *testing.T) {
	root := setupFleetTestProject(t)
	if err := os.MkdirAll(filepath.Join(root, ".agents", "fleet"), 0755); err != nil {
		t.Fatalf("mkdir agents/fleet: %v", err)
	}
	agentsYAML := "transports:\n  agy:\n    models:\n      testvendor/test-model-x:\n        provider: google\n"
	if err := os.WriteFile(filepath.Join(root, ".agents", "fleet", "agents.yaml"), []byte(agentsYAML), 0644); err != nil {
		t.Fatalf("write agents.yaml: %v", err)
	}
	if _, err := core.DisableFleetTarget(root, "agy/testvendor/test-model-x", "quota exhausted", "tester"); err != nil {
		t.Fatalf("disable target: %v", err)
	}

	srv := &Server{projectRoot: root}
	req := httptest.NewRequest(http.MethodGet, "/api/fleet/status", nil)
	w := httptest.NewRecorder()
	srv.fleetStatusHandler(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %v", res.StatusCode)
	}
	var report core.FleetStatusReport
	if err := json.NewDecoder(res.Body).Decode(&report); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(report.Disabled) != 1 {
		t.Fatalf("expected 1 disabled entry, got %+v", report.Disabled)
	}
	entry := report.Disabled[0]
	if entry.Target != "agy/testvendor/test-model-x" || entry.Reason != "quota exhausted" || entry.By != "tester" {
		t.Fatalf("unexpected entry: %+v", entry)
	}
	if entry.AgeSeconds <= 0 {
		t.Fatalf("expected age_seconds > 0, got %v", entry.AgeSeconds)
	}
}

func TestFleetStatusHandler_MethodNotAllowed(t *testing.T) {
	root := setupFleetTestProject(t)
	srv := &Server{projectRoot: root}
	req := httptest.NewRequest(http.MethodPost, "/api/fleet/status", nil)
	w := httptest.NewRecorder()
	srv.fleetStatusHandler(w, req)
	if w.Result().StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %v", w.Result().StatusCode)
	}
}

func TestFleetStatusHandler_503WhenNoProjectRoot(t *testing.T) {
	srv := &Server{projectRoot: ""}
	req := httptest.NewRequest(http.MethodGet, "/api/fleet/status", nil)
	w := httptest.NewRecorder()
	srv.fleetStatusHandler(w, req)
	if w.Result().StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %v", w.Result().StatusCode)
	}
}

func TestFleetDispatchesHandler_RealFixture(t *testing.T) {
	root := setupFleetTestProject(t)
	writeTaskTrace(t, root, "done", "FLEET-023-a", realFleet023ATrace)

	srv := &Server{projectRoot: root}
	req := httptest.NewRequest(http.MethodGet, "/api/fleet/dispatches", nil)
	w := httptest.NewRecorder()
	srv.fleetDispatchesHandler(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %v", res.StatusCode)
	}
	var payload fleetDispatchesResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(payload.Dispatches) != 1 {
		t.Fatalf("expected 1 dispatch, got %+v", payload.Dispatches)
	}
	if payload.Dispatches[0].TaskID != "FLEET-023-a" || payload.Dispatches[0].OutcomeClass != "attempt" {
		t.Fatalf("unexpected dispatch: %+v", payload.Dispatches[0])
	}
	if len(payload.InFlight) != 0 || len(payload.Blocked) != 0 {
		t.Fatalf("expected no in-flight/blocked, got %+v / %+v", payload.InFlight, payload.Blocked)
	}
}

func TestFleetDispatchesHandler_SkipsMalformedTraceWithoutFailingWholeRequest(t *testing.T) {
	root := setupFleetTestProject(t)
	writeTaskTrace(t, root, "done", "FLEET-023-a", realFleet023ATrace)
	writeTaskTrace(t, root, "failed", "FLEET-999", malformedTrace)

	srv := &Server{projectRoot: root}
	req := httptest.NewRequest(http.MethodGet, "/api/fleet/dispatches", nil)
	w := httptest.NewRecorder()
	srv.fleetDispatchesHandler(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 even with a malformed trace present, got %v", res.StatusCode)
	}
	var payload fleetDispatchesResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(payload.Dispatches) != 1 {
		t.Fatalf("expected the one valid dispatch to survive, got %+v", payload.Dispatches)
	}
}

func TestFleetDispatchesHandler_InFlightAndBlockedAggregation(t *testing.T) {
	root := setupFleetTestProject(t)
	now := time.Now().UTC()
	writeTaskTrace(t, root, "active", "FLEET-901", blockedQuestionTerminalTrace(now))
	writeTaskTrace(t, root, "active", "FLEET-903", dispatchStartedTerminalTrace(now))
	writeTaskTrace(t, root, "active", "FLEET-902", blockedThenAnsweredTrace)

	srv := &Server{projectRoot: root}
	req := httptest.NewRequest(http.MethodGet, "/api/fleet/dispatches", nil)
	w := httptest.NewRecorder()
	srv.fleetDispatchesHandler(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %v", res.StatusCode)
	}
	var payload fleetDispatchesResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(payload.InFlight) != 1 || payload.InFlight[0].TaskID != "FLEET-903" {
		t.Fatalf("expected exactly one in-flight (FLEET-903), got %+v", payload.InFlight)
	}
	if len(payload.Blocked) != 1 || payload.Blocked[0].TaskID != "FLEET-901" {
		t.Fatalf("expected exactly one blocked (FLEET-901), got %+v", payload.Blocked)
	}
}

func TestFleetDispatchesHandler_MethodNotAllowed(t *testing.T) {
	root := setupFleetTestProject(t)
	srv := &Server{projectRoot: root}
	req := httptest.NewRequest(http.MethodPost, "/api/fleet/dispatches", nil)
	w := httptest.NewRecorder()
	srv.fleetDispatchesHandler(w, req)
	if w.Result().StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %v", w.Result().StatusCode)
	}
}

func TestFleetDispatchesHandler_503WhenNoProjectRoot(t *testing.T) {
	srv := &Server{projectRoot: ""}
	req := httptest.NewRequest(http.MethodGet, "/api/fleet/dispatches", nil)
	w := httptest.NewRecorder()
	srv.fleetDispatchesHandler(w, req)
	if w.Result().StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %v", w.Result().StatusCode)
	}
}

// TestFleetJSHasPollingNoWebSocket guards AC-4: fleet.js must poll via
// setInterval and never reference WebSocket.
func TestFleetJSHasPollingNoWebSocket(t *testing.T) {
	root := repoRoot(t)
	b, err := os.ReadFile(filepath.Join(root, "internal", "server", "web", "fleet.js"))
	if err != nil {
		t.Fatalf("read fleet.js: %v", err)
	}
	content := string(b)
	if !containsAny(content, []string{"setInterval("}) {
		t.Fatal("fleet.js must contain a setInterval(...) polling call")
	}
	if containsAny(content, []string{"WebSocket"}) {
		t.Fatal("fleet.js must not reference WebSocket")
	}
}

// TestFleetHTMLNoHtmxNoExternalScript guards AC-5.
func TestFleetHTMLNoHtmxNoExternalScript(t *testing.T) {
	root := repoRoot(t)
	b, err := os.ReadFile(filepath.Join(root, "internal", "server", "web", "fleet.html"))
	if err != nil {
		t.Fatalf("read fleet.html: %v", err)
	}
	content := string(b)
	if containsAny(content, []string{"htmx"}) {
		t.Fatal("fleet.html must not reference htmx")
	}
	if containsAny(content, []string{`src="http://`, `src="https://`}) {
		t.Fatal("fleet.html must not load an external <script src=\"http...\"> tag")
	}
}

// TestServerStartDefaultHostUnchanged re-asserts AC-4's existing-behavior
// guard: Start() must keep defaulting host to 127.0.0.1 when empty. Actually
// binding a listener here is unnecessary and flaky in sandboxed CI, so this
// mirrors the drift-check pattern already used above (readAppJS) by asserting
// the default-host branch is still present in server.go's source.
func TestServerStartDefaultHostUnchanged(t *testing.T) {
	root := repoRoot(t)
	b, err := os.ReadFile(filepath.Join(root, "internal", "server", "server.go"))
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	if !containsAny(string(b), []string{`host = "127.0.0.1"`}) {
		t.Fatal("server.go must still default host to 127.0.0.1 when empty")
	}
}

func TestFleetJSSyntax(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "internal", "server", "web", "fleet.js")
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not available")
	}
	cmd := exec.Command("node", "--check", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fleet.js syntax check failed: %v\n%s", err, string(out))
	}
}
