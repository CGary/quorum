package cmd

import (
	"fmt"
	"os"
	"quorum/internal/core"

	"github.com/spf13/cobra"
)

var feedbackConsumeCmd = &cobra.Command{
	Use:   "feedback-consume [task_id]",
	Short: "Consume task feedback",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		taskID := args[0]
		taskDir, err := core.FindTaskDir(taskID, nil)
		if err != nil {
			fmt.Println("[!] Error:", err)
			os.Exit(1)
		}
		if taskDir == nil {
			fmt.Printf("[!] Task %s not found.\n", taskID)
			os.Exit(1)
		}
		
		consumed, err := core.ConsumeFeedback(taskDir.Path)
		if err != nil {
			fmt.Println("[!] Error:", err)
			os.Exit(1)
		}
		if consumed {
			fmt.Println("[+] Consumed feedback.json")
		} else {
			fmt.Println("[-] No feedback.json found")
		}
	},
}

func init() {
	taskCmd.AddCommand(feedbackConsumeCmd)
}
