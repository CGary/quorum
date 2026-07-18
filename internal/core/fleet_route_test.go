package core

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

// Fake model names live only in this test file. The router source must contain
// none of them (see TestRoutePurityNoHardcodedModelNames), proving zero model
// names are hardcoded in Go and that changing policy changes routing without
// recompiling.
const (
	mL0Primary  = "nimbus/spark-lite"
	mL0Fallback = "orbit/oss-base"
	mL1Primary  = "quill/mind-mid"
	mL1Fallback = "nimbus/spark-pro"
	mL1Second   = "zephyr/sec-tiny"
	mL2Primary  = "quill/sage-max"
	mL2Fallback = "orbit/oss-high"
)

var fixtureModelNames = []string{
	mL0Primary, mL0Fallback, mL1Primary, mL1Fallback, mL1Second, mL2Primary, mL2Fallback,
}

func fixtureRules() []RoutingRule {
	r := func(risk, band string, lvl, max int) RoutingRule {
		return RoutingRule{
			Match:         RoutingMatch{Phase: "implement", Risk: risk, Band: band},
			ExecutorLevel: lvl,
			MaxAttempts:   max,
		}
	}
	// low/L uses a distinct budget (3) so the reroute_budget test proves the
	// value is read from policy, not a Go constant.
	return []RoutingRule{
		r("low", "S", 0, 2),
		r("low", "M", 1, 2),
		r("low", "L", 2, 3),
		r("medium", "S", 1, 2),
		r("medium", "M", 1, 2),
		r("medium", "L", 2, 1),
		r("high", "S", 2, 1),
		r("high", "M", 2, 1),
		r("high", "L", 2, 1),
	}
}

func fixturePolicy() RoutePolicy {
	runnerA := TransportAvailability{
		Agent:  "runner-a",
		Active: true,
		Models: []ModelAvailability{
			{Model: mL0Primary, Family: "nimbus"},
			{Model: mL0Fallback, Family: "orbit"},
			{Model: mL1Primary, Family: "quill"},
			{Model: mL1Fallback, Family: "nimbus"},
			{Model: mL1Second, Family: "zephyr"},
			{Model: mL2Primary, Family: "quill"},
			{Model: mL2Fallback, Family: "orbit"},
		},
	}
	// runner-b provides the same L1 primary model, so the transport slice order
	// is the documented tie-break for equal models.
	runnerB := TransportAvailability{
		Agent:  "runner-b",
		Active: true,
		Models: []ModelAvailability{{Model: mL1Primary, Family: "quill"}},
	}
	return RoutePolicy{
		Routing: fixtureRules(),
		Levels: map[int]LevelModels{
			0: {Primary: mL0Primary, Fallback: mL0Fallback},
			1: {Primary: mL1Primary, Fallback: mL1Fallback, Secondary: []string{mL1Second}},
			2: {Primary: mL2Primary, Fallback: mL2Fallback},
		},
		Transports: []TransportAvailability{runnerA, runnerB},
		Hashes:     PolicyHashes{ConfigYAML: "cfg-h", RoutingYAML: "route-h", AgentsYAML: "agents-h"},
	}
}

func req(risk, band string) RouteRequest {
	return RouteRequest{Phase: "implement", Risk: risk, ComplexityBand: band}
}

func TestRouteRiskBandMatrix(t *testing.T) {
	cases := []struct {
		risk, band string
		wantLevel  int
		wantModel  string
	}{
		{"low", "S", 0, mL0Primary},
		{"low", "M", 1, mL1Primary},
		{"low", "L", 2, mL2Primary},
		{"medium", "S", 1, mL1Primary},
		{"medium", "M", 1, mL1Primary},
		{"medium", "L", 2, mL2Primary},
		{"high", "S", 2, mL2Primary},
		{"high", "M", 2, mL2Primary},
		{"high", "L", 2, mL2Primary},
	}
	policy := fixturePolicy()
	for _, tc := range cases {
		t.Run(tc.risk+"_"+tc.band, func(t *testing.T) {
			res, err := Route(req(tc.risk, tc.band), policy)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Blocked != "" {
				t.Fatalf("unexpected block: %s %v", res.Blocked, res.Reasons)
			}
			if res.Candidate == nil {
				t.Fatal("expected a candidate, got nil")
			}
			if res.Candidate.Level != tc.wantLevel {
				t.Errorf("level: got %d want %d", res.Candidate.Level, tc.wantLevel)
			}
			if res.Candidate.Model != tc.wantModel {
				t.Errorf("model: got %q want %q", res.Candidate.Model, tc.wantModel)
			}
			if res.Candidate.Agent != "runner-a" {
				t.Errorf("agent: got %q want runner-a", res.Candidate.Agent)
			}
			if res.InputsSnapshot.Level != tc.wantLevel {
				t.Errorf("snapshot level: got %d want %d", res.InputsSnapshot.Level, tc.wantLevel)
			}
		})
	}
}

func TestRouteDeterministicOrdering(t *testing.T) {
	policy := fixturePolicy()
	r := req("medium", "S") // level 1

	// Primary preference wins, first transport in slice order.
	res, err := Route(r, policy)
	if err != nil {
		t.Fatal(err)
	}
	if res.Candidate.Agent != "runner-a" || res.Candidate.Model != mL1Primary {
		t.Fatalf("first pick: got %v want runner-a/%s", res.Candidate, mL1Primary)
	}

	// Excluding the first transport's primary falls to the next transport that
	// provides the SAME model (transport slice order tie-break).
	r2 := req("medium", "S")
	r2.Exclusions = []Candidate{{Agent: "runner-a", Model: mL1Primary}}
	res2, err := Route(r2, policy)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Candidate.Agent != "runner-b" || res2.Candidate.Model != mL1Primary {
		t.Fatalf("tie-break pick: got %v want runner-b/%s", res2.Candidate, mL1Primary)
	}

	// Repeated identical calls yield identical results (map iteration never
	// leaks into selection).
	first, _ := Route(r, policy)
	for i := 0; i < 5; i++ {
		got, _ := Route(r, policy)
		if !reflect.DeepEqual(first, got) {
			t.Fatalf("non-deterministic result on iteration %d", i)
		}
	}
}

func TestRouteControlFiltering(t *testing.T) {
	policy := fixturePolicy()

	t.Run("model_target_disable", func(t *testing.T) {
		r := req("medium", "S")
		r.Control = ControlState{Disabled: []ControlEntry{{Target: "runner-a/" + mL1Primary}}}
		res, err := Route(r, policy)
		if err != nil {
			t.Fatal(err)
		}
		if res.Candidate.Agent != "runner-b" || res.Candidate.Model != mL1Primary {
			t.Fatalf("got %v want runner-b/%s", res.Candidate, mL1Primary)
		}
	})

	t.Run("agent_target_disable_falls_to_fallback", func(t *testing.T) {
		r := req("medium", "S")
		// Disable runner-a's primary model and runner-b entirely; the surviving
		// candidate is runner-a's fallback preference.
		r.Control = ControlState{Disabled: []ControlEntry{
			{Target: "runner-a/" + mL1Primary},
			{Target: "runner-b"},
		}}
		res, err := Route(r, policy)
		if err != nil {
			t.Fatal(err)
		}
		if res.Candidate.Agent != "runner-a" || res.Candidate.Model != mL1Fallback {
			t.Fatalf("got %v want runner-a/%s", res.Candidate, mL1Fallback)
		}
	})
}

func TestRouteExclusionFiltering(t *testing.T) {
	policy := fixturePolicy()
	r := req("medium", "S")
	r.Exclusions = []Candidate{
		{Agent: "runner-a", Model: mL1Primary},
		{Agent: "runner-b", Model: mL1Primary},
	}
	res, err := Route(r, policy)
	if err != nil {
		t.Fatal(err)
	}
	if res.Candidate.Agent != "runner-a" || res.Candidate.Model != mL1Fallback {
		t.Fatalf("got %v want runner-a/%s", res.Candidate, mL1Fallback)
	}
}

func TestRouteNoViableCandidate(t *testing.T) {
	policy := fixturePolicy()
	r := req("low", "S") // level 0, models only on runner-a
	r.Control = ControlState{Disabled: []ControlEntry{{Target: "runner-a"}}}
	res, err := Route(r, policy)
	if err != nil {
		t.Fatalf("dead-end must not error: %v", err)
	}
	if res.Candidate != nil {
		t.Fatalf("expected nil candidate, got %v", res.Candidate)
	}
	if res.Blocked != "no_viable_candidate" {
		t.Fatalf("blocked: got %q want no_viable_candidate", res.Blocked)
	}
	if len(res.Reasons) == 0 {
		t.Error("expected reasons for the block")
	}
	if res.InputsSnapshot.Level != 0 {
		t.Errorf("snapshot must still carry the resolved level, got %d", res.InputsSnapshot.Level)
	}
}

func TestRouteRerouteByReinvocation(t *testing.T) {
	policy := fixturePolicy()
	r := req("low", "L") // level 2

	first, err := Route(r, policy)
	if err != nil {
		t.Fatal(err)
	}
	if first.Candidate.Model != mL2Primary {
		t.Fatalf("first: got %q want %q", first.Candidate.Model, mL2Primary)
	}

	// Reroute is strictly re-running with the failed candidate excluded.
	r2 := req("low", "L")
	r2.Exclusions = append(r.Exclusions, *first.Candidate)
	second, err := Route(r2, policy)
	if err != nil {
		t.Fatal(err)
	}
	if second.Candidate.Model == first.Candidate.Model {
		t.Fatalf("reroute returned the same candidate %q", second.Candidate.Model)
	}
	if second.Candidate.Model != mL2Fallback {
		t.Fatalf("reroute: got %q want %q", second.Candidate.Model, mL2Fallback)
	}

	// Both snapshots must be sufficient to reconstruct the decision.
	for _, s := range []InputsSnapshot{first.InputsSnapshot, second.InputsSnapshot} {
		if s.RouterVersion == "" {
			t.Error("snapshot missing router version")
		}
		if s.Risk != "low" || s.ComplexityBand != "L" || s.Phase != "implement" {
			t.Errorf("snapshot request fields wrong: %+v", s)
		}
		if s.Hashes.ConfigYAML == "" || s.Hashes.RoutingYAML == "" || s.Hashes.AgentsYAML == "" {
			t.Errorf("snapshot missing policy hashes: %+v", s.Hashes)
		}
	}
	if len(second.InputsSnapshot.Exclusions) != 1 {
		t.Errorf("reroute snapshot must carry the exclusion, got %v", second.InputsSnapshot.Exclusions)
	}
}

func TestRouteRerouteBudgetFromPolicy(t *testing.T) {
	policy := fixturePolicy()

	lowL, _ := Route(req("low", "L"), policy)
	if lowL.RerouteBudget != 3 {
		t.Errorf("low/L budget: got %d want 3 (policy max_attempts)", lowL.RerouteBudget)
	}
	medL, _ := Route(req("medium", "L"), policy)
	if medL.RerouteBudget != 1 {
		t.Errorf("medium/L budget: got %d want 1 (policy max_attempts)", medL.RerouteBudget)
	}
}

func TestRouteFamilyDiversitySoftPreference(t *testing.T) {
	policy := fixturePolicy()

	t.Run("prefers_diverse_family", func(t *testing.T) {
		r := req("medium", "S") // level 1
		r.IncumbentFamily = "quill"
		res, err := Route(r, policy)
		if err != nil {
			t.Fatal(err)
		}
		// L1 primary is family quill; the first non-quill survivor is the fallback.
		if res.Candidate.Model != mL1Fallback {
			t.Fatalf("got %q want %q (diverse family)", res.Candidate.Model, mL1Fallback)
		}
		if res.ReviewFamilyDegraded {
			t.Error("should not be degraded when a diverse candidate exists")
		}
	})

	t.Run("degrades_without_alternative_but_never_blocks", func(t *testing.T) {
		r := req("medium", "S")
		r.IncumbentFamily = "quill"
		// Remove every non-quill survivor; only same-family candidates remain.
		r.Exclusions = []Candidate{
			{Agent: "runner-a", Model: mL1Fallback},
			{Agent: "runner-a", Model: mL1Second},
		}
		res, err := Route(r, policy)
		if err != nil {
			t.Fatal(err)
		}
		if res.Candidate == nil {
			t.Fatal("family diversity must never block; expected a candidate")
		}
		if res.Candidate.Model != mL1Primary {
			t.Fatalf("got %q want %q", res.Candidate.Model, mL1Primary)
		}
		if !res.ReviewFamilyDegraded {
			t.Error("expected ReviewFamilyDegraded=true when only same-family remains")
		}
	})
}

// TestRouteInputsSnapshotIncumbentFamily (AC-4) asserts the snapshot carries
// the caller-supplied IncumbentFamily verbatim — the SAME value the request
// held, independent of which candidate the diversity preference actually
// picked — and defaults to "" when unset.
func TestRouteInputsSnapshotIncumbentFamily(t *testing.T) {
	policy := fixturePolicy()
	cases := []struct {
		name      string
		incumbent string
		want      string
	}{
		// "quill" differs from the level-1 survivor family that ultimately wins
		// (mL1Fallback is family "nimbus"); the snapshot still echoes "quill".
		{"non_empty_differs_from_pick_family", "quill", "quill"},
		{"empty_defaults_to_zero_value", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := req("medium", "S") // level 1
			r.IncumbentFamily = tc.incumbent
			res, err := Route(r, policy)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.InputsSnapshot.IncumbentFamily != tc.want {
				t.Errorf("snapshot IncumbentFamily: got %q want %q", res.InputsSnapshot.IncumbentFamily, tc.want)
			}
		})
	}
}

func TestRouteMalformedPolicy(t *testing.T) {
	t.Run("no_routing_rules", func(t *testing.T) {
		if _, err := Route(req("low", "S"), RoutePolicy{}); err == nil {
			t.Error("expected error for empty routing rules")
		}
	})

	t.Run("no_matching_rule", func(t *testing.T) {
		if _, err := Route(req("unknown", "S"), fixturePolicy()); err == nil {
			t.Error("expected error when no rule matches")
		}
	})

	t.Run("resolved_level_absent", func(t *testing.T) {
		policy := fixturePolicy()
		policy.Routing = []RoutingRule{{
			Match:         RoutingMatch{Phase: "implement", Risk: "low", Band: "S"},
			ExecutorLevel: 9,
			MaxAttempts:   2,
		}}
		if _, err := Route(req("low", "S"), policy); err == nil {
			t.Error("expected error when resolved level is absent from levels map")
		}
	})
}

func TestRouteCustomRouterVersion(t *testing.T) {
	policy := fixturePolicy()
	policy.RouterVersion = "fleet-route/test-v9"
	res, _ := Route(req("low", "S"), policy)
	if res.InputsSnapshot.RouterVersion != "fleet-route/test-v9" {
		t.Errorf("got %q want caller-supplied version", res.InputsSnapshot.RouterVersion)
	}

	def, _ := Route(req("low", "S"), fixturePolicy())
	if def.InputsSnapshot.RouterVersion != RouterVersion {
		t.Errorf("empty policy version must fall back to const, got %q", def.InputsSnapshot.RouterVersion)
	}
}

// TestRoutePurityNoHardcodedModelNames enforces the zero-hardcoded-model-names
// invariant mechanically: it reads fleet_route.go's source and asserts no
// fixture model-name literal appears in it.
func TestRoutePurityNoHardcodedModelNames(t *testing.T) {
	src, err := os.ReadFile("fleet_route.go")
	if err != nil {
		t.Fatalf("read router source: %v", err)
	}
	text := string(src)
	for _, name := range fixtureModelNames {
		if strings.Contains(text, name) {
			t.Errorf("fleet_route.go must not hardcode model name %q", name)
		}
	}
}
