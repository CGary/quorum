package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalyzeContractCheck(t *testing.T) {
	bin := buildAnalyzeCLI(t)
	dir := t.TempDir()

	contractYAML := `
touch:
  - internal/core/*.go
forbid:
  files:
    - quorum.md
  behaviors:
    - Do not do the bad thing.
limits:
  max_files_changed: 5
  max_diff_lines: 600
`
	contractPath := filepath.Join(dir, "02-contract.yaml")
	if err := os.WriteFile(contractPath, []byte(contractYAML), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("ValidStdinCompliant", func(t *testing.T) {
		stdin := `{"contract_path": "` + contractPath + `", "changed_files": ["internal/core/contract_check.go"], "diff_stat": {"insertions": 10, "deletions": 2}}`
		out, err := runAnalyzeCmdErr(t, dir, bin, stdin, "analyze", "contract-check")
		if err != nil {
			t.Fatalf("expected success, got err=%v out=%q", err, out)
		}

		var result struct {
			Ok         bool             `json:"ok"`
			Violations []map[string]any `json:"violations"`
			NotChecked []string         `json:"not_checked"`
		}
		if err := json.Unmarshal([]byte(out), &result); err != nil {
			t.Fatalf("failed to unmarshal stdout: %v, out=%q", err, out)
		}
		if !result.Ok {
			t.Errorf("expected ok=true, got violations=%v", result.Violations)
		}
		if len(result.NotChecked) != 1 || result.NotChecked[0] != "forbid.behaviors" {
			t.Errorf("expected not_checked=[forbid.behaviors], got %v", result.NotChecked)
		}
	})

	t.Run("ValidStdinViolation", func(t *testing.T) {
		stdin := `{"contract_path": "` + contractPath + `", "changed_files": ["quorum.md"], "diff_stat": {"insertions": 1, "deletions": 0}}`
		out, err := runAnalyzeCmdErr(t, dir, bin, stdin, "analyze", "contract-check")
		if err != nil {
			t.Fatalf("expected success (checker reports, does not fail the process), got err=%v out=%q", err, out)
		}

		var result struct {
			Ok         bool             `json:"ok"`
			Violations []map[string]any `json:"violations"`
		}
		if err := json.Unmarshal([]byte(out), &result); err != nil {
			t.Fatalf("failed to unmarshal stdout: %v, out=%q", err, out)
		}
		if result.Ok {
			t.Errorf("expected ok=false for forbidden file, got true")
		}
		if len(result.Violations) == 0 {
			t.Errorf("expected at least one violation")
		}
	})

	t.Run("EmptyStdin", func(t *testing.T) {
		out, err := runAnalyzeCmdErr(t, dir, bin, "", "analyze", "contract-check")
		if err == nil {
			t.Fatal("expected error for empty stdin, got success")
		}
		if !strings.Contains(out, "empty stdin") {
			t.Errorf("expected 'empty stdin' in output, got %q", out)
		}
	})

	t.Run("MalformedJSON", func(t *testing.T) {
		out, err := runAnalyzeCmdErr(t, dir, bin, `{"contract_path": [`, "analyze", "contract-check")
		if err == nil {
			t.Fatal("expected error for malformed JSON, got success")
		}
		if !strings.Contains(out, "unmarshal") {
			t.Errorf("expected unmarshal error in output, got %q", out)
		}
	})

	perClassContractYAML := `
touch:
  - internal/core/*.go
forbid:
  files:
    - quorum.md
  behaviors:
    - Do not do the bad thing.
limits:
  max_files_changed: 5
  max_diff_lines: 600
  per_class:
    - glob: "*_test.go"
      max_diff_lines: 300
    - glob: internal/core/contract_check.go
      max_diff_lines: 20
`
	perClassContractPath := filepath.Join(dir, "02-contract-per-class.yaml")
	if err := os.WriteFile(perClassContractPath, []byte(perClassContractYAML), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("FEAT-014 FileDiffsPresentAppliesPerClassBudget", func(t *testing.T) {
		stdin := `{"contract_path": "` + perClassContractPath + `", "changed_files": ["internal/core/contract_check.go"], "diff_stat": {"insertions": 15, "deletions": 10}, "file_diffs": [{"path": "internal/core/contract_check.go", "insertions": 15, "deletions": 10}]}`
		out, err := runAnalyzeCmdErr(t, dir, bin, stdin, "analyze", "contract-check")
		if err != nil {
			t.Fatalf("expected success, got err=%v out=%q", err, out)
		}

		var result struct {
			Ok         bool             `json:"ok"`
			Violations []map[string]any `json:"violations"`
		}
		if err := json.Unmarshal([]byte(out), &result); err != nil {
			t.Fatalf("failed to unmarshal stdout: %v, out=%q", err, out)
		}
		if result.Ok {
			t.Fatalf("expected ok=false: 25 lines exceeds the per_class budget of 20 for internal/core/contract_check.go, got %v", result.Violations)
		}
		found := false
		for _, v := range result.Violations {
			if v["type"] == "max_diff_lines_per_class" {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected a max_diff_lines_per_class violation, got %v", result.Violations)
		}
	})

	t.Run("FEAT-014 LegacyRequestWithoutFileDiffsDegradesGracefully", func(t *testing.T) {
		// Same per_class contract, but the stdin request omits file_diffs
		// entirely (the pre-existing shape) -- must not error and must not
		// attempt per-class evaluation (AC-4).
		stdin := `{"contract_path": "` + perClassContractPath + `", "changed_files": ["internal/core/contract_check.go"], "diff_stat": {"insertions": 15, "deletions": 10}}`
		out, err := runAnalyzeCmdErr(t, dir, bin, stdin, "analyze", "contract-check")
		if err != nil {
			t.Fatalf("expected success, got err=%v out=%q", err, out)
		}

		var result struct {
			Ok         bool             `json:"ok"`
			Violations []map[string]any `json:"violations"`
		}
		if err := json.Unmarshal([]byte(out), &result); err != nil {
			t.Fatalf("failed to unmarshal stdout: %v, out=%q", err, out)
		}
		if !result.Ok {
			t.Fatalf("expected ok=true (aggregate 25 <= 600, no per-class eval without file_diffs), got %v", result.Violations)
		}
		for _, v := range result.Violations {
			if v["type"] == "max_diff_lines_per_class" {
				t.Fatalf("did not expect per-class evaluation without file_diffs, got %v", result.Violations)
			}
		}
	})

	t.Run("MissingContractPath", func(t *testing.T) {
		stdin := `{"contract_path": "` + filepath.Join(dir, "does-not-exist.yaml") + `", "changed_files": [], "diff_stat": {}}`
		out, err := runAnalyzeCmdErr(t, dir, bin, stdin, "analyze", "contract-check")
		if err == nil {
			t.Fatal("expected error for missing contract file, got success")
		}
		if !strings.Contains(out, "Error reading contract_path") {
			t.Errorf("expected contract_path read error in output, got %q", out)
		}
	})
}
