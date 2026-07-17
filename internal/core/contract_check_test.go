package core

import (
	"strings"
	"testing"
)

func intPtr(v int) *int { return &v }

// containsAll reports whether s contains every substring in subs, used to
// assert violation Detail strings name expected fragments without pinning
// to exact wording.
func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

func TestCheckContract(t *testing.T) {
	t.Run("AC-1 touch violation for uncovered file", func(t *testing.T) {
		c := Contract{Touch: []string{"internal/core/risk.go"}}
		res := CheckContract(c, []string{"internal/core/other.go"}, DiffStat{})
		if res.Ok {
			t.Fatalf("expected ok=false, got true")
		}
		if len(res.Violations) != 1 || res.Violations[0].Type != "touch" || res.Violations[0].File != "internal/core/other.go" {
			t.Fatalf("expected a touch violation for internal/core/other.go, got %+v", res.Violations)
		}
	})

	t.Run("AC-2 touch glob match does not violate", func(t *testing.T) {
		c := Contract{Touch: []string{"internal/core/*.go"}}
		res := CheckContract(c, []string{"internal/core/contract_check.go"}, DiffStat{})
		for _, v := range res.Violations {
			if v.Type == "touch" {
				t.Fatalf("did not expect a touch violation, got %+v", v)
			}
		}
	})

	t.Run("AC-3 new file not in touch is a violation", func(t *testing.T) {
		c := Contract{Touch: []string{"internal/core/risk.go"}}
		res := CheckContract(c, []string{"internal/core/brand_new_file.go"}, DiffStat{})
		if res.Ok {
			t.Fatalf("expected ok=false for new file outside touch")
		}
		found := false
		for _, v := range res.Violations {
			if v.Type == "touch" && v.File == "internal/core/brand_new_file.go" {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected touch violation for new file, got %+v", res.Violations)
		}
	})

	t.Run("AC-4 forbid.files always violates even if touch matches", func(t *testing.T) {
		c := Contract{
			Touch:  []string{"quorum.md"},
			Forbid: ContractForbid{Files: []string{"quorum.md"}},
		}
		res := CheckContract(c, []string{"quorum.md"}, DiffStat{})
		if res.Ok {
			t.Fatalf("expected ok=false, got true")
		}
		found := false
		for _, v := range res.Violations {
			if v.Type == "forbid_files" && v.File == "quorum.md" {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected forbid_files violation for quorum.md, got %+v", res.Violations)
		}
	})

	t.Run("AC-5 max_diff_lines exceeded", func(t *testing.T) {
		c := Contract{
			Touch:  []string{"internal/core/risk.go"},
			Limits: &ContractLimits{MaxDiffLines: intPtr(10), MaxFilesChanged: intPtr(5)},
		}
		res := CheckContract(c, []string{"internal/core/risk.go"}, DiffStat{Insertions: 8, Deletions: 5})
		found := false
		for _, v := range res.Violations {
			if v.Type == "max_diff_lines" {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected max_diff_lines violation, got %+v", res.Violations)
		}
	})

	t.Run("AC-5 max_files_changed exceeded", func(t *testing.T) {
		c := Contract{
			Touch:  []string{"a.go", "b.go", "c.go"},
			Limits: &ContractLimits{MaxDiffLines: intPtr(600), MaxFilesChanged: intPtr(2)},
		}
		res := CheckContract(c, []string{"a.go", "b.go", "c.go"}, DiffStat{Insertions: 1})
		found := false
		for _, v := range res.Violations {
			if v.Type == "max_files_changed" {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected max_files_changed violation, got %+v", res.Violations)
		}
	})

	t.Run("AC-6 no limits block skips rules 3 and 4 without error", func(t *testing.T) {
		c := Contract{Touch: []string{"a.go", "b.go", "c.go"}}
		res := CheckContract(c, []string{"a.go", "b.go", "c.go"}, DiffStat{Insertions: 9999, Deletions: 9999})
		for _, v := range res.Violations {
			if v.Type == "max_diff_lines" || v.Type == "max_files_changed" {
				t.Fatalf("did not expect limits violations without a limits block, got %+v", v)
			}
		}
	})

	t.Run("AC-7 forbid.behaviors always in not_checked", func(t *testing.T) {
		cases := []Contract{
			{Touch: []string{"a.go"}},
			{Touch: []string{"a.go"}, Forbid: ContractForbid{Files: []string{"a.go"}}},
			{Touch: []string{"a.go"}, Limits: &ContractLimits{MaxDiffLines: intPtr(1), MaxFilesChanged: intPtr(1)}},
		}
		for _, c := range cases {
			res := CheckContract(c, []string{"a.go"}, DiffStat{Insertions: 100})
			if len(res.NotChecked) != 1 || res.NotChecked[0] != "forbid.behaviors" {
				t.Fatalf("expected not_checked=[forbid.behaviors], got %+v", res.NotChecked)
			}
		}
	})

	t.Run("AC-9 max_files_changed equality passes, limit+1 fails", func(t *testing.T) {
		c := Contract{
			Touch:  []string{"a.go", "b.go", "c.go"},
			Limits: &ContractLimits{MaxDiffLines: intPtr(600), MaxFilesChanged: intPtr(3)},
		}
		atLimit := CheckContract(c, []string{"a.go", "b.go", "c.go"}, DiffStat{Insertions: 1})
		for _, v := range atLimit.Violations {
			if v.Type == "max_files_changed" {
				t.Fatalf("expected no max_files_changed violation at exactly the limit, got %+v", v)
			}
		}

		overLimitTouch := Contract{
			Touch:  []string{"a.go", "b.go", "c.go", "d.go"},
			Limits: &ContractLimits{MaxDiffLines: intPtr(600), MaxFilesChanged: intPtr(3)},
		}
		overLimit := CheckContract(overLimitTouch, []string{"a.go", "b.go", "c.go", "d.go"}, DiffStat{Insertions: 1})
		found := false
		for _, v := range overLimit.Violations {
			if v.Type == "max_files_changed" {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected max_files_changed violation at limit+1, got %+v", overLimit.Violations)
		}
	})

	t.Run("AC-9 max_diff_lines equality passes, limit+1 fails", func(t *testing.T) {
		c := Contract{
			Touch:  []string{"internal/core/risk.go"},
			Limits: &ContractLimits{MaxDiffLines: intPtr(10), MaxFilesChanged: intPtr(5)},
		}
		atLimit := CheckContract(c, []string{"internal/core/risk.go"}, DiffStat{Insertions: 6, Deletions: 4})
		for _, v := range atLimit.Violations {
			if v.Type == "max_diff_lines" {
				t.Fatalf("expected no max_diff_lines violation at exactly the limit, got %+v", v)
			}
		}

		overLimit := CheckContract(c, []string{"internal/core/risk.go"}, DiffStat{Insertions: 6, Deletions: 5})
		found := false
		for _, v := range overLimit.Violations {
			if v.Type == "max_diff_lines" {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected max_diff_lines violation at limit+1, got %+v", overLimit.Violations)
		}
	})

	t.Run("AC-8 fully compliant reports ok=true with empty violations", func(t *testing.T) {
		c := Contract{
			Touch:  []string{"internal/core/*.go"},
			Forbid: ContractForbid{Files: []string{"quorum.md"}},
			Limits: &ContractLimits{MaxDiffLines: intPtr(600), MaxFilesChanged: intPtr(5)},
		}
		res := CheckContract(c, []string{"internal/core/contract_check.go", "internal/core/contract_check_test.go"}, DiffStat{Insertions: 100, Deletions: 20})
		if !res.Ok {
			t.Fatalf("expected ok=true, got violations=%+v", res.Violations)
		}
		if len(res.Violations) != 0 {
			t.Fatalf("expected empty violations, got %+v", res.Violations)
		}
	})
}

// TestCheckContractPerClass covers FEAT-014's optional limits.per_class
// evaluation: table-driven for the per-file cases (AC-1/2/3/4/6), plus two
// standalone cases for AC-5 (legacy byte-identity) and the documented
// safeGlobMatch depth limitation.
func TestCheckContractPerClass(t *testing.T) {
	cases := []struct {
		name         string
		limits       *ContractLimits
		changedFiles []string
		diffStat     DiffStat
		fileDiffs    []FileDiff
		wantOk       bool
		wantPerClass bool
		wantDetail   []string
	}{
		{
			name:         "AC-1 matched file checked against class budget, not global",
			limits:       &ContractLimits{MaxDiffLines: intPtr(10), PerClass: []PerClassLimit{{Glob: "*_test.go", MaxDiffLines: 300}}},
			changedFiles: []string{"internal/core/contract_check_test.go"},
			diffStat:     DiffStat{Insertions: 200, Deletions: 50},
			fileDiffs:    []FileDiff{{Path: "internal/core/contract_check_test.go", Insertions: 200, Deletions: 50}},
			wantOk:       false, // pre-existing aggregate check still fails: 250 > 10
			wantPerClass: false, // but 250 <= 300 test budget, so no per-class violation
		},
		{
			name:         "AC-2 first declared per_class match wins",
			limits:       &ContractLimits{MaxDiffLines: intPtr(600), PerClass: []PerClassLimit{{Glob: "*_test.go", MaxDiffLines: 5}, {Glob: "contract_check_test.go", MaxDiffLines: 300}}},
			changedFiles: []string{"internal/core/contract_check_test.go"},
			diffStat:     DiffStat{Insertions: 50, Deletions: 10},
			fileDiffs:    []FileDiff{{Path: "internal/core/contract_check_test.go", Insertions: 50, Deletions: 10}},
			wantOk:       false,
			wantPerClass: true,
			wantDetail:   []string{"*_test.go"},
		},
		{
			name:         "AC-3 unmatched file falls back to global aggregate check",
			limits:       &ContractLimits{MaxDiffLines: intPtr(600), PerClass: []PerClassLimit{{Glob: "*_test.go", MaxDiffLines: 5}}},
			changedFiles: []string{"internal/core/contract_check.go"},
			diffStat:     DiffStat{Insertions: 50, Deletions: 10},
			fileDiffs:    []FileDiff{{Path: "internal/core/contract_check.go", Insertions: 50, Deletions: 10}},
			wantOk:       true,
			wantPerClass: false,
		},
		{
			name:         "AC-4 missing file_diffs degrades to global-limit-only, no error",
			limits:       &ContractLimits{MaxDiffLines: intPtr(600), PerClass: []PerClassLimit{{Glob: "*_test.go", MaxDiffLines: 5}}},
			changedFiles: []string{"internal/core/contract_check_test.go"},
			diffStat:     DiffStat{Insertions: 50, Deletions: 10},
			fileDiffs:    nil,
			wantOk:       true,
			wantPerClass: false,
		},
		{
			name:         "AC-6 violation names glob and measured-vs-budgeted lines",
			limits:       &ContractLimits{MaxDiffLines: intPtr(600), PerClass: []PerClassLimit{{Glob: "*.go", MaxDiffLines: 20}}},
			changedFiles: []string{"src/prod.go"},
			diffStat:     DiffStat{Insertions: 15, Deletions: 10},
			fileDiffs:    []FileDiff{{Path: "src/prod.go", Insertions: 15, Deletions: 10}},
			wantOk:       false,
			wantPerClass: true,
			wantDetail:   []string{"*.go", "25", "20"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := Contract{Touch: []string{"**"}, Limits: tc.limits}
			res := CheckContract(c, tc.changedFiles, tc.diffStat, tc.fileDiffs...)
			if res.Ok != tc.wantOk {
				t.Fatalf("Ok=%v, want %v (violations=%+v)", res.Ok, tc.wantOk, res.Violations)
			}
			var pc *ContractViolation
			for i := range res.Violations {
				if res.Violations[i].Type == "max_diff_lines_per_class" {
					pc = &res.Violations[i]
				}
			}
			if tc.wantPerClass != (pc != nil) {
				t.Fatalf("max_diff_lines_per_class present=%v, want %v (violations=%+v)", pc != nil, tc.wantPerClass, res.Violations)
			}
			if pc != nil {
				if pc.Rule != "limits.per_class" {
					t.Fatalf("expected rule=limits.per_class, got %q", pc.Rule)
				}
				if !containsAll(pc.Detail, tc.wantDetail...) {
					t.Fatalf("expected detail to contain %v, got %q", tc.wantDetail, pc.Detail)
				}
			}
		})
	}

	t.Run("AC-5 legacy contract without per_class is byte-identical regardless of file_diffs", func(t *testing.T) {
		c := Contract{Touch: []string{"**"}, Limits: &ContractLimits{MaxDiffLines: intPtr(10), MaxFilesChanged: intPtr(5)}}
		fd := []FileDiff{{Path: "internal/core/contract_check.go", Insertions: 8, Deletions: 5}}
		with := CheckContract(c, []string{"internal/core/contract_check.go"}, DiffStat{Insertions: 8, Deletions: 5}, fd...)
		without := CheckContract(c, []string{"internal/core/contract_check.go"}, DiffStat{Insertions: 8, Deletions: 5})
		if with.Ok != without.Ok || len(with.Violations) != len(without.Violations) {
			t.Fatalf("expected identical results, got %+v vs %+v", with, without)
		}
	})

	t.Run("safeGlobMatch depth limitation: leading **/X and bare *_test.go match at depth, trailing dir/** does not", func(t *testing.T) {
		perClass := func(glob string, budget int) *ContractLimits {
			return &ContractLimits{MaxDiffLines: intPtr(600), PerClass: []PerClassLimit{{Glob: glob, MaxDiffLines: budget}}}
		}
		fd := []FileDiff{{Path: "a/b/c/deep_test.go", Insertions: 4, Deletions: 4}}
		hasPerClass := func(res ContractCheckResult) bool {
			for _, v := range res.Violations {
				if v.Type == "max_diff_lines_per_class" {
					return true
				}
			}
			return false
		}

		leading := CheckContract(Contract{Touch: []string{"**"}, Limits: perClass("**/deep_test.go", 5)}, []string{"a/b/c/deep_test.go"}, DiffStat{}, fd...)
		if !hasPerClass(leading) {
			t.Fatalf("expected leading **/X glob to match at depth, got %+v", leading.Violations)
		}

		bare := CheckContract(Contract{Touch: []string{"**"}, Limits: perClass("*_test.go", 5)}, []string{"a/b/c/deep_test.go"}, DiffStat{}, fd...)
		if !hasPerClass(bare) {
			t.Fatalf("expected bare *_test.go glob to match at depth via filepath.Base, got %+v", bare.Violations)
		}

		trailing := CheckContract(Contract{Touch: []string{"**"}, Limits: perClass("a/**", 1)}, []string{"a/b/c/deep_test.go"}, DiffStat{}, fd...)
		if hasPerClass(trailing) {
			t.Fatalf("did not expect trailing dir/** to match beyond one directory level, got %+v", trailing.Violations)
		}
	})
}
