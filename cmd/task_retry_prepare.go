package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"quorum/internal/core"
)

var taskRetryPrepareCmd = &cobra.Command{
	Use:   "retry-prepare <TASK_ID>",
	Short: "Prepare a failed child task for retry",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		taskID := args[0]
		success := core.PrepareFailedChildRetry(taskID)
		if !success {
			fmt.Printf("[!] Failed to prepare task %s for retry.\n", taskID)
			os.Exit(1)
		}
	},
}

func init() {
	taskCmd.AddCommand(taskRetryPrepareCmd)
}
