package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"quorum/internal/core"
	"strings"

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
			_ = ensureReportsGitignoreEntry(root)
		}
	},
}

func ensureReportsGitignoreEntry(root string) error {
	giPath := filepath.Join(root, ".gitignore")
	b, err := os.ReadFile(giPath)
	if err != nil {
		return err
	}
	if strings.Contains(string(b), ".ai/reports/") {
		return nil
	}
	f, err := os.OpenFile(giPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if len(b) > 0 && b[len(b)-1] != '\n' {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}
	_, err = f.WriteString(".ai/reports/\n")
	return err
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
