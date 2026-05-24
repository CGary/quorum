package cmd

import (
    "github.com/spf13/cobra"
)

var taskCmd = &cobra.Command{
    Use:   "task",
    Short: "Task management",
}

func init() {
    rootCmd.AddCommand(taskCmd)
}
