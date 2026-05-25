package cmd

import (
	"fmt"
	"os"
	"quorum/internal/core"

	"github.com/spf13/cobra"
)

var splitCmd = &cobra.Command{
	Use:   "split [task_id]",
	Short: "Materialize child tasks from decomposition",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("[*] Materialising children for %s...\n", args[0])
		if err := core.SplitTask(args[0]); err != nil {
			fmt.Println("[!] Error:", err)
			os.Exit(1)
		}
	},
}

func init() {
	taskCmd.AddCommand(splitCmd)
}
