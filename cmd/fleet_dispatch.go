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
	spec := core.DispatchSpec{
		TaskID: req.TaskID, TaskDir: taskDir.Path, Agent: req.Agent, Model: req.Model,
		DispatchID: req.DispatchID, Worktree: worktree, Binary: transport.Binary,
		Argv: substituteFleetArgv(transport.ArgvTemplate, vars), StdinPrompt: prompt,
		TimeoutS: timeoutS, FailureSignatures: transport.FailureSignatures, OutputFormat: transport.OutputFormat,
	}
	if _, err := core.Dispatch(spec); err != nil {
		return "", err
	}
	return filepath.Join(dispatchDir, "result.json"), nil
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
