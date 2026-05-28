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
		force, _ := cmd.Flags().GetBool("force")
		stash, _ := cmd.Flags().GetBool("stash")
		core.BackTask(args[0], force, stash)
	},
}

func init() {
	backCmd.Flags().Bool("force", false, "Discard uncommitted changes in the task worktree before removing it.")
	backCmd.Flags().Bool("stash", false, "Save uncommitted changes (including untracked) to worktrees/.stash/ before removing the worktree.")
	taskCmd.AddCommand(backCmd)
}
