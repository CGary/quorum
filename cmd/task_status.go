package cmd

import (
    "fmt"
    "github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
    Use:   "status [task_id]",
    Short: "Get task status",
    Args:  cobra.ExactArgs(1),
    Run: func(cmd *cobra.Command, args []string) {
        fmt.Println("status stub", args[0])
    },
}

func init() {
    taskCmd.AddCommand(statusCmd)
}
