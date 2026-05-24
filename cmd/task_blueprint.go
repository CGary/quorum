package cmd

import (
    "fmt"
    "github.com/spf13/cobra"
)

var blueprintCmd = &cobra.Command{
    Use:   "blueprint [task_id]",
    Short: "Generate technical blueprint",
    Args:  cobra.ExactArgs(1),
    Run: func(cmd *cobra.Command, args []string) {
        fmt.Println("blueprint stub", args[0])
    },
}

func init() {
    taskCmd.AddCommand(blueprintCmd)
}
