package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"quorum/internal/core"
)

type fleetDispatchRequest struct {
	TaskID     string `json:"task_id"`
	Agent      string `json:"agent"`
	Model      string `json:"model"`
	BundlePath string `json:"bundle_path"`
	TimeoutS   int    `json:"timeout_s"`
	DispatchID string `json:"dispatch_id"`
}

type fleetTransport struct {
	Binary            string   `yaml:"binary"`
	ArgvTemplate      []string `yaml:"argv_template"`
	OutputFormat      string   `yaml:"output_format"`
	FailureSignatures []string `yaml:"failure_signatures"`
	Timeouts          struct {
		DefaultS int `yaml:"default_s"`
	} `yaml:"timeouts"`
	Models map[string]map[string]any `yaml:"models"`
	Active bool                      `yaml:"active"`
}

var fleetDispatchCmd = &cobra.Command{
	Use:   "dispatch",
	Short: "Run a headless delegate dispatch for an active task (reads JSON from stdin)",
	Run: func(cmd *cobra.Command, args []string) {
		raw, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Println("[!] Error reading stdin:", err)
			os.Exit(1)
		}
		var req fleetDispatchRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			fmt.Println("[!] Error parsing dispatch request:", err)
			os.Exit(1)
		}
		store, err := core.DefaultTaskStore()
		if err != nil {
			fmt.Println("[!] Error initializing task store:", err)
			os.Exit(1)
		}
		resultPath, err := runFleetDispatch(store, req)
		if err != nil {
			fmt.Println("[!]", err)
			os.Exit(1)
		}
		fmt.Printf("[+] Dispatch result: %s\n", resultPath)
	},
}

func runFleetDispatch(store core.TaskStore, req fleetDispatchRequest) (string, error) {
	if req.TaskID == "" || req.Agent == "" || req.Model == "" || req.DispatchID == "" {
		return "", fmt.Errorf("dispatch request requires task_id, agent, model, and dispatch_id")
	}
	taskDir, err := store.FindTask(req.TaskID, "active")
	if err != nil {
		return "", err
	}
	if taskDir == nil {
		return "", fmt.Errorf("active task %s not found", req.TaskID)
	}
	transport, err := loadFleetTransport(store.ProjectRoot, req.Agent)
	if err != nil {
		return "", err
	}
	if !transport.Active {
		return "", fmt.Errorf("fleet transport %q is inactive (active:false in agents.yaml); not dispatchable", req.Agent)
	}
	worktree := filepath.Join(store.ProjectRoot, "worktrees", req.TaskID)
	if _, statErr := os.Stat(worktree); statErr != nil {
		return "", fmt.Errorf("worktree for %s not found (run quorum task start): %w", req.TaskID, statErr)
	}
	var prompt string
	if req.BundlePath != "" {
		b, rerr := os.ReadFile(req.BundlePath)
		if rerr != nil {
			return "", fmt.Errorf("cannot read bundle_path %s: %w", req.BundlePath, rerr)
		}
		prompt = string(b)
	}
	timeoutS := req.TimeoutS
	if timeoutS <= 0 {
		timeoutS = transport.Timeouts.DefaultS
	}
	dispatchDir := filepath.Join(taskDir.Path, "dispatch", req.DispatchID)
	vars := map[string]string{
		"worktree":         worktree,
		"prompt":           prompt,
		"out":              filepath.Join(dispatchDir, "delegate-out.jsonl"),
		"model_arg":        stringField(transport.Models[req.Model], "model_arg"),
		"reasoning_effort": stringField(transport.Models[req.Model], "reasoning_effort"),
	}
	argv := substituteFleetArgv(transport.ArgvTemplate, vars)
	stdinPrompt := prompt
	if containsToken(transport.ArgvTemplate, "{prompt_file}") {
		aiderArgv, aerr := assembleAiderInvocation(taskDir.Path, dispatchDir, prompt, vars, transport.ArgvTemplate)
		if aerr != nil {
			return "", aerr
		}
		argv = aiderArgv
		stdinPrompt = "" // aider has no stdin channel (input_channel: prompt_file)
	}
	spec := core.DispatchSpec{
		TaskID: req.TaskID, TaskDir: taskDir.Path, Agent: req.Agent, Model: req.Model,
		DispatchID: req.DispatchID, Worktree: worktree, Binary: transport.Binary,
		Argv: argv, StdinPrompt: stdinPrompt,
		TimeoutS: timeoutS, FailureSignatures: transport.FailureSignatures, OutputFormat: transport.OutputFormat,
	}
	if _, err := core.Dispatch(spec); err != nil {
		return "", err
	}
	return filepath.Join(dispatchDir, "result.json"), nil
}

// containsToken reports whether tmpl contains the exact placeholder token
// (e.g. "{prompt_file}"), used to key the aider-specific dispatch branch
// without hardcoding an agent name.
func containsToken(tmpl []string, token string) bool {
	for _, t := range tmpl {
		if t == token {
			return true
		}
	}
	return false
}

// assembleAiderInvocation builds the aider argv for the prompt_file/{files}
// dispatch branch (FLEET-017): the bundle is materialized to a temp file
// under taskDir/dispatch/<id> (OUTSIDE the worktree, so it never pollutes the
// delegate's diff), {files} is sourced from 02-contract.yaml's touch list via
// core.Contract, and the resulting argv is validated and preflight-checked
// before the caller ever execs it. StdinPrompt must be set to "" by the
// caller (aider has no stdin channel).
func assembleAiderInvocation(taskDirPath, dispatchDir, prompt string, vars map[string]string, argvTemplate []string) ([]string, error) {
	if err := os.MkdirAll(dispatchDir, 0o755); err != nil {
		return nil, fmt.Errorf("cannot create dispatch dir for aider message file: %w", err)
	}
	messageFile := filepath.Join(dispatchDir, "message.txt")
	if err := os.WriteFile(messageFile, []byte(prompt), 0o644); err != nil {
		return nil, fmt.Errorf("cannot write aider message file: %w", err)
	}
	contractPath := filepath.Join(taskDirPath, "02-contract.yaml")
	raw, err := os.ReadFile(contractPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s for aider {files}: %w", contractPath, err)
	}
	var contract core.Contract
	if err := yaml.Unmarshal(raw, &contract); err != nil {
		return nil, fmt.Errorf("cannot parse %s: %w", contractPath, err)
	}
	if len(contract.Touch) == 0 {
		return nil, fmt.Errorf("02-contract.yaml touch list is empty; aider requires at least one file")
	}
	renderVars := make(map[string]string, len(vars)+1)
	for k, v := range vars {
		renderVars[k] = v
	}
	renderVars["prompt_file"] = messageFile
	argv := core.RenderAiderArgv(argvTemplate, renderVars, contract.Touch)
	if err := core.ValidateAiderArgv(argv); err != nil {
		return nil, err
	}
	if err := core.CheckAiderPreflight(renderVars["model_arg"]); err != nil {
		return nil, err
	}
	return argv, nil
}
func loadFleetTransport(projectRoot, agent string) (fleetTransport, error) {
	path := filepath.Join(projectRoot, ".agents", "fleet", "agents.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		return fleetTransport{}, fmt.Errorf("cannot read %s: %w", path, err)
	}
	var file struct {
		Transports map[string]fleetTransport `yaml:"transports"`
	}
	if err := yaml.Unmarshal(raw, &file); err != nil {
		return fleetTransport{}, fmt.Errorf("cannot parse %s: %w", path, err)
	}
	transport, ok := file.Transports[agent]
	if !ok {
		return fleetTransport{}, fmt.Errorf("unknown fleet transport %q", agent)
	}
	return transport, nil
}
func substituteFleetArgv(tmpl []string, vars map[string]string) []string {
	out := make([]string, 0, len(tmpl))
	for _, tok := range tmpl {
		for k, v := range vars {
			tok = strings.ReplaceAll(tok, "{"+k+"}", v)
		}
		out = append(out, tok)
	}
	return out
}
func stringField(m map[string]any, key string) string {
	if m != nil {
		if v, ok := m[key].(string); ok {
			return v
		}
	}
	return ""
}
func init() {
	fleetCmd.AddCommand(fleetDispatchCmd)
}
