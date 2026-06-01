package cmd

import (
	"fmt"
	"io"
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
		if err := core.ValidateReportID(id); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

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

		// Validate-before-write: the seeded payload must pass report.schema.json
		// before it reaches disk. This makes .agents/templates/report.yaml a seed
		// that must be valid by construction.
		if _, err := core.SaveArtifact(reportPath, payload); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Created report: %s\n", reportPath)
	},
}

var reportSaveCmd = &cobra.Command{
	Use:   "save <id>",
	Short: "Validate a report from stdin and persist it to .ai/reports/<id>.yaml",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]

		// Hard write-point invariant #1: the ID must match the canonical regex.
		if err := core.ValidateReportID(id); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		projectRoot, err := core.ProjectRoot()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error locating project root: %v\n", err)
			os.Exit(1)
		}

		raw, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading stdin: %v\n", err)
			os.Exit(1)
		}

		reportsDir := filepath.Join(projectRoot, ".ai", "reports")
		if err := os.MkdirAll(reportsDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "error creating reports directory: %v\n", err)
			os.Exit(1)
		}

		// Parse via a temp file so YAML and JSON inputs share the project's
		// canonical loader (mirrors `task artifact-save`).
		reportPath := filepath.Join(reportsDir, fmt.Sprintf("%s.yaml", id))
		tmpPath := reportPath + ".tmp"
		if err := os.WriteFile(tmpPath, raw, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		defer os.Remove(tmpPath)

		payload, err := core.LoadArtifactPayload(tmpPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: payload parse failed: %v\n", err)
			os.Exit(1)
		}

		// Hard write-point invariant #2: meta.id must match the filename id.
		if err := core.CheckReportIDMatches(payload, id); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		// Validate-before-write: schema validation runs inside SaveArtifact via the
		// dynamic reports/ matching before anything touches disk.
		if _, err := core.SaveArtifact(reportPath, payload); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Saved report: %s\n", reportPath)
	},
}

func init() {
	reportCmd.AddCommand(reportNewCmd)
	reportCmd.AddCommand(reportSaveCmd)
	rootCmd.AddCommand(reportCmd)
}
