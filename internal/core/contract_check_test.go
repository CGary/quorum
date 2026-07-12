package core

import "testing"

func intPtr(v int) *int { return &v }

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
