package cmd

import (
	"fmt"
	"os"
	"quorum/internal/core"

	"github.com/spf13/cobra"
)

type AnalyzeDecompositionCoverageRequest struct {
	ParentSpecPath string `json:"parent_spec_path"`
	AiTasksRoot    string `json:"ai_tasks_root,omitempty"`
}

var analyzeDecompositionCoverageCmd = &cobra.Command{
	Use:   "decomposition-coverage",
	Short: "Analyze decomposition coverage",
	Run: func(cmd *cobra.Command, args []string) {
		var req AnalyzeDecompositionCoverageRequest
		if err := readStdinJSON(&req); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		if req.AiTasksRoot == "" {
			req.AiTasksRoot = ".ai/tasks"
		}

		result := core.AnalyzeParentChildCoverage(req.ParentSpecPath, req.AiTasksRoot)

		if err := printStdoutJSON(result); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	analyzeCmd.AddCommand(analyzeDecompositionCoverageCmd)
}
