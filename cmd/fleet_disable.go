package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"quorum/internal/core"
)

var fleetDisableReason string

var fleetDisableCmd = &cobra.Command{
	Use:   "disable <target>",
	Short: "Disable an agent or model from routing",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectRoot, err := core.ProjectRoot()
		if err != nil {
			fmt.Fprintln(os.Stderr, "[!] Error resolving project root:", err)
			os.Exit(1)
		}
		if code := runFleetDisable(projectRoot, args[0], fleetDisableReason, os.Stdout, os.Stderr); code != 0 {
			os.Exit(code)
		}
	},
}

func init() {
	fleetDisableCmd.Flags().StringVar(&fleetDisableReason, "reason", "", "Reason for disabling the target")
	fleetDisableCmd.MarkFlagRequired("reason")
	fleetCmd.AddCommand(fleetDisableCmd)
}

func runFleetDisable(projectRoot, target, reason string, stdout, stderr io.Writer) int {
	if strings.TrimSpace(reason) == "" {
		fmt.Fprintln(stderr, "[!] --reason is required and cannot be empty")
		return 1
	}

	_, err := core.DisableFleetTarget(projectRoot, target, reason, "human")
	if err != nil {
		fmt.Fprintf(stderr, "[!] Error disabling target: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "[+] Disabled %s\n", target)
	return 0
}
