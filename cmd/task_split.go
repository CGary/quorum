package cmd

import (
    "fmt"
    "github.com/spf13/cobra"
)

var splitCmd = &cobra.Command{
    Use:   "split [task_id]",
    Short: "Materialise child tasks from a parent's `decomposition` field (authored by /q-decompose)",
    Args:  cobra.ExactArgs(1),
    Run: func(cmd *cobra.Command, args []string) {
        fmt.Println("split stub", args[0])
    },
}

func init() {
    taskCmd.AddCommand(splitCmd)
}
