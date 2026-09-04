package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"quorum/internal/core"
)

// catalogSentinelModel is a value that cannot be a real model name, used to
// force the transport's own "unknown model" rejection path without sending a
// real prompt to any model. Never used for comparison logic against real model
// names — it is only the probe value injected into {model_arg}.
const catalogSentinelModel = "quorum-catalog-probe-nonexistent"

// catalogProbeTimeoutS is the default probe timeout in seconds.
const catalogProbeTimeoutS = 30

var fleetCatalogJSON bool
var fleetCatalogTimeoutS int

var fleetCatalogCmd = &cobra.Command{
	Use:   "catalog <transport>",
	Short: "Compare a transport's live model catalog against agents.yaml declared models (read-only, manual-only)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectRoot, err := core.ProjectRoot()
		if err != nil {
			fmt.Fprintln(os.Stderr, "[!] Error resolving project root:", err)
			os.Exit(1)
		}
		if code := runFleetCatalog(projectRoot, args[0], fleetCatalogTimeoutS, fleetCatalogJSON, os.Stdout, os.Stderr); code != 0 {
			os.Exit(code)
		}
	},
}

func init() {
	fleetCatalogCmd.Flags().BoolVar(&fleetCatalogJSON, "json", false, "Output catalog delta as JSON")
	fleetCatalogCmd.Flags().IntVar(&fleetCatalogTimeoutS, "timeout", catalogProbeTimeoutS, "Probe timeout in seconds")
	fleetCmd.AddCommand(fleetCatalogCmd)
}

// catalogResult is the JSON envelope emitted by runFleetCatalog.
type catalogResult struct {
	Transport string `json:"transport"`
	// Status is one of "ok", "unknown", or "unsupported".
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	// Delta is populated only when Status is "ok".
	Delta *core.CatalogDelta `json:"delta,omitempty"`
}

// runFleetCatalog is the testable core of `quorum fleet catalog`.
// It performs process execution only at this cmd-layer call site, satisfying
// the spec invariant that internal/core/fleet_catalog.go has zero exec/IO.
func runFleetCatalog(projectRoot, transport string, timeoutS int, jsonOutput bool, stdout, stderr io.Writer) int {
	if timeoutS <= 0 {
		timeoutS = catalogProbeTimeoutS
	}

	// Load the named transport from agents.yaml; reject unknown names.
	t, err := loadFleetTransport(projectRoot, transport)
	if err != nil {
		msg := fmt.Sprintf("unknown transport %q: %v", transport, err)
		fmt.Fprintf(stderr, "[!] %s\n", msg)
		return 1
	}

	// If the transport's argv_template carries no {model_arg} placeholder there
	// is no model-flag mechanism to inject the sentinel; report "unsupported"
	// and exit 0 per spec invariant (non-goal: opencode/aider/codex probing).
	if !containsToken(t.ArgvTemplate, "{model_arg}") {
		emit(catalogResult{
			Transport: transport,
			Status:    "unsupported",
			Message:   "transport argv_template has no {model_arg} placeholder; live catalog probing is unsupported for this transport",
		}, jsonOutput, stdout)
		return 0
	}

	// Build vars for argv substitution: inject sentinel as model_arg, use
	// project root as cwd (safe per blueprint risk note: agy_edit probe never
	// reaches the sandbox step when the sentinel is rejected first).
	vars := map[string]string{
		"model_arg":        catalogSentinelModel,
		"cwd":              projectRoot,
		"prompt":           "catalog probe: this line is never reached because the sentinel model is rejected first",
		"reasoning_effort": "",
		"print_timeout":    formatPrintTimeout(timeoutS),
	}
	argv := substituteFleetArgv(t.ArgvTemplate, vars)

	// Execute the transport in the project root. Exec happens ONLY here in the
	// cmd layer — internal/core/fleet_catalog.go has no exec/IO (verified by
	// the '! grep -n "os/exec" internal/core/fleet_catalog.go' verify step).
	res := core.RunDelegate(core.RunDelegateInput{
		Binary:            t.Binary,
		Argv:              argv,
		Cwd:               projectRoot,
		StdinPrompt:       "",
		TimeoutS:          timeoutS,
		FailureSignatures: t.FailureSignatures,
		OutputFormat:      t.OutputFormat,
	})

	// Combine stdout+stderr (RunDelegate already merges them into Output).
	output := res.Output

	// Feed combined output into ParseAvailableModels. Status classification
	// NEVER uses the process exit code (blueprint risk note: agy always exits
	// 0 even on the unknown-model path; exit code is unreliable here).
	names, ok := core.ParseAvailableModels(output)
	if !ok {
		// Output did not contain a parseable "Available models:" block.
		emit(catalogResult{
			Transport: transport,
			Status:    "unknown",
			Message:   "probe output did not contain a parseable 'Available models:' block; cannot determine live catalog",
		}, jsonOutput, stdout)
		return 0
	}

	// Build the declared map: canonical key -> model_arg display name.
	declared := make(map[string]string, len(t.Models))
	for key, modelEntry := range t.Models {
		if displayName := stringField(modelEntry, "model_arg"); displayName != "" {
			declared[key] = displayName
		}
	}

	delta := core.DiffCatalog(declared, names)

	emit(catalogResult{
		Transport: transport,
		Status:    "ok",
		Delta:     &delta,
	}, jsonOutput, stdout)
	return 0
}

// emit writes the catalogResult to stdout either as JSON or human-readable text.
func emit(r catalogResult, jsonOutput bool, stdout io.Writer) {
	if jsonOutput {
		b, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			fmt.Fprintf(stdout, "{\"error\": %q}\n", err.Error())
			return
		}
		fmt.Fprintln(stdout, string(b))
		return
	}
	// Human-readable output.
	fmt.Fprintf(stdout, "transport: %s\nstatus:    %s\n", r.Transport, r.Status)
	if r.Message != "" {
		fmt.Fprintf(stdout, "message:   %s\n", r.Message)
	}
	if r.Delta != nil {
		if len(r.Delta.DeclaredDead) == 0 {
			fmt.Fprintln(stdout, "declared_dead: (none)")
		} else {
			fmt.Fprintln(stdout, "declared_dead:")
			for _, key := range r.Delta.DeclaredDead {
				fmt.Fprintf(stdout, "  - %s\n", key)
			}
		}
		if len(r.Delta.LiveUndeclared) == 0 {
			fmt.Fprintln(stdout, "live_undeclared: (none)")
		} else {
			fmt.Fprintln(stdout, "live_undeclared:")
			for _, name := range r.Delta.LiveUndeclared {
				fmt.Fprintf(stdout, "  - %s\n", strings.TrimSpace(name))
			}
		}
	}
}
