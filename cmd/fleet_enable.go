package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"quorum/internal/core"
)

var fleetEnableCmd = &cobra.Command{
	Use:   "enable <target>",
	Short: "Enable an agent or model for routing",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectRoot, err := core.ProjectRoot()
		if err != nil {
			fmt.Fprintln(os.Stderr, "[!] Error resolving project root:", err)
			os.Exit(1)
		}
		if code := runFleetEnable(projectRoot, args[0], os.Stdout, os.Stderr); code != 0 {
			os.Exit(code)
		}
	},
}

func init() {
	fleetCmd.AddCommand(fleetEnableCmd)
}

func runFleetEnable(projectRoot, target string, stdout, stderr io.Writer) int {
	_, err := core.EnableFleetTarget(projectRoot, target)
	if err != nil {
		fmt.Fprintf(stderr, "[!] Error enabling target: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "[+] Enabled %s\n", target)
	return 0
}
