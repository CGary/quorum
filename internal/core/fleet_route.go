package core

import "fmt"

// RouterVersion identifies the routing algorithm that produced a decision. It
// is a router identifier baked into the inputs snapshot for reproducibility; it
// is NEVER a model or agent name. When a caller supplies its own
// RoutePolicy.RouterVersion, that value wins and this const is the fallback.
const RouterVersion = "fleet-route/v1"

// Candidate is the single executor identity used everywhere in this package:
// the returned pick, exclusion entries, and the reroute append. Level is the
// executor level the candidate was resolved at.
type Candidate struct {
	Agent string `json:"agent" yaml:"agent"`
	Model string `json:"model" yaml:"model"`
	Level int    `json:"level" yaml:"level"`
}

// ControlEntry mirrors one disabled target in .ai/fleet-control.json (doc 11).
// Target is matched by exact string equality against a candidate's agent
// (whole-transport disable) or agent+"/"+model (single-model disable).
type ControlEntry struct {
	Target string `json:"target" yaml:"target"`
	Reason string `json:"reason" yaml:"reason"`
	By     string `json:"by" yaml:"by"`
	At     string `json:"at" yaml:"at"`
}

// ControlState is the runtime kill-switch snapshot (doc 11). It is request
// state, not policy: the caller supplies the control read observed at dispatch
// time on RouteRequest.Control.
type ControlState struct {
	Disabled  []ControlEntry `json:"disabled" yaml:"disabled"`
	UpdatedAt string         `json:"updated_at" yaml:"updated_at"`
}

// PolicyHashes carries pre-computed content hashes of the three policy files.
// The router copies them verbatim into the snapshot; it performs no hashing and
// no file I/O (purest core, matching risk.go / complexity_score.go).
type PolicyHashes struct {
	ConfigYAML  string `json:"config_yaml" yaml:"config_yaml"`
	RoutingYAML string `json:"routing_yaml" yaml:"routing_yaml"`
	AgentsYAML  string `json:"agents_yaml" yaml:"agents_yaml"`
}

// RouteRequest is the caller-supplied routing request: phase, resolved risk,
// pre-computed complexity band, incumbent review family (soft diversity hint),
// the runtime control state, and the failed candidates to exclude (reroute).
type RouteRequest struct {
	Phase           string       `json:"phase" yaml:"phase"`
	Risk            string       `json:"risk" yaml:"risk"`
	ComplexityBand  string       `json:"complexity_band" yaml:"complexity_band"`
	IncumbentFamily string       `json:"incumbent_family" yaml:"incumbent_family"`
	Control         ControlState `json:"control" yaml:"control"`
	Exclusions      []Candidate  `json:"exclusions" yaml:"exclusions"`
}

// RoutingMatch mirrors a routing.yaml rule match; an empty field is a wildcard.
type RoutingMatch struct {
	Phase string `json:"phase" yaml:"phase"`
	Risk  string `json:"risk" yaml:"risk"`
	Band  string `json:"band" yaml:"band"`
}

// RoutingRule mirrors one routing.yaml rule: a match plus the executor level and
// the policy-declared reroute budget (max_attempts). First matching rule wins.
type RoutingRule struct {
	Match         RoutingMatch `json:"match" yaml:"match"`
	ExecutorLevel int          `json:"executor_level" yaml:"executor_level"`
	MaxAttempts   int          `json:"max_attempts" yaml:"max_attempts"`
}

// LevelModels mirrors config.yaml levels[N] and defines candidate preference
// order: Primary, then Fallback, then each Secondary in slice order. Empty
// string fields are skipped so a legacy two-field level still enumerates.
type LevelModels struct {
	Primary   string   `json:"primary" yaml:"primary"`
	Fallback  string   `json:"fallback" yaml:"fallback"`
	Secondary []string `json:"secondary" yaml:"secondary"`
}

// ModelAvailability declares one available model on a transport and its review
// family (soft-diversity metadata).
type ModelAvailability struct {
	Model  string `json:"model" yaml:"model"`
	Family string `json:"family" yaml:"family"`
}

// TransportAvailability mirrors one agents.yaml transport: its agent id, whether
// it is active, and the models it can run.
type TransportAvailability struct {
	Agent  string              `json:"agent" yaml:"agent"`
	Active bool                `json:"active" yaml:"active"`
	Models []ModelAvailability `json:"models" yaml:"models"`
}

// RoutePolicy is the caller-supplied policy aggregate. Levels is a map indexed
// only by the resolved level int (never ranged). Transports is an ORDERED slice
// because Go map iteration is randomized and determinism requires a stable
// transport order supplied by the caller.
type RoutePolicy struct {
	Routing       []RoutingRule           `json:"routing" yaml:"routing"`
	Levels        map[int]LevelModels     `json:"levels" yaml:"levels"`
	Transports    []TransportAvailability `json:"transports" yaml:"transports"`
	Hashes        PolicyHashes            `json:"hashes" yaml:"hashes"`
	RouterVersion string                  `json:"router_version" yaml:"router_version"`
}

// InputsSnapshot is sufficient on its own to reconstruct a decision later: it
// carries the policy content hashes, the control snapshot, risk, band, phase,
// resolved level, exclusions, and the router version.
type InputsSnapshot struct {
	RouterVersion   string       `json:"router_version" yaml:"router_version"`
	Risk            string       `json:"risk" yaml:"risk"`
	ComplexityBand  string       `json:"complexity_band" yaml:"complexity_band"`
	Phase           string       `json:"phase" yaml:"phase"`
	Level           int          `json:"level" yaml:"level"`
	IncumbentFamily string       `json:"incumbent_family" yaml:"incumbent_family"`
	Exclusions      []Candidate  `json:"exclusions" yaml:"exclusions"`
	Control         ControlState `json:"control" yaml:"control"`
	Hashes          PolicyHashes `json:"hashes" yaml:"hashes"`
}

// RouteResult is the router output. On a pick Candidate is non-nil and Blocked
// is empty; on a legitimate dead-end Candidate is nil and Blocked is
// "no_viable_candidate" with Reasons. InputsSnapshot is always populated.
type RouteResult struct {
	Candidate            *Candidate     `json:"candidate,omitempty" yaml:"candidate,omitempty"`
	RerouteBudget        int            `json:"reroute_budget" yaml:"reroute_budget"`
	ReviewFamilyDegraded bool           `json:"review_family_degraded" yaml:"review_family_degraded"`
	Blocked              string         `json:"blocked,omitempty" yaml:"blocked,omitempty"`
	Reasons              []string       `json:"reasons,omitempty" yaml:"reasons,omitempty"`
	InputsSnapshot       InputsSnapshot `json:"inputs_snapshot" yaml:"inputs_snapshot"`
}

// Route composes a routing request with policy data into either a viable
// candidate or a structured no_viable_candidate block. It is a pure function:
// no file I/O, no YAML parsing, no hashing, no network, no map ranging for
// selection, and zero hardcoded model/agent names. It returns an error only for
// structurally malformed policy (no routing rules, no matching rule, or a
// resolved level absent from Levels); legitimate dead-ends return a nil error
// with Blocked set. It never panics.
func Route(req RouteRequest, policy RoutePolicy) (RouteResult, error) {
	if len(policy.Routing) == 0 {
		return RouteResult{}, fmt.Errorf("invalid policy: no routing rules")
	}

	rule, ok := resolveRule(policy.Routing, req)
	if !ok {
		return RouteResult{}, fmt.Errorf(
			"invalid policy: no routing rule matches phase=%q risk=%q band=%q",
			req.Phase, req.Risk, req.ComplexityBand)
	}
	level := rule.ExecutorLevel

	models, ok := policy.Levels[level]
	if !ok {
		return RouteResult{}, fmt.Errorf("invalid policy: resolved level %d absent from levels map", level)
	}

	version := policy.RouterVersion
	if version == "" {
		version = RouterVersion
	}
	snapshot := InputsSnapshot{
		RouterVersion:   version,
		Risk:            req.Risk,
		ComplexityBand:  req.ComplexityBand,
		Phase:           req.Phase,
		Level:           level,
		IncumbentFamily: req.IncumbentFamily,
		Exclusions:      req.Exclusions,
		Control:         req.Control,
		Hashes:          policy.Hashes,
	}

	enumerated := enumerateCandidates(level, models, policy.Transports)

	var survivors []candidateWithFamily
	var reasons []string
	for _, c := range enumerated {
		if isDisabled(req.Control, c.cand) {
			reasons = append(reasons, fmt.Sprintf("disabled: %s/%s", c.cand.Agent, c.cand.Model))
			continue
		}
		if isExcluded(req.Exclusions, c.cand) {
			reasons = append(reasons, fmt.Sprintf("excluded: %s/%s", c.cand.Agent, c.cand.Model))
			continue
		}
		survivors = append(survivors, c)
	}

	if len(survivors) == 0 {
		if len(enumerated) == 0 {
			reasons = append(reasons, fmt.Sprintf("no active transport provides level %d models", level))
		}
		return RouteResult{
			Blocked:        "no_viable_candidate",
			Reasons:        reasons,
			InputsSnapshot: snapshot,
		}, nil
	}

	pick, degraded := preferDiverseFamily(survivors, req.IncumbentFamily)

	return RouteResult{
		Candidate:            &pick,
		RerouteBudget:        rule.MaxAttempts,
		ReviewFamilyDegraded: degraded,
		InputsSnapshot:       snapshot,
	}, nil
}

// candidateWithFamily pairs a candidate with the review family of its model, so
// the soft-diversity step can partition without re-looking-up availability.
type candidateWithFamily struct {
	cand   Candidate
	family string
}

// resolveRule returns the first rule whose non-empty match fields all equal the
// request (first-match-wins, mirroring routing.yaml). An empty match field is a
// wildcard.
func resolveRule(rules []RoutingRule, req RouteRequest) (RoutingRule, bool) {
	for _, r := range rules {
		if r.Match.Phase != "" && r.Match.Phase != req.Phase {
			continue
		}
		if r.Match.Risk != "" && r.Match.Risk != req.Risk {
			continue
		}
		if r.Match.Band != "" && r.Match.Band != req.ComplexityBand {
			continue
		}
		return r, true
	}
	return RoutingRule{}, false
}

// enumerateCandidates builds the deterministic ordered candidate list: preference
// order (primary, fallback, secondaries) as the outer loop, the ordered
// transport slice as the inner loop. A candidate is emitted when a transport is
// active and lists the model.
func enumerateCandidates(level int, models LevelModels, transports []TransportAvailability) []candidateWithFamily {
	ordered := make([]string, 0, 2+len(models.Secondary))
	if models.Primary != "" {
		ordered = append(ordered, models.Primary)
	}
	if models.Fallback != "" {
		ordered = append(ordered, models.Fallback)
	}
	for _, s := range models.Secondary {
		if s != "" {
			ordered = append(ordered, s)
		}
	}

	var out []candidateWithFamily
	for _, model := range ordered {
		for _, t := range transports {
			if !t.Active {
				continue
			}
			for _, m := range t.Models {
				if m.Model == model {
					out = append(out, candidateWithFamily{
						cand:   Candidate{Agent: t.Agent, Model: model, Level: level},
						family: m.Family,
					})
					break
				}
			}
		}
	}
	return out
}

// isDisabled reports whether the control state disables the candidate, matching
// Target against the whole agent or the exact agent+"/"+model identity (no
// slash-splitting of the provider/model canonical name).
func isDisabled(control ControlState, c Candidate) bool {
	full := c.Agent + "/" + c.Model
	for _, e := range control.Disabled {
		if e.Target == c.Agent || e.Target == full {
			return true
		}
	}
	return false
}

// isExcluded reports whether the candidate's exact {agent, model} pair is in the
// exclusion set (level is ignored so a reroute append matches regardless).
func isExcluded(exclusions []Candidate, c Candidate) bool {
	for _, e := range exclusions {
		if e.Agent == c.Agent && e.Model == c.Model {
			return true
		}
	}
	return false
}

// preferDiverseFamily is a soft preference, never a hard filter. When an
// incumbent family is given, survivors with a different family are preferred via
// a stable partition; otherwise the first survivor wins. degraded is true only
// when the pick shares the incumbent family (no diverse alternative existed).
func preferDiverseFamily(survivors []candidateWithFamily, incumbent string) (Candidate, bool) {
	if incumbent == "" {
		return survivors[0].cand, false
	}
	var diverse, same []candidateWithFamily
	for _, c := range survivors {
		if c.family != incumbent {
			diverse = append(diverse, c)
		} else {
			same = append(same, c)
		}
	}
	if len(diverse) > 0 {
		return diverse[0].cand, false
	}
	return same[0].cand, true
}
