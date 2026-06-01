package cmd

import (
	"fmt"
	"os"
	"strings"
	"quorum/internal/core"

	"github.com/spf13/cobra"
)

var initProjectID string
var initProjectName string

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize Quorum in the current project",
	Run: func(cmd *cobra.Command, args []string) {
		opts := core.InitOptions{
			ProjectID:      initProjectID,
			ProjectName:    initProjectName,
			NonInteractive: !stdinIsTerminal(),
		}
		if err := core.InitializeProjectWithOptions(opts); err != nil {
			fmt.Fprintf(os.Stderr, "[!] %v\n", err)
			os.Exit(1)
		}
		
		if root, err := core.ProjectRoot(); err == nil {
			giPath := root + "/.gitignore"
			if b, err := os.ReadFile(giPath); err == nil {
				if !strings.Contains(string(b), ".ai/reports/") {
					f, _ := os.OpenFile(giPath, os.O_APPEND|os.O_WRONLY, 0644)
					f.WriteString(".ai/reports/\n")
					f.Close()
				}
			}
		}
	},
}

func init() {
	initCmd.Flags().StringVar(&initProjectID, "project-id", "", "Stable project id for centralized memory")
	initCmd.Flags().StringVar(&initProjectName, "project-name", "", "Human project name for centralized memory")
	rootCmd.AddCommand(initCmd)
}

func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
