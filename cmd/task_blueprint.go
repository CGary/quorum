package cmd

import (
	"fmt"
	"os"
	"quorum/internal/core"

	"github.com/spf13/cobra"
)

var blueprintCmd = &cobra.Command{
	Use:   "blueprint [task_id]",
	Short: "Generate technical blueprint",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("[*] Generating technical blueprint for %s...\n", args[0])
		_, err := core.PrepareBlueprint(args[0])
		if err != nil {
			fmt.Println("[!] Error:", err)
			os.Exit(1)
		}
		fmt.Println("[!] Please use the '/q-blueprint' skill to generate '01-blueprint.yaml' and '02-contract.yaml'.")
	},
}

func init() {
	taskCmd.AddCommand(blueprintCmd)
}
