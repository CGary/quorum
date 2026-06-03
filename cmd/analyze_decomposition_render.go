package cmd

import (
	"fmt"
	"os"
	"quorum/internal/core"

	"github.com/spf13/cobra"
)

type AnalyzeDecompositionRenderRequest struct {
	Decomposition []any `json:"decomposition"`
}

type AnalyzeDecompositionRenderResponse struct {
	Ascii string `json:"ascii"`
}

var analyzeDecompositionRenderCmd = &cobra.Command{
	Use:   "decomposition-render",
	Short: "Render an ASCII DAG for a decomposition",
	Args:  cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		var req AnalyzeDecompositionRenderRequest
		if err := readStdinJSON(&req); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		ascii := core.RenderAsciiDag(req.Decomposition)
		resp := AnalyzeDecompositionRenderResponse{Ascii: ascii}

		if err := printStdoutJSON(resp); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	analyzeCmd.AddCommand(analyzeDecompositionRenderCmd)
}
