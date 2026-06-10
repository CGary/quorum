package core

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// writeFixture writes content to a file under dir and returns its path.
func writeFixture(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func rowByID(rows []AcceptanceCoverageRow, id string) (AcceptanceCoverageRow, bool) {
	for _, r := range rows {
		if r.ItemID == id {
			return r, true
		}
	}
	return AcceptanceCoverageRow{}, false
}

func TestAcceptanceCoverage(t *testing.T) {
	useSchemas(t)

	const specCovered = "task_id: F-99\n" +
		"summary: Spec with structured acceptance\n" +
		"goal: Exercise acceptance coverage analysis.\n" +
		"invariants:\n  - Existing specs keep validating.\n" +
		"acceptance:\n" +
		"  - id: AC-1\n    statement: User can select CASH.\n" +
		"  - id: AC-2\n    statement: User can select QR.\n"

	const blueprintCovered = "task_id: F-99\n" +
		"summary: Blueprint covering both criteria\n" +
		"affected_files:\n  - src/a.go\n" +
		"symbols: []\n" +
		"dependencies: []\n" +
		"test_scenarios:\n" +
		"  - statement: Unit test for CASH selection.\n    covers: [AC-1]\n" +
		"  - statement: Unit test for QR selection.\n    covers: [AC-2]\n"

	t.Run("AC-1 uncovered criterion is a high gap", func(t *testing.T) {
		dir := t.TempDir()
		specPath := writeFixture(t, dir, "00-spec.yaml", specCovered)
		bp := "task_id: F-99\n" +
			"summary: Blueprint covering only AC-1\n" +
			"affected_files:\n  - src/a.go\n" +
			"symbols: []\n" +
			"dependencies: []\n" +
			"test_scenarios:\n" +
			"  - statement: Unit test for CASH selection.\n    covers: [AC-1]\n" +
			"  - Fast lint pass over the changed module.\n"
		bpPath := writeFixture(t, dir, "01-blueprint.yaml", bp)

		res := AnalyzeAcceptanceCoverage(specPath, bpPath)
		if res.Status != "issues_found" {
			t.Fatalf("status = %q, want issues_found", res.Status)
		}
		if len(res.Gaps) != 1 {
			t.Fatalf("gaps = %d, want 1: %#v", len(res.Gaps), res.Gaps)
		}
		if res.Gaps[0].Severity != "high" {
			t.Fatalf("gap severity = %q, want high", res.Gaps[0].Severity)
		}
		row, ok := rowByID(res.Coverage, "AC-2")
		if !ok || row.State != "gap" {
			t.Fatalf("AC-2 row = %#v, want state gap", row)
		}
		if covered, _ := rowByID(res.Coverage, "AC-1"); covered.State != "covered" {
			t.Fatalf("AC-1 state = %q, want covered", covered.State)
		}
	})

	t.Run("AC-2 dangling covers reference is a high finding", func(t *testing.T) {
		dir := t.TempDir()
		specPath := writeFixture(t, dir, "00-spec.yaml", specCovered)
		bp := "task_id: F-99\n" +
			"summary: Blueprint with a dangling covers id\n" +
			"affected_files:\n  - src/a.go\n" +
			"symbols: []\n" +
			"dependencies: []\n" +
			"test_scenarios:\n" +
			"  - statement: Unit test for CASH selection.\n    covers: [AC-1]\n" +
			"  - statement: Unit test for QR selection.\n    covers: [AC-2, AC-9]\n"
		bpPath := writeFixture(t, dir, "01-blueprint.yaml", bp)

		res := AnalyzeAcceptanceCoverage(specPath, bpPath)
		if res.Status != "issues_found" {
			t.Fatalf("status = %q, want issues_found", res.Status)
		}
		var dangling *Finding
		for i := range res.Findings {
			if res.Findings[i].Severity == "high" && strings.Contains(res.Findings[i].Issue, "AC-9") {
				dangling = &res.Findings[i]
			}
		}
		if dangling == nil {
			t.Fatalf("expected a high dangling finding for AC-9, got %#v", res.Findings)
		}
		if len(res.Gaps) != 0 {
			t.Fatalf("dangling ref should not create gaps, got %#v", res.Gaps)
		}
	})

	t.Run("AC-3 legacy string criteria are legacy_untracked, never gaps", func(t *testing.T) {
		dir := t.TempDir()
		spec := "task_id: F-99\n" +
			"summary: Spec mixing legacy and structured acceptance\n" +
			"goal: Exercise legacy untracked reporting.\n" +
			"invariants:\n  - Existing specs keep validating.\n" +
			"acceptance:\n" +
			"  - Plain legacy criterion without an id.\n" +
			"  - id: AC-1\n    statement: User can select CASH.\n"
		specPath := writeFixture(t, dir, "00-spec.yaml", spec)
		bp := "task_id: F-99\n" +
			"summary: Blueprint covering AC-1\n" +
			"affected_files:\n  - src/a.go\n" +
			"symbols: []\n" +
			"dependencies: []\n" +
			"test_scenarios:\n" +
			"  - statement: Unit test for CASH selection.\n    covers: [AC-1]\n"
		bpPath := writeFixture(t, dir, "01-blueprint.yaml", bp)

		res := AnalyzeAcceptanceCoverage(specPath, bpPath)
		if res.Status != "pass" {
			t.Fatalf("status = %q, want pass: %#v", res.Status, res)
		}
		if len(res.Gaps) != 0 {
			t.Fatalf("legacy criterion must not be a gap, got %#v", res.Gaps)
		}
		var legacy *AcceptanceCoverageRow
		for i := range res.Coverage {
			if res.Coverage[i].State == "legacy_untracked" {
				legacy = &res.Coverage[i]
			}
		}
		if legacy == nil {
			t.Fatalf("expected a legacy_untracked row, got %#v", res.Coverage)
		}
		if legacy.ItemID != "" {
			t.Fatalf("legacy row should have empty item_id, got %q", legacy.ItemID)
		}
	})

	t.Run("AC-4 duplicate acceptance ids yield blocked", func(t *testing.T) {
		dir := t.TempDir()
		spec := "task_id: F-99\n" +
			"summary: Spec with duplicate acceptance ids\n" +
			"goal: Exercise ambiguous coverage handling.\n" +
			"invariants:\n  - Existing specs keep validating.\n" +
			"acceptance:\n" +
			"  - id: AC-1\n    statement: First.\n" +
			"  - id: AC-1\n    statement: Duplicate id.\n"
		specPath := writeFixture(t, dir, "00-spec.yaml", spec)
		bpPath := writeFixture(t, dir, "01-blueprint.yaml", blueprintCovered)

		res := AnalyzeAcceptanceCoverage(specPath, bpPath)
		if res.Status != "blocked" {
			t.Fatalf("status = %q, want blocked", res.Status)
		}
		if len(res.Coverage) != 0 {
			t.Fatalf("blocked result must not compute coverage, got %#v", res.Coverage)
		}
	})

	t.Run("all covered yields pass", func(t *testing.T) {
		dir := t.TempDir()
		specPath := writeFixture(t, dir, "00-spec.yaml", specCovered)
		bpPath := writeFixture(t, dir, "01-blueprint.yaml", blueprintCovered)

		res := AnalyzeAcceptanceCoverage(specPath, bpPath)
		if res.Status != "pass" {
			t.Fatalf("status = %q, want pass: %#v", res.Status, res)
		}
		if len(res.Gaps) != 0 || len(res.Findings) != 0 {
			t.Fatalf("expected no gaps/findings, got gaps=%#v findings=%#v", res.Gaps, res.Findings)
		}
	})

	t.Run("AC-6 analysis is pure: no file writes", func(t *testing.T) {
		dir := t.TempDir()
		specPath := writeFixture(t, dir, "00-spec.yaml", specCovered)
		bpPath := writeFixture(t, dir, "01-blueprint.yaml", blueprintCovered)

		before := snapshotDir(t, dir)
		_ = AnalyzeAcceptanceCoverage(specPath, bpPath)
		after := snapshotDir(t, dir)

		if len(before) != len(after) {
			t.Fatalf("file set changed: before=%v after=%v", before, after)
		}
		for i := range before {
			if before[i] != after[i] {
				t.Fatalf("file changed: before=%q after=%q", before[i], after[i])
			}
		}
	})
}

// snapshotDir returns a sorted list of "name|size|mtime" entries for files in dir.
func snapshotDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, info.Name()+"|"+strconv.FormatInt(info.Size(), 10)+"|"+info.ModTime().String())
	}
	sort.Strings(out)
	return out
}
