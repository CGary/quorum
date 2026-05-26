package cmd

import (
	"quorum/internal/core"
	"github.com/spf13/cobra"
)

var backCmd = &cobra.Command{
	Use:   "back [task_id]",
	Short: "Revert a task to its previous state (worktree, then active->inbox, or done/failed->active)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		core.BackTask(args[0])
	},
}

func init() {
	taskCmd.AddCommand(backCmd)
}
