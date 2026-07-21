package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"quorum/internal/core"
)

var fleetStatusJSON bool

var fleetStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current disabled targets",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		projectRoot, err := core.ProjectRoot()
		if err != nil {
			fmt.Fprintln(os.Stderr, "[!] Error resolving project root:", err)
			os.Exit(1)
		}
		if code := runFleetStatus(projectRoot, fleetStatusJSON, os.Stdout, os.Stderr); code != 0 {
			os.Exit(code)
		}
	},
}

func init() {
	fleetStatusCmd.Flags().BoolVar(&fleetStatusJSON, "json", false, "Output status as JSON")
	fleetCmd.AddCommand(fleetStatusCmd)
}

func runFleetStatus(projectRoot string, jsonOutput bool, stdout, stderr io.Writer) int {
	state, err := core.LoadFleetControlState(projectRoot)
	if err != nil {
		fmt.Fprintf(stderr, "[!] Error loading fleet control state: %v\n", err)
		return 1
	}

	report := core.BuildFleetStatusReport(state, time.Now().UTC())

	if jsonOutput {
		b, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "[!] Error marshaling JSON: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, string(b))
		return 0
	}

	if len(report.Disabled) == 0 {
		fmt.Fprintln(stdout, "no targets disabled")
		return 0
	}

	for _, e := range report.Disabled {
		age := time.Duration(e.AgeSeconds * float64(time.Second)).Round(time.Second)
		fmt.Fprintf(stdout, "Target: %s | Reason: %s | By: %s | Age: %s\n", e.Target, e.Reason, e.By, age)
	}
	return 0
}
