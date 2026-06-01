package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"quorum/internal/core"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Manage report artifacts",
}

var reportNewCmd = &cobra.Command{
	Use:   "new <id>",
	Short: "Create a new report artifact from template",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		projectRoot, err := core.ProjectRoot()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error locating project root: %v\n", err)
			os.Exit(1)
		}

		// Load or initialize config
		config, err := core.ReadQuorumConfigFrom(projectRoot)
		if config == nil || err != nil {
			config = &core.QuorumConfig{ProjectID: filepath.Base(projectRoot), ProjectName: filepath.Base(projectRoot)}
		}
		
		db, err := core.OpenMemoryDB("")
		if err != nil {
			fmt.Fprintf(os.Stderr, "error opening memory db: %v\n", err)
			os.Exit(1)
		}
		defer db.Close()
		
		remote := core.GitRemote(projectRoot)
		if err := core.EnsureMemoryProject(db, config, projectRoot, remote); err != nil {
			fmt.Fprintf(os.Stderr, "error registering project in memory: %v\n", err)
			os.Exit(1)
		}

		reportsDir := filepath.Join(projectRoot, ".ai", "reports")
		if err := os.MkdirAll(reportsDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "error creating reports directory: %v\n", err)
			os.Exit(1)
		}

		reportPath := filepath.Join(reportsDir, fmt.Sprintf("%s.yaml", id))
		if _, err := os.Stat(reportPath); err == nil {
			fmt.Fprintf(os.Stderr, "error: report file %s already exists\n", reportPath)
			os.Exit(1)
		}

		tmplPath := filepath.Join(projectRoot, ".agents", "templates", "report.yaml")
		tmplData, err := os.ReadFile(tmplPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading template: %v\n", err)
			os.Exit(1)
		}

		var payload map[string]any
		if err := yaml.Unmarshal(tmplData, &payload); err != nil {
			fmt.Fprintf(os.Stderr, "error parsing template: %v\n", err)
			os.Exit(1)
		}
		
		if meta, ok := payload["meta"].(map[string]any); ok {
			meta["id"] = id
			meta["date"] = time.Now().UTC().Format(time.RFC3339)
		}

		outData, err := yaml.Marshal(payload)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error serializing report: %v\n", err)
			os.Exit(1)
		}

		if err := os.WriteFile(reportPath, outData, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "error writing report: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Created report: %s\n", reportPath)
	},
}

func init() {
	reportCmd.AddCommand(reportNewCmd)
	rootCmd.AddCommand(reportCmd)
}
