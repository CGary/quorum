package cmd

import (
	"quorum/internal/core"
	"github.com/spf13/cobra"
)

var cleanCmd = &cobra.Command{
	Use:   "clean [task_id]",
	Short: "Clean up a task",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		force, _ := cmd.Flags().GetBool("force")
		stash, _ := cmd.Flags().GetBool("stash")
		core.CleanTask(args[0], force, stash)
	},
}

func init() {
	cleanCmd.Flags().Bool("force", false, "Discard uncommitted changes in the task worktree before removing it.")
	cleanCmd.Flags().Bool("stash", false, "Save uncommitted changes (including untracked) to worktrees/.stash/ before removing the worktree.")
	taskCmd.AddCommand(cleanCmd)
}
