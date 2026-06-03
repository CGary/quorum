package cmd

import (
	"fmt"
	"os"
	"quorum/internal/core"

	"github.com/spf13/cobra"
)

type AnalyzeFailureLookupRequest struct {
	Blueprint core.Blueprint `json:"blueprint"`
	TasksDir  string         `json:"tasks_dir,omitempty"`
}

var analyzeFailureLookupCmd = &cobra.Command{
	Use:   "failure-lookup",
	Short: "Find related failed tasks for a blueprint",
	Args:  cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		var req AnalyzeFailureLookupRequest
		if err := readStdinJSON(&req); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		if req.TasksDir == "" {
			req.TasksDir = ".ai/tasks"
		}

		results, err := core.FindRelatedFailedTasks(req.Blueprint, req.TasksDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		if err := printStdoutJSON(results); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	analyzeCmd.AddCommand(analyzeFailureLookupCmd)
}
