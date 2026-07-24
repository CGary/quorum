package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"quorum/internal/core"
)

var fleetStatsFlags core.StatsFilter
var fleetStatsJSON bool

var fleetStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Aggregate fleet dispatch telemetry from terminal tasks",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		projectRoot, err := core.ProjectRoot()
		if err != nil {
			fmt.Fprintln(os.Stderr, "[!] Error resolving project root:", err)
			os.Exit(1)
		}
		if code := runFleetStats(projectRoot, fleetStatsFlags, fleetStatsJSON, os.Stdout, os.Stderr); code != 0 {
			os.Exit(code)
		}
	},
}

func init() {
	fleetStatsCmd.Flags().BoolVar(&fleetStatsJSON, "json", false, "Output stats as JSON")
	fleetStatsCmd.Flags().StringVar(&fleetStatsFlags.Agent, "agent", "", "Filter by agent")
	fleetStatsCmd.Flags().StringVar(&fleetStatsFlags.Model, "model", "", "Filter by model")
	fleetStatsCmd.Flags().StringVar(&fleetStatsFlags.Since, "since", "", "Filter by started_at >= YYYY-MM-DD or RFC3339")
	fleetStatsCmd.Flags().StringVar(&fleetStatsFlags.GroupBy, "group-by", core.FleetStatsGroupByCell, "Group by cell, level, or band")
	fleetCmd.AddCommand(fleetStatsCmd)
}

func runFleetStats(projectRoot string, filter core.StatsFilter, jsonOutput bool, stdout, stderr io.Writer) int {
	groupBy := strings.TrimSpace(filter.GroupBy)
	if groupBy == "" {
		groupBy = core.FleetStatsGroupByCell
	}
	if groupBy != core.FleetStatsGroupByCell && groupBy != core.FleetStatsGroupByLevel && groupBy != core.FleetStatsGroupByBand {
		fmt.Fprintf(stderr, "[!] invalid --group-by %q: expected cell, level, or band\n", filter.GroupBy)
		return 1
	}

	records, err := core.CollectDispatchRecords(projectRoot, filter)
	if err != nil {
		fmt.Fprintf(stderr, "[!] Error collecting fleet stats: %v\n", err)
		return 1
	}
	report := core.ComputeFleetStats(records, groupBy)

	if jsonOutput {
		b, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "[!] Error marshaling JSON: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, string(b))
		return 0
	}

	renderFleetStatsTable(stdout, report)
	return 0
}

func renderFleetStatsTable(w io.Writer, report core.FleetStatsReport) {
	if len(report.Groups) == 0 {
		fmt.Fprintln(w, "no fleet dispatch records found")
		return
	}
	// tabwriter aligns columns for human readability; --json remains the stable
	// machine-parseable envelope and is unaffected by this formatting.
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintf(tw, "GROUP\tN\tSUCCESS\tFAILURE\tREROUTE\tBLOCKED\tNOOP\tTIMEOUT\tSUCCESS_RATE\tP50_S\tP95_S\tUSAGE\n")
	for _, group := range report.Groups {
		usage := "unavailable"
		if group.UsageMetricsAvailable {
			usage = "available"
		}
		fmt.Fprintf(
			tw,
			"%s\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%.2f\t%s\t%s\t%s\n",
			group.Key,
			group.N,
			group.Success,
			group.Failure,
			group.Reroute,
			group.Blocked,
			group.Noop,
			group.Timeout,
			group.SuccessRate,
			formatFleetStatsFloat(group.DurationP50S),
			formatFleetStatsFloat(group.DurationP95S),
			usage,
		)
	}
	tw.Flush()
}

func formatFleetStatsFloat(value *float64) string {
	if value == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.2f", *value)
}
