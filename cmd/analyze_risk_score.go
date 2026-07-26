package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"quorum/internal/core"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type AnalyzeRiskScoreRequest struct {
	Blueprint    core.Blueprint  `json:"blueprint"`
	Policy       core.RiskPolicy `json:"policy,omitempty"`
	DeclaredRisk string          `json:"declared_risk,omitempty"`
}

type AnalyzeRiskScoreResponse struct {
	RiskLevel   string                `json:"risk_level"`
	Reasons     []string              `json:"reasons"`
	Signals     core.RiskSignals      `json:"signals"`
	TraceEvents []core.RiskTraceEvent `json:"trace_events"`
}

var analyzeRiskScoreCmd = &cobra.Command{
	Use:   "risk-score",
	Short: "Score risk based on blueprint and policy",
	Args:  cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		var req AnalyzeRiskScoreRequest
		if err := readStdinJSON(&req); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		if req.Policy.SensitivePaths == nil {
			if root, err := core.ProjectRoot(); err == nil {
				req.Policy = loadDefaultRiskPolicy(root)
			}
		}

		result := core.AssignRiskLevel(req.Blueprint, req.Policy)
		traceEvents := core.BuildRiskTraceEvents(req.DeclaredRisk, result)

		resp := AnalyzeRiskScoreResponse{
			RiskLevel:   result.Level,
			Reasons:     result.Reasons,
			Signals:     result.Signals,
			TraceEvents: traceEvents,
		}

		if err := printStdoutJSON(resp); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	analyzeCmd.AddCommand(analyzeRiskScoreCmd)
}

func loadDefaultRiskPolicy(projectRoot string) core.RiskPolicy {
	filePath := filepath.Join(projectRoot, ".agents", "policies", "risk.yaml")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return core.RiskPolicy{}
	}
	var policy core.RiskPolicy
	if err := yaml.Unmarshal(data, &policy); err != nil {
		return core.RiskPolicy{}
	}
	return policy
}
