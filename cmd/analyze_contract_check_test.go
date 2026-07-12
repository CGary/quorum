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
