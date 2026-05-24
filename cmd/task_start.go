package cmd

import (
    "fmt"
    "github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
    Use:   "start [task_id]",
    Short: "Start a task",
    Args:  cobra.ExactArgs(1),
    Run: func(cmd *cobra.Command, args []string) {
        fmt.Println("start stub", args[0])
    },
}

func init() {
    taskCmd.AddCommand(startCmd)
}
