package cmd

import (
	"fmt"
	"os"
	"quorum/internal/core"

	"github.com/spf13/cobra"
)

type AnalyzeComplexityScoreRequest struct {
	Blueprint core.ComplexityBlueprint `json:"blueprint"`
	Policy    core.ComplexityPolicy    `json:"policy"`
}

var analyzeComplexityScoreCmd = &cobra.Command{
	Use:   "complexity-score",
	Short: "Score an advisory complexity band (S/M/L) based on blueprint and policy",
	Long: `Reads a JSON request from stdin containing a blueprint (affected_files, symbols,
and migration/public_api/schema_change flags) and a policy (S/M/L cut points from
.agents/policies/complexity.yaml). Emits an advisory complexity band plus the
signals that triggered it and the inputs used; never a bare numeric score.`,
	Args: cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		var req AnalyzeComplexityScoreRequest
		if err := readStdinJSON(&req); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		result, err := core.ScoreComplexity(req.Blueprint, req.Policy)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		if err := printStdoutJSON(result); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	analyzeCmd.AddCommand(analyzeComplexityScoreCmd)
}
