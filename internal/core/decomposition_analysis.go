package core

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Finding struct {
	Severity string `json:"severity"`
	Artifact string `json:"artifact"`
	Issue    string `json:"issue"`
}

type CoverageRow struct {
	Item      string   `json:"item"`
	CoveredBy []string `json:"covered_by"`
}

type Coverage struct {
	Invariants []CoverageRow `json:"invariants"`
	Acceptance []CoverageRow `json:"acceptance"`
}

type ChildStatus struct {
	Status    string   `json:"status"`
	Path      *string  `json:"path"`
	Error     *string  `json:"error"`
	DependsOn []string `json:"depends_on"`
}

type DecompositionAnalysisResult struct {
	Applies         bool                   `json:"applies"`
	Status          string                 `json:"status"`
	ParentTaskID    *string                `json:"parent_task_id"`
	ParentSpecPath  *string                `json:"parent_spec_path"`
	Children        map[string]ChildStatus `json:"children"`
	Coverage        Coverage               `json:"coverage"`
	Gaps            []Finding              `json:"gaps"`
	Inconsistencies []Finding              `json:"inconsistencies"`
	Findings        []Finding              `json:"findings"`
}

func AnalyzeParentChildCoverage(parentSpecPath, aiTasksRoot string) DecompositionAnalysisResult {
	res := DecompositionAnalysisResult{
		Applies:         false,
		Status:          "not_decomposed",
		Children:        make(map[string]ChildStatus),
		Gaps:            []Finding{},
		Inconsistencies: []Finding{},
		Findings:        []Finding{},
		Coverage: Coverage{
			Invariants: []CoverageRow{},
			Acceptance: []CoverageRow{},
		},
	}

	payload, err := LoadArtifactPayload(parentSpecPath)
	if err != nil {
		res.Status = "blocked"
		msg := "Parent spec could not be loaded: " + err.Error()
		res.Findings = append(res.Findings, Finding{"critical", filepath.ToSlash(parentSpecPath), msg})
		return res
	}

	parentSpec, ok := payload.(map[string]any)
	if !ok {
		res.Status = "blocked"
		msg := "Parent spec could not be loaded: spec root is not an object"
		res.Findings = append(res.Findings, Finding{"critical", filepath.ToSlash(parentSpecPath), msg})
		return res
	}

	// Validate parent (using core.ValidateArtifact)
	if err := ValidateArtifact(parentSpecPath, parentSpec); err != nil {
		var reason string
		if ve, ok := err.(ArtifactValidationError); ok {
			reason = ve.Message
		} else {
			reason = err.Error()
		}
		res.Status = "blocked"
		msg := "Parent spec could not be loaded: " + reason
		res.Findings = append(res.Findings, Finding{"critical", filepath.ToSlash(parentSpecPath), msg})
		return res
	}

	taskID := ""
	if id, ok := parentSpec["task_id"].(string); ok && id != "" {
		taskID = id
	} else {
		taskID = filepath.Base(filepath.Dir(parentSpecPath))
	}
	res.ParentTaskID = &taskID
	psp := filepath.ToSlash(parentSpecPath)
	res.ParentSpecPath = &psp

	decompObj := parentSpec["decomposition"]
	if decompObj == nil {
		return res
	}

	decompList, ok := asSlice(decompObj)
	if !ok {
		res.Applies = true
		res.Status = "blocked"
		res.Findings = append(res.Findings, Finding{"critical", "00-spec.yaml.decomposition", "Expected decomposition to be a list."})
		return res
	}

	res.Applies = true

	var entries []map[string]any
	var findings []Finding
	seen := make(map[string]bool)

	for i, entryAny := range decompList {
		artifact := fmt.Sprintf("00-spec.yaml.decomposition[%d]", i)
		entry, ok := entryAny.(map[string]any)
		if !ok {
			findings = append(findings, Finding{"high", artifact, "Expected decomposition entry to be an object."})
			continue
		}
		childID, ok := entry["child_id"].(string)
		if !ok || childID == "" {
			findings = append(findings, Finding{"high", artifact, "Missing child_id."})
			continue
		}
		if seen[childID] {
			findings = append(findings, Finding{"high", artifact, "Duplicate child_id " + childID + "."})
			continue
		}
		seen[childID] = true
		entries = append(entries, entry)
	}

	loadedChildren := make(map[string]map[string]any)

	for _, entry := range entries {
		childID := entry["child_id"].(string)
		var expected []string
		if depsAny, ok := asSlice(entry["depends_on"]); ok {
			for _, d := range depsAny {
				if ds, ok := d.(string); ok {
					expected = append(expected, ds)
				}
			}
		}

		childDirMatch, err := FindTaskDirIn(filepath.Dir(filepath.Dir(aiTasksRoot)), childID, []string{"inbox", "active", "done", "failed"})
		var childSpecPath string
		var resolverError string

		if err != nil {
			resolverError = err.Error()
		} else if childDirMatch == nil {
			resolverError = "Child " + childID + " is declared but no 00-spec.yaml was found."
		} else {
			childSpecPath = filepath.Join(childDirMatch.Path, "00-spec.yaml")
		}

		if resolverError != "" {
			res.Children[childID] = ChildStatus{
				Status:    "missing",
				Error:     &resolverError,
				DependsOn: expected,
			}
			findings = append(findings, Finding{"high", "00-spec.yaml.decomposition[" + childID + "]", resolverError})
			continue
		}

		csp := filepath.ToSlash(childSpecPath)
		childPayload, err := LoadArtifactPayload(childSpecPath)
		var loadError string
		if err != nil {
			loadError = err.Error()
		} else {
			if _, ok := childPayload.(map[string]any); !ok {
				loadError = "spec root is not an object"
			} else {
				if err := ValidateArtifact(childSpecPath, childPayload); err != nil {
					if ve, ok := err.(ArtifactValidationError); ok {
						loadError = ve.Message
					} else {
						loadError = err.Error()
					}
				}
			}
		}

		if loadError != "" {
			res.Children[childID] = ChildStatus{
				Status:    "invalid",
				Path:      &csp,
				Error:     &loadError,
				DependsOn: expected,
			}
			findings = append(findings, Finding{"high", csp, "Child " + childID + " has an invalid 00-spec.yaml: " + loadError})
			continue
		}

		childSpec := childPayload.(map[string]any)
		res.Children[childID] = ChildStatus{
			Status:    "loaded",
			Path:      &csp,
			DependsOn: expected,
		}
		loadedChildren[childID] = childSpec

		if pt, ok := childSpec["parent_task"].(string); !ok || pt != taskID {
			actualParent := ""
			if ok {
				actualParent = pt
			}
			findings = append(findings, Finding{"high", csp + ".parent_task", "Child " + childID + " references parent_task='" + actualParent + "'; expected '" + taskID + "'."})
		}

		var actual []string
		if depsAny, ok := asSlice(childSpec["depends_on"]); ok {
			for _, d := range depsAny {
				if ds, ok := d.(string); ok {
					actual = append(actual, ds)
				}
			}
		}

		var unknown []string
		for _, a := range actual {
			if !seen[a] {
				unknown = append(unknown, a)
			}
		}
		if len(unknown) > 0 {
			sort.Strings(unknown)
			findings = append(findings, Finding{"medium", csp + ".depends_on", "Child " + childID + " depends on undeclared sibling(s): " + strings.Join(unknown, ", ") + "."})
		}

		if !stringSlicesEqual(actual, expected) {
			aCopy := append([]string(nil), actual...)
			eCopy := append([]string(nil), expected...)
			sort.Strings(aCopy)
			sort.Strings(eCopy)
			aStr := "[]"
			if len(aCopy) > 0 {
				aStr = "['" + strings.Join(aCopy, "', '") + "']"
			}
			eStr := "[]"
			if len(eCopy) > 0 {
				eStr = "['" + strings.Join(eCopy, "', '") + "']"
			}
			findings = append(findings, Finding{"medium", csp + ".depends_on", "Child " + childID + " depends_on " + aStr + " does not match parent decomposition " + eStr + "."})
		}
	}

	for _, entry := range entries {
		childID := entry["child_id"].(string)
		if depsAny, ok := asSlice(entry["depends_on"]); ok {
			for _, d := range depsAny {
				if ds, ok := d.(string); ok && !seen[ds] {
					findings = append(findings, Finding{"medium", "00-spec.yaml.decomposition[" + childID + "].depends_on", "Child " + childID + " depends on undeclared sibling " + ds + "."})
				}
			}
		}
	}

	res.Coverage.Invariants = coverageForItems(parentSpec["invariants"], loadedChildren, "invariants")
	res.Coverage.Acceptance = coverageForItems(parentSpec["acceptance"], loadedChildren, "acceptance")

	for i, row := range res.Coverage.Invariants {
		if len(row.CoveredBy) == 0 {
			findings = append(findings, Finding{"medium", fmt.Sprintf("00-spec.yaml.invariants[%d]", i), "No child spec covers parent invariant: " + row.Item})
			res.Gaps = append(res.Gaps, findings[len(findings)-1])
		}
	}
	for i, row := range res.Coverage.Acceptance {
		if len(row.CoveredBy) == 0 {
			findings = append(findings, Finding{"high", fmt.Sprintf("00-spec.yaml.acceptance[%d]", i), "No child spec covers parent acceptance: " + row.Item})
			res.Gaps = append(res.Gaps, findings[len(findings)-1])
		}
	}

	res.Findings = findings
	for _, f := range findings {
		isGap := false
		for _, g := range res.Gaps {
			if f.Issue == g.Issue && f.Artifact == g.Artifact {
				isGap = true
				break
			}
		}
		if !isGap {
			res.Inconsistencies = append(res.Inconsistencies, f)
		}
	}

	if len(findings) > 0 {
		res.Status = "issues_found"
	} else {
		res.Status = "pass"
	}

	return res
}

func coverageForItems(parentItemsAny any, loadedChildren map[string]map[string]any, field string) []CoverageRow {
	var rows []CoverageRow
	parentItems, ok := asSlice(parentItemsAny)
	if !ok {
		return rows
	}

	childIDs := []string{}
	for id := range loadedChildren {
		childIDs = append(childIDs, id)
	}
	sort.Strings(childIDs)

	for _, piAny := range parentItems {
		pi := acceptanceStatement(piAny)
		var coveredBy []string
		for _, childID := range childIDs {
			childSpec := loadedChildren[childID]
			childItemsAny := childSpec[field]
			if childItems, ok := asSlice(childItemsAny); ok {
				covers := false
				for _, ciAny := range childItems {
					ci := acceptanceStatement(ciAny)
					if coversItem(pi, ci) {
						covers = true
						break
					}
				}
				if covers {
					coveredBy = append(coveredBy, childID)
				}
			}
		}
		rows = append(rows, CoverageRow{Item: pi, CoveredBy: coveredBy})
	}
	return rows
}

func coversItem(parentItem, childItem string) bool {
	pNorm := normStr(parentItem)
	cNorm := normStr(childItem)
	return pNorm != "" && (pNorm == cNorm || strings.Contains(cNorm, pNorm))
}

var nonWordRe = regexp.MustCompile(`\W+`)

func normStr(s string) string {
	return strings.TrimSpace(nonWordRe.ReplaceAllString(strings.ToLower(s), " "))
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aMap := make(map[string]bool)
	for _, s := range a {
		aMap[s] = true
	}
	for _, s := range b {
		if !aMap[s] {
			return false
		}
	}
	return true
}
