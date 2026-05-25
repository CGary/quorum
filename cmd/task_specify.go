package cmd

import (
	"fmt"
	"os"
	"quorum/internal/core"

	"github.com/spf13/cobra"
)

var specifyCmd = &cobra.Command{
	Use:   "specify [task_id]",
	Short: "Initialize specification session",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		taskID := ""
		if len(args) > 0 {
			taskID = args[0]
		}
		fmt.Println("[*] Initializing specification session...")
		path, err := core.InitializeSpecify(taskID)
		if err != nil {
			fmt.Println("[!] Error:", err)
			os.Exit(1)
		}
		fmt.Printf("[+] Task directory created: %s\n", path)
		fmt.Println("[!] Please use the '/q-brief' skill to fill '00-spec.yaml'.")
	},
}

func init() {
	taskCmd.AddCommand(specifyCmd)
}
