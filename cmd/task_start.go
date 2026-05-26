package cmd

import (
	"quorum/internal/core"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start [task_id]",
	Short: "Start a task",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		core.StartTask(args[0])
	},
}

func init() {
	taskCmd.AddCommand(startCmd)
}
