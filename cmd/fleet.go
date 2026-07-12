package cmd

import (
	"github.com/spf13/cobra"
)

// fleetCmd is the parent of the fleet command group: headless-delegate
// dispatch helpers. quorum fleet bundle is its first subcommand; the
// existing quorum analyze fleet-preflight stays under analyze (relocating it
// here is explicitly out of scope for FLEET-005).
var fleetCmd = &cobra.Command{
	Use:   "fleet",
	Short: "Fleet dispatch helpers for headless delegate execution",
}

func init() {
	rootCmd.AddCommand(fleetCmd)
}
