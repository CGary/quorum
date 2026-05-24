package cmd

import (
    "fmt"
    "github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
    Use:   "init",
    Short: "Initialize Quorum in the current project",
    Run: func(cmd *cobra.Command, args []string) {
        fmt.Println("init stub")
    },
}

func init() {
    rootCmd.AddCommand(initCmd)
}
