package server

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"quorum/internal/core"
)

// fleetTemplate is parsed once from the embedded web/fleet.html asset, reusing
// the existing webFS embed.FS declared in embed.go (//go:embed web/* already
// recursively covers this file; no edit to embed.go is needed).
var (
	fleetTemplateOnce sync.Once
	fleetTemplate     *template.Template
)

func loadFleetTemplate() *template.Template {
	fleetTemplateOnce.Do(func() {
		fleetTemplate = template.Must(template.ParseFS(webFS, "web/fleet.html"))
	})
	return fleetTemplate
}

// fleetPageData is the substitution data passed to fleet.html.
type fleetPageData struct {
	PollIntervalMS int
	ProjectRoot    string
	// Token is only ever non-empty on a loopback bind (Start never generates
	// a fleetToken for a non-loopback bind, so there is nothing to embed
	// there); FLEET-026 requires the non-loopback token to be delivered
	// out-of-band via the server log instead.
	Token string
}

// effectiveLoopbackBind normalizes bind state for guard/page purposes: a
// Server whose bindHost was never set by Start (bare &Server{...} as built
// directly by unit tests, or a caller that skipped Start) is treated as a
// loopback bind, matching FLEET-026's "zero-value Server behaves like
// loopback" contract.
func (s *Server) effectiveLoopbackBind() bool {
	if s.bindHost == "" {
		return true
	}
	return s.loopbackBind
}

// fleetSecurityConfig snapshots the Server fields guardFleetToggle needs.
func (s *Server) fleetSecurityConfig() fleetSecurityConfig {
	return fleetSecurityConfig{
		bindHost:     s.bindHost,
		bindPort:     s.bindPort,
		fleetToken:   s.fleetToken,
		loopbackBind: s.effectiveLoopbackBind(),
	}
}

// fleetPageHandler renders the read-only /fleet dashboard shell.
func (s *Server) fleetPageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data := fleetPageData{PollIntervalMS: 5000, ProjectRoot: s.projectRoot}
	if s.effectiveLoopbackBind() {
		data.Token = s.fleetToken
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := loadFleetTemplate().Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// fleetStatusHandler exposes core.BuildFleetStatusReport verbatim: the
// per-target kill-switch state read from .ai/fleet-control.json via the
// FLEET-023-a core loader. Fields are never reshaped or renamed.
func (s *Server) fleetStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.projectRoot == "" {
		http.Error(w, "Fleet dashboard unavailable: no project root resolved", http.StatusServiceUnavailable)
		return
	}

	state, err := core.LoadFleetControlState(s.projectRoot)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	report := core.BuildFleetStatusReport(state, time.Now().UTC())

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}

// fleetToggleRequest is the JSON body for POST /api/fleet/toggle.
type fleetToggleRequest struct {
	Target string `json:"target"`
	Action string `json:"action"`
	Reason string `json:"reason"`
}

// fleetToggleHandler is the sole write route the web UI exposes: it calls
// core.DisableFleetTarget/core.EnableFleetTarget exclusively -- it never
// writes .ai/fleet-control.json directly -- so the CLI (quorum fleet
// disable/enable) and the HTTP path share exactly one write function.
func (s *Server) fleetToggleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.projectRoot == "" {
		http.Error(w, "Fleet dashboard unavailable: no project root resolved", http.StatusServiceUnavailable)
		return
	}

	if ok, status, message := guardFleetToggle(r, s.fleetSecurityConfig()); !ok {
		http.Error(w, message, status)
		return
	}

	var req fleetToggleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	target := strings.TrimSpace(req.Target)
	if target == "" {
		http.Error(w, "target is required", http.StatusBadRequest)
		return
	}

	var state core.ControlState
	var err error
	switch req.Action {
	case "disable":
		reason := strings.TrimSpace(req.Reason)
		if reason == "" {
			http.Error(w, "reason is required to disable a target", http.StatusBadRequest)
			return
		}
		state, err = core.DisableFleetTarget(s.projectRoot, target, reason, "human")
	case "enable":
		state, err = core.EnableFleetTarget(s.projectRoot, target)
	default:
		http.Error(w, fmt.Sprintf("unknown action: %q", req.Action), http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	report := core.BuildFleetStatusReport(state, time.Now().UTC())
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}

// dispatchRecord is one completed (attempt/reroute) dispatch surfaced on the
// dashboard, joined from 07-trace.json events[] (and, for attempt-class
// dispatches, the corresponding attempts[] entry).
type dispatchRecord struct {
	TaskID       string   `json:"task_id"`
	DispatchID   string   `json:"dispatch_id,omitempty"`
	Phase        string   `json:"phase,omitempty"`
	Agent        string   `json:"agent,omitempty"`
	Model        string   `json:"model,omitempty"`
	OutcomeClass string   `json:"outcome_class"`
	Result       string   `json:"result,omitempty"`
	DurationS    *float64 `json:"duration_s,omitempty"`
	TokensIn     *int     `json:"tokens_in,omitempty"`
	TokensOut    *int     `json:"tokens_out,omitempty"`
	CostUSD      *float64 `json:"cost_usd,omitempty"`
	TS           string   `json:"ts,omitempty"`
}

// inFlightRecord is a dispatch that started but never reached a matching
// dispatch_finished event in the trace.
type inFlightRecord struct {
	TaskID     string  `json:"task_id"`
	DispatchID string  `json:"dispatch_id,omitempty"`
	StartedAt  string  `json:"started_at,omitempty"`
	AgeSeconds float64 `json:"age_seconds"`
}

// blockedRecord is a task parked on an unanswered blocked_question event.
type blockedRecord struct {
	TaskID     string  `json:"task_id"`
	Path       string  `json:"path,omitempty"`
	Reason     string  `json:"reason,omitempty"`
	Severity   string  `json:"severity,omitempty"`
	At         string  `json:"at,omitempty"`
	AgeSeconds float64 `json:"age_seconds"`
}

// fleetDispatchesResponse is the aggregate payload for /api/fleet/dispatches.
type fleetDispatchesResponse struct {
	GeneratedAt string           `json:"generated_at"`
	Dispatches  []dispatchRecord `json:"dispatches"`
	InFlight    []inFlightRecord `json:"in_flight"`
	Blocked     []blockedRecord  `json:"blocked"`
}

type pendingDispatch struct {
	dispatchID string
	ts         string
}

type pendingBlocked struct {
	ts       string
	path     string
	reason   string
	severity string
}

// buildTaskDispatchView is the pure, table-testable core of the dispatches
// view: it walks one task's raw 07-trace.json payload (already unmarshalled
// via core.LoadArtifactPayload) and derives dispatch/in-flight/blocked
// records. It never panics on malformed input -- callers should treat any
// unreadable/malformed trace as skippable, not fatal to the whole request.
func buildTaskDispatchView(taskID string, trace map[string]any, now time.Time) (dispatches []dispatchRecord, inFlight *inFlightRecord, blocked *blockedRecord) {
	rawEvents, _ := trace["events"].([]any)

	var pendingDisp *pendingDispatch
	var pendingBlk *pendingBlocked
	agent, model := "", ""

	// attemptClassDispatches records, in declared order, the index-position
	// placeholder for each outcome_class=="attempt" dispatchRecord emitted
	// below; attemptRecords records the phase=="execute" attempts[] entries in
	// order. They are joined positionally after both walks complete.
	var attemptClassIdx []int

	for _, raw := range rawEvents {
		ev, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		evType, _ := ev["type"].(string)
		switch evType {
		case "routing_decision":
			if cand, ok := ev["candidate"].(map[string]any); ok {
				if a, ok := cand["agent"].(string); ok {
					agent = a
				}
				if m, ok := cand["model"].(string); ok {
					model = m
				}
			}
		case "dispatch_started":
			dispatchID, _ := ev["dispatch_id"].(string)
			ts, _ := ev["ts"].(string)
			pendingDisp = &pendingDispatch{dispatchID: dispatchID, ts: ts}
		case "dispatch_finished":
			dispatchID, _ := ev["dispatch_id"].(string)
			ts, _ := ev["ts"].(string)
			outcomeClass, _ := ev["outcome_class"].(string)
			if pendingDisp != nil && pendingDisp.dispatchID == dispatchID {
				pendingDisp = nil
			}
			rec := dispatchRecord{
				TaskID:       taskID,
				DispatchID:   dispatchID,
				Phase:        "execute",
				Agent:        agent,
				Model:        model,
				OutcomeClass: outcomeClass,
				TS:           ts,
			}
			dispatches = append(dispatches, rec)
			if outcomeClass == "attempt" {
				attemptClassIdx = append(attemptClassIdx, len(dispatches)-1)
			}
		case "blocked_question":
			path, _ := ev["path"].(string)
			reason, _ := ev["reason"].(string)
			severity, _ := ev["severity"].(string)
			ts, _ := ev["ts"].(string)
			pendingBlk = &pendingBlocked{ts: ts, path: path, reason: reason, severity: severity}
		case "blocked_answer":
			pendingBlk = nil
		}
	}

	// Positional join: attempts[] filtered to phase=="execute" grows 1:1 with
	// outcome_class=="attempt" dispatch_finished events inside a single
	// appendDispatchTrace writer (internal/core/fleet_dispatch.go). Capped at
	// min length so a mismatch never panics -- it just leaves extra records
	// without duration/tokens.
	rawAttempts, _ := trace["attempts"].([]any)
	var attemptRecords []map[string]any
	for _, raw := range rawAttempts {
		a, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if phase, _ := a["phase"].(string); phase == "execute" {
			attemptRecords = append(attemptRecords, a)
		}
	}
	joinN := len(attemptClassIdx)
	if len(attemptRecords) < joinN {
		joinN = len(attemptRecords)
	}
	for i := 0; i < joinN; i++ {
		idx := attemptClassIdx[i]
		a := attemptRecords[i]
		if result, ok := a["result"].(string); ok {
			dispatches[idx].Result = result
		}
		if d, ok := a["duration_s"].(float64); ok {
			dispatches[idx].DurationS = &d
		}
		if m, ok := a["model"].(string); ok && m != "" {
			dispatches[idx].Model = m
		}
		if v, ok := a["tokens_in"].(float64); ok {
			iv := int(v)
			dispatches[idx].TokensIn = &iv
		}
		if v, ok := a["tokens_out"].(float64); ok {
			iv := int(v)
			dispatches[idx].TokensOut = &iv
		}
		if v, ok := a["cost_usd"].(float64); ok {
			dispatches[idx].CostUSD = &v
		}
	}

	if pendingDisp != nil {
		age := 0.0
		if parsed, err := time.Parse(time.RFC3339, pendingDisp.ts); err == nil {
			age = now.Sub(parsed).Seconds()
		}
		inFlight = &inFlightRecord{
			TaskID:     taskID,
			DispatchID: pendingDisp.dispatchID,
			StartedAt:  pendingDisp.ts,
			AgeSeconds: age,
		}
	}

	if pendingBlk != nil {
		age := 0.0
		if parsed, err := time.Parse(time.RFC3339, pendingBlk.ts); err == nil {
			age = now.Sub(parsed).Seconds()
		}
		blocked = &blockedRecord{
			TaskID:     taskID,
			Path:       pendingBlk.path,
			Reason:     pendingBlk.reason,
			Severity:   pendingBlk.severity,
			At:         pendingBlk.ts,
			AgeSeconds: age,
		}
	}

	return dispatches, inFlight, blocked
}

// fleetDispatchesHandler aggregates dispatch/in-flight/blocked records across
// every task's 07-trace.json under .ai/tasks/{active,done,failed} (inbox is
// skipped -- pre-blueprint tasks are never dispatched).
func (s *Server) fleetDispatchesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.projectRoot == "" {
		http.Error(w, "Fleet dashboard unavailable: no project root resolved", http.StatusServiceUnavailable)
		return
	}

	limit, err := parseOptionalInt(r.URL.Query().Get("limit"), 50)
	if err != nil {
		http.Error(w, "Invalid limit", http.StatusBadRequest)
		return
	}

	now := time.Now().UTC()
	resp := fleetDispatchesResponse{
		GeneratedAt: now.Format(time.RFC3339),
		Dispatches:  []dispatchRecord{},
		InFlight:    []inFlightRecord{},
		Blocked:     []blockedRecord{},
	}

	for _, loc := range []string{"active", "done", "failed"} {
		res, err := core.QueryTasks(core.TaskListOptions{ProjectRoot: s.projectRoot, Location: loc})
		if err != nil {
			continue
		}
		for _, item := range res.Items {
			if !item.Artifacts["07-trace.json"] {
				continue
			}
			tracePath := filepath.Join(s.projectRoot, ".ai", "tasks", item.Location, item.Directory, "07-trace.json")
			payload, err := core.LoadArtifactPayload(tracePath)
			if err != nil {
				continue
			}
			trace, ok := payload.(map[string]any)
			if !ok {
				continue
			}
			taskID := item.ID
			if taskID == "" {
				taskID = item.Directory
			}
			d, inF, blk := buildTaskDispatchView(taskID, trace, now)
			resp.Dispatches = append(resp.Dispatches, d...)
			if inF != nil {
				resp.InFlight = append(resp.InFlight, *inF)
			}
			if blk != nil {
				resp.Blocked = append(resp.Blocked, *blk)
			}
		}
	}

	sort.SliceStable(resp.Dispatches, func(i, j int) bool {
		return resp.Dispatches[i].TS > resp.Dispatches[j].TS
	})
	if limit > 0 && len(resp.Dispatches) > limit {
		resp.Dispatches = resp.Dispatches[:limit]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
