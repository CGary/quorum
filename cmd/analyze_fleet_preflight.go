package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"quorum/internal/core"
)

// fleetPreflightRequest carries agents.yaml, agents.schema.json, and the
// flattened config.yaml.levels model list as raw text/JSON fields; the
// caller owns all file I/O, this shim only forwards bytes to
// core.RunFleetPreflight.
type fleetPreflightRequest struct {
	AgentsYAML  string   `json:"agents_yaml"`
	SchemaJSON  string   `json:"schema_json"`
	LevelModels []string `json:"level_models"`
}

var analyzeFleetPreflightCmd = &cobra.Command{
	Use:   "fleet-preflight",
	Short: "Schema-validate agents.yaml and join config.yaml.levels models against its active transports from stdin JSON",
	Args:  cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		var req fleetPreflightRequest
		if err := readStdinJSON(&req); err != nil {
			fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
			os.Exit(1)
		}

		result, err := core.RunFleetPreflight([]byte(req.AgentsYAML), []byte(req.SchemaJSON), req.LevelModels)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error running fleet preflight: %v\n", err)
			os.Exit(1)
		}

		if err := printStdoutJSON(result); err != nil {
			fmt.Fprintf(os.Stderr, "Error printing JSON: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	analyzeCmd.AddCommand(analyzeFleetPreflightCmd)
}
