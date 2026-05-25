package cmd

import (
	"fmt"
	"os"
	"quorum/internal/core"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status [task_id]",
	Short: "Show task status",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := core.ShowStatus(args[0]); err != nil {
			fmt.Println("[!] Error:", err)
			os.Exit(1)
		}
	},
}

func init() {
	taskCmd.AddCommand(statusCmd)
}
