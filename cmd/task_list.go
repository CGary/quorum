package cmd

import (
    "fmt"
    "github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
    Use:   "list",
    Short: "List tasks with summaries",
    Args:  cobra.ExactArgs(0),
    Run: func(cmd *cobra.Command, args []string) {
        fmt.Println("list stub")
    },
}

func init() {
    taskCmd.AddCommand(listCmd)
}
