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
		save, _ := cmd.Flags().GetBool("save")
		core.CleanTask(args[0], force, save)
	},
}

func init() {
	cleanCmd.Flags().Bool("force", false, "Discard uncommitted changes in the task worktree before removing it.")
	cleanCmd.Flags().Bool("save", false, "Stash uncommitted changes (including untracked) before removing the worktree.")
	taskCmd.AddCommand(cleanCmd)
}
