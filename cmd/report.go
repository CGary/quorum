package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"quorum/internal/core"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	reportSaveFile      string
	reportSaveDryRun    bool
	reportNewOutput     string
	reportNewKind       string
)

// reportMetaDateLine matches the single active `date:` line inside the template's
// meta block (commented lines start with `#`, so they are not matched), letting
// the scaffold stamp a real timestamp while preserving the template's comments.
var reportMetaDateLine = regexp.MustCompile(`(?m)^(\s+)date:\s*".*"`)

// scaffoldReportTemplate stamps the agreed id and current timestamp into the raw
// template TEXT without parsing it, so the documented commented menu survives.
func scaffoldReportTemplate(tmpl []byte, id string) string {
	out := strings.Replace(string(tmpl), `id: "template-id"`, fmt.Sprintf(`id: %q`, id), 1)
	out = reportMetaDateLine.ReplaceAllString(out, fmt.Sprintf(`${1}date: %q`, time.Now().UTC().Format(time.RFC3339)))
	return out
}

// loadReportTemplate resolves the report template: on-disk first (so a project
// can customize it), then the bundle embedded in the binary. The embedded
// fallback makes `report new` work even in projects where `quorum init` never
// placed a template on disk.
func loadReportTemplate(projectRoot, kind string) ([]byte, error) {
	if kind == "" || kind == "generic" {
		onDisk := filepath.Join(projectRoot, ".agents", "templates", "report.yaml")
		if b, err := os.ReadFile(onDisk); err == nil {
			return b, nil
		}
		if b, ok := core.EmbeddedAgentFile("templates/report.yaml"); ok {
			return b, nil
		}
		return nil, fmt.Errorf("report template not found on disk (%s) or embedded in the binary", onDisk)
	}

	onDisk := filepath.Join(projectRoot, ".agents", "templates", "reports", kind+".yaml")
	if b, err := os.ReadFile(onDisk); err == nil {
		return b, nil
	}
	if b, ok := core.EmbeddedAgentFile("templates/reports/" + kind + ".yaml"); ok {
		return b, nil
	}
	return nil, fmt.Errorf("report template for kind %q not found on disk (%s) or embedded in the binary", kind, onDisk)
}

// fillReportMetadata stamps machine-set meta fields (schemaVersion, date) when
// the author omitted them, so a hand-written draft need only carry meta.id.
// Explicit values are preserved. Runs before validation.
func fillReportMetadata(payload any) {
	root, ok := payload.(map[string]any)
	if !ok {
		return
	}
	meta, ok := root["meta"].(map[string]any)
	if !ok {
		meta = map[string]any{}
		root["meta"] = meta
	}
	if s, _ := meta["schemaVersion"].(string); strings.TrimSpace(s) == "" {
		meta["schemaVersion"] = "1.1"
	}
	if d, _ := meta["date"].(string); strings.TrimSpace(d) == "" {
		meta["date"] = time.Now().UTC().Format(time.RFC3339)
	}
}

// readReportSaveInput reads the report payload from --file or stdin, mirroring
// readMemorySaveInput so the CLI exposes ONE convention for "input by file"
// across `memory save` and `report save`. Supplying both stdin and --file is
// rejected to keep the source of truth unambiguous.
func readReportSaveInput(filePath string, stdin *os.File) ([]byte, error) {
	if filePath != "" {
		if stdinHasData(stdin) {
			rawStdin, err := io.ReadAll(stdin)
			if err != nil {
				return nil, fmt.Errorf("failed to read stdin: %w", err)
			}
			if strings.TrimSpace(string(rawStdin)) != "" {
				return nil, fmt.Errorf("provide the report through stdin or --file, not both")
			}
		}
		raw, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read --file %s: %w", filePath, err)
		}
		if strings.TrimSpace(string(raw)) == "" {
			return nil, fmt.Errorf("report input is required")
		}
		return raw, nil
	}

	if !stdinHasData(stdin) {
		return nil, fmt.Errorf("report input is required; pipe YAML to stdin or use --file")
	}
	raw, err := io.ReadAll(stdin)
	if err != nil {
		return nil, fmt.Errorf("failed to read stdin: %w", err)
	}
	if strings.TrimSpace(string(raw)) == "" {
		return nil, fmt.Errorf("report input is required")
	}
	return raw, nil
}

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

		// --output scaffolds a draft to an arbitrary path (e.g. .tmp/<id>.yaml)
		// for staging, preserving the template's documented commented menu and
		// stamping meta.id/date. It does NOT register the project in memory and
		// does NOT touch .ai/reports/ — persistence still goes through
		// `quorum report save`.
		if reportNewOutput != "" {
			tmplData, err := loadReportTemplate(projectRoot, reportNewKind)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			scaffold := scaffoldReportTemplate(tmplData, id)

			var payload map[string]any
			if err := yaml.Unmarshal([]byte(scaffold), &payload); err != nil {
				fmt.Fprintf(os.Stderr, "error parsing scaffolded template: %v\n", err)
				os.Exit(1)
			}
			// Mirror `report save`: stamp machine-set metadata before validation so a
			// kind template that omits schemaVersion/date still scaffolds a valid draft.
			fillReportMetadata(payload)
			if err := core.ValidateAgainstSchema("report.schema.json", reportNewOutput, payload); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			if err := os.MkdirAll(filepath.Dir(reportNewOutput), 0755); err != nil {
				fmt.Fprintf(os.Stderr, "error creating output directory: %v\n", err)
				os.Exit(1)
			}
			if err := os.WriteFile(reportNewOutput, []byte(scaffold), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "error writing scaffold: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Created report scaffold: %s\n", reportNewOutput)
			return
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

		tmplData, err := loadReportTemplate(projectRoot, reportNewKind)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
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
	Short: "Validate a report from stdin or --file and persist it to .ai/reports/<id>.yaml",
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

		raw, err := readReportSaveInput(reportSaveFile, os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
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

		// Auto-fill machine-set metadata (schemaVersion, date) before validation,
		// so a directly-authored draft only needs meta.id.
		fillReportMetadata(payload)

		// Hard write-point invariant #2: meta.id must match the filename id.
		if err := core.CheckReportIDMatches(payload, id); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		// --dry-run runs the FULL write-path preflight (id regex + identity +
		// schema) without persisting, so a draft can be checked in one command
		// before committing it to disk.
		if reportSaveDryRun {
			if err := core.ValidateArtifact(reportPath, payload); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("OK (dry-run): %s is valid (id, identity, and schema); not written\n", reportPath)
			return
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
	reportNewCmd.Flags().StringVarP(&reportNewOutput, "output", "o", "", "Scaffold the draft to this path (e.g. .tmp/<id>.yaml) instead of .ai/reports/")
	reportNewCmd.Flags().StringVar(&reportNewKind, "kind", "", "Scaffold a specific kind of report (e.g. audit, refactor_plan)")
	reportSaveCmd.Flags().StringVar(&reportSaveFile, "file", "", "Read the report YAML from a file instead of stdin")
	reportSaveCmd.Flags().BoolVar(&reportSaveDryRun, "dry-run", false, "Validate the full write path (id, identity, schema) without persisting")
	reportCmd.AddCommand(reportNewCmd)
	reportCmd.AddCommand(reportSaveCmd)
	rootCmd.AddCommand(reportCmd)
}
