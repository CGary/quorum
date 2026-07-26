package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalyzeRiskScore(t *testing.T) {
	bin := buildAnalyzeCLI(t)

	setupTempRepo := func(t *testing.T) string {
		dir := t.TempDir()
		cmd := exec.Command("git", "init")
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatalf("failed to git init: %v", err)
		}
		return dir
	}

	t.Run("AC-1_NoPolicyInStdin_WithRiskYaml_HighRisk", func(t *testing.T) {
		dir := setupTempRepo(t)
		policyDir := filepath.Join(dir, ".agents", "policies")
		if err := os.MkdirAll(policyDir, 0755); err != nil {
			t.Fatal(err)
		}
		policyYAML := "sensitive_paths:\n  - \"**/*.schema.*\"\n"
		if err := os.WriteFile(filepath.Join(policyDir, "risk.yaml"), []byte(policyYAML), 0644); err != nil {
			t.Fatal(err)
		}

		stdin := `{"blueprint": {"affected_files": ["db/user.schema.sql"]}}`
		out, err := runAnalyzeCmdErr(t, dir, bin, stdin, "analyze", "risk-score")
		if err != nil {
			t.Fatalf("expected success, got err=%v out=%q", err, out)
		}

		var resp AnalyzeRiskScoreResponse
		if err := json.Unmarshal([]byte(out), &resp); err != nil {
			t.Fatal(err)
		}

		if resp.RiskLevel != "high" {
			t.Errorf("expected risk level high, got %q", resp.RiskLevel)
		}
		if len(resp.Reasons) == 0 || !strings.Contains(resp.Reasons[0], "sensitive_path_match") {
			t.Errorf("expected sensitive_path_match in reasons, got %v", resp.Reasons)
		}
	})

	t.Run("AC-2_ExplicitPolicyInStdinOverridesRiskYaml", func(t *testing.T) {
		dir := setupTempRepo(t)
		policyDir := filepath.Join(dir, ".agents", "policies")
		if err := os.MkdirAll(policyDir, 0755); err != nil {
			t.Fatal(err)
		}
		policyYAML := "sensitive_paths:\n  - \"**/*.schema.*\"\n"
		if err := os.WriteFile(filepath.Join(policyDir, "risk.yaml"), []byte(policyYAML), 0644); err != nil {
			t.Fatal(err)
		}

		// Stdin provides empty explicit policy
		stdin := `{"blueprint": {"affected_files": ["db/user.schema.sql"]}, "policy": {"sensitive_paths": []}}`
		out, err := runAnalyzeCmdErr(t, dir, bin, stdin, "analyze", "risk-score")
		if err != nil {
			t.Fatalf("expected success, got err=%v out=%q", err, out)
		}

		var resp AnalyzeRiskScoreResponse
		if err := json.Unmarshal([]byte(out), &resp); err != nil {
			t.Fatal(err)
		}

		if resp.RiskLevel == "high" {
			t.Errorf("expected risk level not high, got %q", resp.RiskLevel)
		}
	})

	t.Run("AC-3_NoRiskYaml_NoPolicy_RunsLowRisk", func(t *testing.T) {
		dir := setupTempRepo(t)
		stdin := `{"blueprint": {"affected_files": ["db/user.schema.sql"]}}`
		out, err := runAnalyzeCmdErr(t, dir, bin, stdin, "analyze", "risk-score")
		if err != nil {
			t.Fatalf("expected success, got err=%v out=%q", err, out)
		}

		var resp AnalyzeRiskScoreResponse
		if err := json.Unmarshal([]byte(out), &resp); err != nil {
			t.Fatal(err)
		}

		if resp.RiskLevel != "low" {
			t.Errorf("expected risk level low, got %q", resp.RiskLevel)
		}
	})
}
