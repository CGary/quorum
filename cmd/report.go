package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"quorum/internal/core"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var (
	reportSaveFile   string
	reportSaveDryRun bool
	reportNewOutput  string
	reportNewKind    string
	reportListJSON   bool
)

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

		svc := core.ReportService{ProjectRoot: projectRoot}
		opts := core.ReportNewOptions{
			OutputPath: reportNewOutput,
			Kind:       reportNewKind,
		}

		res, err := svc.NewReport(id, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		if res.IsScaffold {
			fmt.Printf("Created report scaffold: %s\n", res.Path)
		} else {
			fmt.Printf("Created report: %s\n", res.Path)
		}
	},
}

var reportSaveCmd = &cobra.Command{
	Use:   "save <id>",
	Short: "Validate a report from stdin or --file and persist it to .ai/reports/<id>.yaml",
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

		raw, err := readReportSaveInput(reportSaveFile, os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		svc := core.ReportService{ProjectRoot: projectRoot}
		opts := core.ReportSaveOptions{
			DryRun: reportSaveDryRun,
		}

		res, err := svc.SaveReport(id, raw, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		if res.DryRun {
			fmt.Printf("OK (dry-run): %s is valid (id, identity, and schema); not written\n", res.Path)
		} else {
			fmt.Printf("Saved report: %s\n", res.Path)
		}
	},
}

var reportListCmd = &cobra.Command{
	Use:   "list",
	Short: "List persisted report artifacts",
	Run: func(cmd *cobra.Command, args []string) {
		projectRoot, err := core.ProjectRoot()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error locating project root: %v\n", err)
			os.Exit(1)
		}

		svc := core.ReportService{ProjectRoot: projectRoot}
		reports, err := svc.ListReports()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error listing reports: %v\n", err)
			os.Exit(1)
		}

		if reportListJSON {
			raw, err := json.Marshal(reports)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error encoding reports: %v\n", err)
				os.Exit(1)
			}
			fmt.Print(string(raw))
			return
		}

		if len(reports) == 0 {
			fmt.Println("No reports found in .ai/reports/.")
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "id\tkind\ttitle\tdate")
		for _, report := range reports {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", report.ID, report.Kind, report.Title, report.Date)
		}
		w.Flush()
	},
}

func init() {
	reportNewCmd.Flags().StringVarP(&reportNewOutput, "output", "o", "", "Scaffold the draft to this path (e.g. .tmp/<id>.yaml) instead of .ai/reports/")
	reportNewCmd.Flags().StringVar(&reportNewKind, "kind", "", "Scaffold a specific kind of report (e.g. audit, refactor_plan)")
	reportSaveCmd.Flags().StringVar(&reportSaveFile, "file", "", "Read the report YAML from a file instead of stdin")
	reportSaveCmd.Flags().BoolVar(&reportSaveDryRun, "dry-run", false, "Validate the full write path (id, identity, schema) without persisting")
	reportListCmd.Flags().BoolVar(&reportListJSON, "json", false, "Print reports as a JSON array")
	reportCmd.AddCommand(reportNewCmd)
	reportCmd.AddCommand(reportSaveCmd)
	reportCmd.AddCommand(reportListCmd)
	rootCmd.AddCommand(reportCmd)
}
