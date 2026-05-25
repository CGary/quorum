package cmd

import (
	"fmt"
	"os"
	"quorum/internal/core"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all tasks",
	Run: func(cmd *cobra.Command, args []string) {
		if err := core.ListTasks(); err != nil {
			fmt.Println("[!] Error:", err)
			os.Exit(1)
		}
	},
}

func init() {
	taskCmd.AddCommand(listCmd)
}
