package cmd

import (
	"fmt"
	"os"
	"quorum/internal/core"

	"github.com/spf13/cobra"
)

type AnalyzeBlueprintContextRequest struct {
	Blueprint   map[string]any `json:"blueprint"`
	ProjectRoot string         `json:"project_root,omitempty"`
	MaxHops     int            `json:"max_hops,omitempty"`
}

var analyzeBlueprintContextCmd = &cobra.Command{
	Use:   "blueprint-context",
	Short: "Enrich blueprint with retrievers",
	Args:  cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		var req AnalyzeBlueprintContextRequest
		if err := readStdinJSON(&req); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		if req.ProjectRoot == "" {
			req.ProjectRoot = "."
		}
		if req.MaxHops == 0 {
			req.MaxHops = 3
		}

		enriched := core.EnrichBlueprintWithRetrievers(req.Blueprint, req.ProjectRoot, req.MaxHops)
		if err := printStdoutJSON(enriched); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	analyzeCmd.AddCommand(analyzeBlueprintContextCmd)
}
