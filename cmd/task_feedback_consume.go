package cmd

import (
    "fmt"
    "github.com/spf13/cobra"
)

var feedbackConsumeCmd = &cobra.Command{
    Use:   "feedback-consume [task_id]",
    Short: "Remove feedback.json after mechanical q-analyze findings are applied",
    Args:  cobra.ExactArgs(1),
    Run: func(cmd *cobra.Command, args []string) {
        fmt.Println("feedback-consume stub", args[0])
    },
}

func init() {
    taskCmd.AddCommand(feedbackConsumeCmd)
}
