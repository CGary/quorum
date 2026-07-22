package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"quorum/internal/core"
)

// fleetSmokeCmd is the LEVEL 2 manual-only smoke check (FLEET-007 AC-4): it
// performs ONE REAL core.Dispatch call against a transport and consumes real
// quota. It is reachable ONLY via explicit CLI invocation -- never wired into
// any cron entry, CI workflow, or q-* skill auto-transition (non_goal D8 in
// 00-spec.yaml). It reuses the same transport-loading and argv-substitution
// helpers as 'quorum fleet dispatch' (loadFleetTransport, substituteFleetArgv,
// stringField in cmd/fleet_dispatch.go) WITHOUT editing that file.
var fleetSmokeCmd = &cobra.Command{
	Use:   "smoke <agent> <task_id>",
	Short: "Manual-only level 2 smoke dispatch (consumes real quota; never automated)",
	Long: `quorum fleet smoke <agent> <task_id> runs a single REAL dispatch through the
named fleet transport (e.g. agy) against an existing task worktree, reusing
the same transport-loading path as 'quorum fleet dispatch'.

This is a LEVEL 2 (manual) check: it invokes a real CLI and a real model, so
it consumes real quota. It must NEVER be wired into CI, cron, or any q-*
skill; it is reachable only by an explicit human-invoked command.`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		agent, taskID := args[0], args[1]
		store, err := core.DefaultTaskStore()
		if err != nil {
			fmt.Println("[!] Error initializing task store:", err)
			os.Exit(1)
		}
		resultPath, err := runFleetSmoke(store, agent, taskID)
		if err != nil {
			fmt.Println("[!]", err)
			os.Exit(1)
		}
		fmt.Printf("[+] Smoke dispatch result: %s\n", resultPath)
	},
}

// fleetSmokePrompt is a small, fixed prompt: the smoke check exists to prove
// the transport wiring still works end-to-end, not to exercise a real task.
const fleetSmokePrompt = "This is a manual fleet smoke check. Reply with a short confirmation only; do not modify any files."

// runFleetSmoke performs one real dispatch, reusing the same
// transport-loading (loadFleetTransport) and argv-substitution
// (substituteFleetArgv) helpers cmd/fleet_dispatch.go already defines in this
// package, without editing that file.
func runFleetSmoke(store core.TaskStore, agent, taskID string) (string, error) {
	if agent == "" || taskID == "" {
		return "", fmt.Errorf("smoke dispatch requires agent and task_id")
	}
	taskDir, err := store.FindTask(taskID, "active")
	if err != nil {
		return "", err
	}
	if taskDir == nil {
		return "", fmt.Errorf("active task %s not found", taskID)
	}
	transport, err := loadFleetTransport(store.ProjectRoot, agent)
	if err != nil {
		return "", err
	}
	if !transport.Active {
		return "", fmt.Errorf("fleet transport %q is inactive (active:false in agents.yaml); not dispatchable", agent)
	}
	applyFleetTransportEnv(transport.Env)
	worktree := filepath.Join(store.ProjectRoot, "worktrees", taskID)
	if _, statErr := os.Stat(worktree); statErr != nil {
		return "", fmt.Errorf("worktree for %s not found (run quorum task start): %w", taskID, statErr)
	}
	model := smokeDefaultModel(transport.Models)
	if model == "" {
		return "", fmt.Errorf("fleet transport %q declares no models to smoke", agent)
	}
	timeoutS := transport.Timeouts.DefaultS
	dispatchID := fmt.Sprintf("smoke-%d", time.Now().UTC().UnixNano())
	dispatchDir := filepath.Join(taskDir.Path, "dispatch", dispatchID)
	vars := map[string]string{
		"worktree":         worktree,
		"cwd":              worktree,
		"prompt":           fleetSmokePrompt,
		"out":              filepath.Join(dispatchDir, "delegate-out.jsonl"),
		"model_arg":        stringField(transport.Models[model], "model_arg"),
		"reasoning_effort": stringField(transport.Models[model], "reasoning_effort"),
		"print_timeout":    formatPrintTimeout(timeoutS),
	}
	argv := substituteFleetArgv(transport.ArgvTemplate, vars)
	stdinPrompt := fleetSmokePrompt
	if containsToken(transport.ArgvTemplate, "{prompt_file}") {
		aiderArgv, aerr := assembleAiderInvocation(taskDir.Path, dispatchDir, fleetSmokePrompt, vars, transport.ArgvTemplate)
		if aerr != nil {
			return "", aerr
		}
		argv = aiderArgv
		stdinPrompt = "" // aider has no stdin channel (input_channel: prompt_file)
	}
	if transport.StdinEmpty {
		stdinPrompt = ""
	}
	spec := core.DispatchSpec{
		TaskID: taskID, TaskDir: taskDir.Path, Agent: agent, Model: model,
		DispatchID: dispatchID, Worktree: worktree, Binary: transport.Binary,
		Argv: argv, StdinPrompt: stdinPrompt,
		TimeoutS: timeoutS, FailureSignatures: transport.FailureSignatures, OutputFormat: transport.OutputFormat,
	}
	if _, err := core.Dispatch(spec); err != nil {
		return "", err
	}
	if containsToken(transport.ArgvTemplate, "{prompt_file}") {
		checkAiderCostGuard(dispatchDir, transport.Models[model])
	}
	return filepath.Join(dispatchDir, "result.json"), nil
}

// smokeDefaultModel picks a deterministic model out of a transport's models
// map (Go map iteration order is randomized): the lexicographically smallest
// canonical model name.
func smokeDefaultModel(models map[string]map[string]any) string {
	if len(models) == 0 {
		return ""
	}
	names := make([]string, 0, len(models))
	for name := range models {
		names = append(names, name)
	}
	sort.Strings(names)
	return names[0]
}

func init() {
	fleetCmd.AddCommand(fleetSmokeCmd)
}
