package cmd

import (
    "fmt"
    "github.com/spf13/cobra"
)

var specifyCmd = &cobra.Command{
    Use:   "specify [task_id]",
    Short: "Initialize specification session",
    Args:  cobra.ExactArgs(1),
    Run: func(cmd *cobra.Command, args []string) {
        fmt.Println("specify stub", args[0])
    },
}

func init() {
    taskCmd.AddCommand(specifyCmd)
}
