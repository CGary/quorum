package cmd

import (
    "fmt"
    "github.com/spf13/cobra"
)

var artifactSaveCmd = &cobra.Command{
    Use:   "artifact-save [task_id] [artifact_path]",
    Short: "Persist a supported task artifact from stdin with schema validation",
    Args:  cobra.ExactArgs(2),
    Run: func(cmd *cobra.Command, args []string) {
        fmt.Println("artifact-save stub", args[0], args[1])
    },
}

func init() {
    taskCmd.AddCommand(artifactSaveCmd)
}
