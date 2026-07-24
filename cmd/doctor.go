package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"quorum/internal/core"
)

var doctorJSON bool

// doctorCmd is a top-level, strictly read-only consistency diagnosis across
// .ai/tasks state dirs, worktrees/, and ai/<ID> branches. It never writes,
// mutates, deletes, or repairs anything; it only reports divergences and
// exits non-zero when any are found.
var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose divergences across task dirs, worktrees, and ai/<ID> branches (read-only)",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		projectRoot, err := core.ProjectRoot()
		if err != nil {
			fmt.Fprintln(os.Stderr, "[!] Error resolving project root:", err)
			os.Exit(1)
		}
		if code := runDoctor(projectRoot, doctorJSON, os.Stdout, os.Stderr); code != 0 {
			os.Exit(code)
		}
	},
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorJSON, "json", false, "Output diagnosis as JSON")
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(projectRoot string, jsonOutput bool, stdout, stderr io.Writer) int {
	facts, err := core.CollectDoctorFacts(projectRoot)
	if err != nil {
		fmt.Fprintf(stderr, "[!] Error collecting doctor facts: %v\n", err)
		return 1
	}
	report := core.EvaluateDoctor(facts)

	if jsonOutput {
		b, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "[!] Error marshaling JSON: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, string(b))
	} else {
		renderDoctorReport(stdout, report)
	}

	if !report.OK {
		return 1
	}
	return 0
}

func renderDoctorReport(w io.Writer, report core.DoctorReport) {
	if report.OK {
		fmt.Fprintln(w, "[ok] project state is consistent: task dirs, worktrees, and ai/<ID> branches all match.")
		return
	}
	fmt.Fprintf(w, "[!] %d divergence(s) found:\n", len(report.Findings))
	for _, f := range report.Findings {
		fmt.Fprintf(w, "  - [%s] %s\n", f.Check, f.Message)
	}
}
