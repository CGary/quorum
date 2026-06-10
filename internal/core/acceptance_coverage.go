package core

import (
	"fmt"
	"path/filepath"
)

// AcceptanceCoverageRow is the per-criterion coverage view. It is intentionally
// a distinct struct from CoverageRow (decomposition): acceptance coverage is
// keyed by stable AC-id, not by a flattened statement string.
type AcceptanceCoverageRow struct {
	ItemID    string   `json:"item_id"`
	Statement string   `json:"statement"`
	CoveredBy []string `json:"covered_by"`
	State     string   `json:"state"`
}

type AcceptanceCoverageResult struct {
	Status        string                  `json:"status"`
	SpecPath      *string                 `json:"spec_path"`
	BlueprintPath *string                 `json:"blueprint_path"`
	Coverage      []AcceptanceCoverageRow `json:"coverage"`
	Gaps          []Finding               `json:"gaps"`
	Findings      []Finding               `json:"findings"`
}

// AnalyzeAcceptanceCoverage cross-checks 00-spec.yaml acceptance ids against
// 01-blueprint.yaml test_scenarios[].covers[]. It is pure and read-only: it
// loads and validates both artifacts, computes coverage on demand, and never
// writes files, mutates task state, or executes commands.
//
// Mirrors the decomposition-coverage pattern (AnalyzeParentChildCoverage) but
// matches by explicit AC-id rather than lexical containment.
func AnalyzeAcceptanceCoverage(specPath, blueprintPath string) AcceptanceCoverageResult {
	res := AcceptanceCoverageResult{
		Status:   "pass",
		Coverage: []AcceptanceCoverageRow{},
		Gaps:     []Finding{},
		Findings: []Finding{},
	}

	spec, ok := loadValidatedObject(specPath, &res)
	if !ok {
		return res
	}
	ssp := filepath.ToSlash(specPath)
	res.SpecPath = &ssp

	blueprint, ok := loadValidatedObject(blueprintPath, &res)
	if !ok {
		return res
	}
	bsp := filepath.ToSlash(blueprintPath)
	res.BlueprintPath = &bsp

	// Collect acceptance criteria. Object items carry a stable AC-id; plain
	// string items are legacy and explicitly untracked (never a gap).
	type criterion struct {
		id        string
		statement string
		legacy    bool
	}
	var criteria []criterion
	idSeen := make(map[string]int)
	acceptanceItems, _ := asSlice(spec["acceptance"])
	for _, item := range acceptanceItems {
		if id, ok := acceptanceID(item); ok {
			idSeen[id]++
			criteria = append(criteria, criterion{id: id, statement: acceptanceStatement(item)})
		} else {
			criteria = append(criteria, criterion{statement: acceptanceStatement(item), legacy: true})
		}
	}

	// Duplicate ids make coverage ambiguous: refuse to compute (AC-4). The doc
	// [1] high finding owns uniqueness enforcement; here we just decline.
	var dups []string
	for _, c := range criteria {
		if !c.legacy && idSeen[c.id] > 1 {
			dups = append(dups, c.id)
		}
	}
	if len(dups) > 0 {
		res.Status = "blocked"
		seen := map[string]bool{}
		for _, id := range dups {
			if seen[id] {
				continue
			}
			seen[id] = true
			res.Findings = append(res.Findings, Finding{"high", ssp + ".acceptance", "Duplicate acceptance id " + id + "; coverage is ambiguous."})
		}
		return res
	}

	validID := make(map[string]bool)
	for _, c := range criteria {
		if !c.legacy {
			validID[c.id] = true
		}
	}

	// Walk blueprint scenarios, mapping each covered id to the scenario
	// statements that cover it, and flagging dangling references (AC-2).
	coveredBy := make(map[string][]string)
	var findings []Finding
	scenarios, _ := asSlice(blueprint["test_scenarios"])
	for i, sc := range scenarios {
		stmt := acceptanceStatement(sc)
		coversAny, ok := lookupKey(sc, "covers")
		if !ok {
			continue
		}
		coversList, _ := asSlice(coversAny)
		for _, cv := range coversList {
			id, _ := cv.(string)
			if id == "" {
				continue
			}
			if !validID[id] {
				findings = append(findings, Finding{"high", fmt.Sprintf("01-blueprint.yaml.test_scenarios[%d].covers", i), "covers references unknown acceptance id " + id + "."})
				continue
			}
			coveredBy[id] = append(coveredBy[id], stmt)
		}
	}

	// Build coverage rows and gaps in spec order (deterministic output).
	var gaps []Finding
	for _, c := range criteria {
		if c.legacy {
			res.Coverage = append(res.Coverage, AcceptanceCoverageRow{
				ItemID:    "",
				Statement: c.statement,
				CoveredBy: []string{},
				State:     "legacy_untracked",
			})
			continue
		}
		cb := coveredBy[c.id]
		if cb == nil {
			cb = []string{}
		}
		row := AcceptanceCoverageRow{ItemID: c.id, Statement: c.statement, CoveredBy: cb}
		if len(cb) == 0 {
			row.State = "gap"
			gap := Finding{"high", fmt.Sprintf("01-blueprint.yaml.test_scenarios (covers %s)", c.id), fmt.Sprintf("No test scenario covers acceptance %s: %s", c.id, c.statement)}
			gaps = append(gaps, gap)
		} else {
			row.State = "covered"
		}
		res.Coverage = append(res.Coverage, row)
	}

	res.Gaps = append(res.Gaps, gaps...)
	res.Findings = append(res.Findings, findings...)
	res.Findings = append(res.Findings, gaps...)

	if len(res.Findings) > 0 {
		res.Status = "issues_found"
	}
	return res
}

// loadValidatedObject loads an artifact, validates it against its schema, and
// returns it as an object. On any failure it sets res.Status=blocked, appends a
// critical finding, and returns ok=false.
func loadValidatedObject(path string, res *AcceptanceCoverageResult) (map[string]any, bool) {
	payload, err := LoadArtifactPayload(path)
	if err != nil {
		res.Status = "blocked"
		res.Findings = append(res.Findings, Finding{"critical", filepath.ToSlash(path), "Artifact could not be loaded: " + err.Error()})
		return nil, false
	}
	obj, ok := payload.(map[string]any)
	if !ok {
		res.Status = "blocked"
		res.Findings = append(res.Findings, Finding{"critical", filepath.ToSlash(path), "Artifact root is not an object."})
		return nil, false
	}
	if err := ValidateArtifact(path, obj); err != nil {
		reason := err.Error()
		if ve, ok := err.(ArtifactValidationError); ok {
			reason = ve.Message
		}
		res.Status = "blocked"
		res.Findings = append(res.Findings, Finding{"critical", filepath.ToSlash(path), "Artifact failed schema validation: " + reason})
		return nil, false
	}
	return obj, true
}

// acceptanceID returns the stable AC-id of an object-form acceptance/test item.
// String (legacy) items have no id.
func acceptanceID(item any) (string, bool) {
	if v, ok := lookupKey(item, "id"); ok {
		if s, ok := v.(string); ok && s != "" {
			return s, true
		}
	}
	return "", false
}
