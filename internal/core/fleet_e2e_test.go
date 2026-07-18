package core

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// FLEET-022-d: end-to-end composition test wiring the real core.Route,
// core.BuildBundle, and core.Dispatch together in one flow. It never invokes
// a real transport binary: every Dispatch() below runs the existing
// FLEET_FAKE_MODE self-exec fixture (TestMain in
// fleet_dispatch_helper_test.go, os.Args[0] re-exec, same pattern as
// fleet_dispatch_fake_test.go/fleet_dispatch_test.go), so no OpenRouter or
// subscription quota is spent. This file defines no testing.M/TestMain of
// its own; it reuses the package's single existing one.

// e2eAgentsYAML is a test-local duplicate of cmd/fleet_route.go's
// fleetRouteAgentsFile shim. internal/core cannot import the cmd package (no
// reverse dependency in this monorepo's package layout -- see
// 01-blueprint.yaml risks), so this e2e test must re-parse the same additive
// slice of .agents/fleet/agents.yaml the real `quorum fleet route` CLI reads.
// If that CLI shim's parsing logic ever drifts from this duplicate, this
// test's fidelity to the real binary can silently degrade over time; that
// structural coupling is documented, not eliminated, by this task.
type e2eAgentsYAML struct {
	Transports map[string]struct {
		Active bool `yaml:"active"`
		Models map[string]struct {
			Provider string `yaml:"provider"`
		} `yaml:"models"`
	} `yaml:"transports"`
}

// e2eConfigYAML is a test-local duplicate of the slice of .agents/config.yaml
// that cmd/fleet_route.go's fleetRouteConfigFile shim reads (levels map plus
// the transport ordering list). Levels reuses core.LevelModels directly since
// it already carries the matching yaml tags.
type e2eConfigYAML struct {
	Levels   map[int]LevelModels `yaml:"levels"`
	Policies struct {
		FleetTransportOrder []string `yaml:"fleet_transport_order"`
	} `yaml:"policies"`
}

// e2eRoutingYAML is a test-local duplicate of the `rules:` slice of
// routing.yaml that cmd/fleet_route.go's fleetRouteRoutingFile shim reads.
// Legacy keys unrelated to core.RoutingRule are silently ignored by
// yaml.Unmarshal, mirroring the real CLI shim.
type e2eRoutingYAML struct {
	Rules []RoutingRule `yaml:"rules"`
}

// loadRealRoutePolicy parses THIS repository's own ratified policy files
// (.agents/config.yaml, .agents/policies/routing.yaml,
// .agents/fleet/agents.yaml resolved from sourceRoot, never a synthetic
// per-test fixture) into a RoutePolicy, mirroring
// cmd/fleet_route.go's buildRoutePolicyFromDisk. This is what lets Route()
// below pick a candidate from the real, ratified G1 cell set instead of a
// fabricated policy.
func loadRealRoutePolicy(t *testing.T) RoutePolicy {
	t.Helper()
	root := sourceRoot(t)

	configPath := filepath.Join(root, ".agents", "config.yaml")
	routingPath := filepath.Join(root, ".agents", "policies", "routing.yaml")
	agentsPath := filepath.Join(root, ".agents", "fleet", "agents.yaml")

	configRaw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read %s: %v", configPath, err)
	}
	routingRaw, err := os.ReadFile(routingPath)
	if err != nil {
		t.Fatalf("read %s: %v", routingPath, err)
	}
	agentsRaw, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read %s: %v", agentsPath, err)
	}

	var config e2eConfigYAML
	if err := yaml.Unmarshal(configRaw, &config); err != nil {
		t.Fatalf("parse %s: %v", configPath, err)
	}
	var routing e2eRoutingYAML
	if err := yaml.Unmarshal(routingRaw, &routing); err != nil {
		t.Fatalf("parse %s: %v", routingPath, err)
	}
	var agents e2eAgentsYAML
	if err := yaml.Unmarshal(agentsRaw, &agents); err != nil {
		t.Fatalf("parse %s: %v", agentsPath, err)
	}

	transports := make([]TransportAvailability, 0, len(config.Policies.FleetTransportOrder))
	for _, name := range config.Policies.FleetTransportOrder {
		tr, ok := agents.Transports[name]
		if !ok {
			continue // absent from agents.yaml -> unreachable via fleet route
		}
		ta := TransportAvailability{Agent: name, Active: tr.Active}
		for model, m := range tr.Models {
			ta.Models = append(ta.Models, ModelAvailability{Model: model, Family: m.Provider})
		}
		transports = append(transports, ta)
	}

	return RoutePolicy{
		Routing:    routing.Rules,
		Levels:     config.Levels,
		Transports: transports,
	}
}

// TestFleetE2ERouteBundleDispatchAttempt exercises the happy-path leg:
// Route() picks a live candidate from the real ratified policy at
// phase=implement/risk=low/band=S, BuildBundle assembles a prompt/manifest
// for a minimal fixture task whose bundle_hash flows unchanged into
// DispatchSpec.BundleHash, and Dispatch() against the FLEET_FAKE_MODE
// success_diff fixture classifies an applied attempt with the expected trace
// events.
func TestFleetE2ERouteBundleDispatchAttempt(t *testing.T) {
	policy := loadRealRoutePolicy(t)
	result, err := Route(RouteRequest{Phase: "implement", Risk: "low", ComplexityBand: "S"}, policy)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if result.Candidate == nil {
		t.Fatalf("want a viable candidate from the real G1 policy, got blocked=%q reasons=%v", result.Blocked, result.Reasons)
	}

	env := setupDispatchEnv(t)
	bundle, err := BuildBundle(BundleInput{
		TaskID:   env.taskID,
		SpecYAML: []byte("task_id: " + env.taskID + "\nsummary: e2e fixture spec\n"),
	}, 0, "2026-07-18T00:00:00Z")
	if err != nil {
		t.Fatalf("BuildBundle: %v", err)
	}

	spec := env.fakeSpec("d1")
	spec.Agent = result.Candidate.Agent
	spec.Model = result.Candidate.Model
	spec.BundleHash = bundle.Manifest.BundleHash

	t.Setenv("FLEET_FAKE_MODE", "success_diff")
	res, err := Dispatch(spec)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if res.Outcome.Class != "attempt" || !res.Applied {
		t.Fatalf("want an applied attempt, got outcome=%+v applied=%v", res.Outcome, res.Applied)
	}
	if res.Agent != result.Candidate.Agent || res.Model != result.Candidate.Model {
		t.Fatalf("dispatch result identity = %s/%s, want the routed candidate %s/%s",
			res.Agent, res.Model, result.Candidate.Agent, result.Candidate.Model)
	}

	events := loadTraceEvents(t, env.taskDir)
	types := eventTypesInTrace(events)
	if !containsStr(types, "dispatch_started") || !containsStr(types, "dispatch_finished") {
		t.Fatalf("trace events = %v, want dispatch_started and dispatch_finished", types)
	}
	for _, e := range events {
		if e["type"] != "dispatch_started" {
			continue
		}
		if got, _ := e["bundle_hash"].(string); got != bundle.Manifest.BundleHash {
			t.Fatalf("dispatch_started bundle_hash = %q, want %q (unchanged from BuildBundle's manifest)", got, bundle.Manifest.BundleHash)
		}
	}
}

// TestFleetE2ERouteBundleDispatchBlocked exercises the blocked leg: a live
// routed candidate is dispatched against the FLEET_FAKE_MODE=blocked
// fixture, which must classify as blocked with a parsed BlockedSignal, emit
// a blocked_question trace event, and append zero execute attempts.
func TestFleetE2ERouteBundleDispatchBlocked(t *testing.T) {
	policy := loadRealRoutePolicy(t)
	result, err := Route(RouteRequest{Phase: "implement", Risk: "low", ComplexityBand: "S"}, policy)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if result.Candidate == nil {
		t.Fatalf("want a viable candidate from the real G1 policy, got blocked=%q reasons=%v", result.Blocked, result.Reasons)
	}

	env := setupDispatchEnv(t)
	spec := env.fakeSpec("d1")
	spec.Agent = result.Candidate.Agent
	spec.Model = result.Candidate.Model

	t.Setenv("FLEET_FAKE_MODE", "blocked")
	res, err := Dispatch(spec)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if res.Outcome.Class != "blocked" || res.Outcome.Blocked == nil {
		t.Fatalf("want a blocked outcome with a parsed signal, got %+v", res.Outcome)
	}
	want := BlockedSignal{Path: "cmd/new_helper.go", Reason: "needs a helper not in touch", Severity: "critical"}
	if *res.Outcome.Blocked != want {
		t.Fatalf("parsed blocked signal = %+v, want %+v", *res.Outcome.Blocked, want)
	}
	if n := countExecuteAttempts(t, env.taskDir); n != 0 {
		t.Fatalf("blocked leg must append zero execute attempts, got %d", n)
	}
	types := eventTypesInTrace(loadTraceEvents(t, env.taskDir))
	if !containsStr(types, "blocked_question") {
		t.Fatalf("trace events = %v, want blocked_question", types)
	}
}

// TestFleetE2ERerouteThenDispatch exercises route -> exclude -> reroute ->
// dispatch in one flow: a second Route() call with the first candidate
// appended to Exclusions must return a different, non-excluded candidate,
// which is itself dispatched successfully end to end against the
// FLEET_FAKE_MODE success_diff fixture.
func TestFleetE2ERerouteThenDispatch(t *testing.T) {
	policy := loadRealRoutePolicy(t)

	first, err := Route(RouteRequest{Phase: "implement", Risk: "low", ComplexityBand: "S"}, policy)
	if err != nil {
		t.Fatalf("Route (first): %v", err)
	}
	if first.Candidate == nil {
		t.Fatalf("want a first candidate from the real G1 policy, got blocked=%q reasons=%v", first.Blocked, first.Reasons)
	}

	second, err := Route(RouteRequest{
		Phase: "implement", Risk: "low", ComplexityBand: "S",
		Exclusions: []Candidate{*first.Candidate},
	}, policy)
	if err != nil {
		t.Fatalf("Route (reroute): %v", err)
	}
	if second.Candidate == nil {
		t.Fatalf("want a rerouted candidate, got blocked=%q reasons=%v", second.Blocked, second.Reasons)
	}
	if *second.Candidate == *first.Candidate {
		t.Fatalf("reroute must exclude the first candidate, got the same candidate twice: %+v", *second.Candidate)
	}

	env := setupDispatchEnv(t)
	spec := env.fakeSpec("d2")
	spec.Agent = second.Candidate.Agent
	spec.Model = second.Candidate.Model

	t.Setenv("FLEET_FAKE_MODE", "success_diff")
	res, err := Dispatch(spec)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if res.Outcome.Class != "attempt" || !res.Applied {
		t.Fatalf("want the rerouted candidate to dispatch successfully, got outcome=%+v applied=%v", res.Outcome, res.Applied)
	}
	if res.Agent != second.Candidate.Agent || res.Model != second.Candidate.Model {
		t.Fatalf("dispatch result identity = %s/%s, want the rerouted candidate %s/%s",
			res.Agent, res.Model, second.Candidate.Agent, second.Candidate.Model)
	}
}
