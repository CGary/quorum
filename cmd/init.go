package cmd

import (
	"quorum/internal/core"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize Quorum in the current project",
	Run: func(cmd *cobra.Command, args []string) {
		core.InitializeProject()
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
