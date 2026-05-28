package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"quorum/internal/core"
)

var analyzeFeedbackPartitionCmd = &cobra.Command{
	Use:   "feedback-partition",
	Short: "Partition feedback findings from stdin JSON into mechanical and semantic",
	Run: func(cmd *cobra.Command, args []string) {
		var payload map[string]any
		if err := readStdinJSON(&payload); err != nil {
			fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
			os.Exit(1)
		}

		result := core.PartitionFeedbackFindings(payload)

		if err := printStdoutJSON(result); err != nil {
			fmt.Fprintf(os.Stderr, "Error printing JSON: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	analyzeCmd.AddCommand(analyzeFeedbackPartitionCmd)
}
