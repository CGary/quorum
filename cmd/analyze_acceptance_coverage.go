package cmd

import (
	"fmt"
	"os"
	"quorum/internal/core"

	"github.com/spf13/cobra"
)

type AnalyzeAcceptanceCoverageRequest struct {
	SpecPath      string `json:"spec_path"`
	BlueprintPath string `json:"blueprint_path"`
}

var analyzeAcceptanceCoverageCmd = &cobra.Command{
	Use:   "acceptance-coverage",
	Short: "Analyze acceptance-to-test coverage",
	Args:  cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		var req AnalyzeAcceptanceCoverageRequest
		if err := readStdinJSON(&req); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		result := core.AnalyzeAcceptanceCoverage(req.SpecPath, req.BlueprintPath)

		if err := printStdoutJSON(result); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	analyzeCmd.AddCommand(analyzeAcceptanceCoverageCmd)
}
