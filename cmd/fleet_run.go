package cmd

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"quorum/internal/core"
)

// 'quorum fleet run' is a NON-LIFECYCLE standalone runner (FLEET-018). It
// executes an agent transport (primarily agy) in an explicit --cwd via the
// policy-free core.RunDelegate primitive, WITHOUT the SDC lifecycle: no task,
// no worktree, no git operation, no forensic ref, no 07-trace append, and no
// result.json. It reuses the transport-loading/argv-substitution helpers from
// cmd/fleet_dispatch.go unchanged and follows the mk-cli agent I/O contract.

const fleetRunCommand = "fleet.run"

// fleetRunParams is the data-only input to the testable runner core. The cobra
// command binds flags into this struct; tests construct it directly.
type fleetRunParams struct {
	Agent       string
	Model       string
	Cwd         string
	Input       string
	Output      string
	TimeoutS    int
	JSON        bool
	Plain       bool
	Quiet       bool
	DryRun      bool
	NoInput     bool
	Schema      bool
	ProjectRoot string
	Stdin       io.Reader
}

// runFleetRun is the testable core: it returns a process exit code and writes
// exactly one result envelope to stdout (logs to stderr). It never touches
// .ai/tasks, git, a worktree, a forensic ref, 07-trace, or result.json.
func runFleetRun(p fleetRunParams, stdout, stderr io.Writer) int {
	emit := fleetEmit{JSON: p.JSON, Plain: p.Plain, Quiet: p.Quiet}
	agent := p.Agent
	if agent == "" {
		agent = "agy"
	}
	fail := func(env fleetErrorEnvelope) int {
		emit.failure(stdout, stderr, env)
		return 1
	}

	// Transport is needed both for --schema (to list the model enum) and for a
	// real run. A load failure is only fatal for a run.
	transport, terr := loadFleetTransport(p.ProjectRoot, agent)

	if p.Schema {
		emit.schema(stdout, fleetRunSchema(transport))
		return 0
	}
	if terr != nil {
		return fail(fleetAgentError(fleetRunCommand, errCodeInvalidArgument,
			fmt.Sprintf("cannot load transport %q: %v", agent, terr), "agent", agent, false, ""))
	}
	applyFleetTransportEnv(transport.Env)

	// Required flags (mk-cli MISSING_REQUIRED_FLAG).
	if p.Model == "" {
		return fail(fleetAgentError(fleetRunCommand, errCodeMissingRequired,
			"Missing required flag: --model", "model", "", true,
			"quorum fleet run --agent "+agent+" --model <name> --cwd <dir> --input <file> --json"))
	}
	if p.Cwd == "" {
		return fail(fleetAgentError(fleetRunCommand, errCodeMissingRequired,
			"Missing required flag: --cwd", "cwd", "", true, ""))
	}
	if p.Input == "" {
		return fail(fleetAgentError(fleetRunCommand, errCodeMissingRequired,
			"Missing required flag: --input (prompt file path, or - for stdin)", "input", "", true, ""))
	}

	// --cwd must be an existing directory.
	if info, err := os.Stat(p.Cwd); err != nil || !info.IsDir() {
		return fail(fleetAgentError(fleetRunCommand, errCodeFileNotFound,
			"--cwd directory does not exist", "cwd", p.Cwd, false, ""))
	}

	// Closed --model enum derived at runtime from the transport models map.
	if _, ok := transport.Models[p.Model]; !ok {
		valid := sortedKeys(transport.Models)
		return fail(fleetAgentError(fleetRunCommand, errCodeInvalidEnum,
			fmt.Sprintf("--model must be one of: %s", strings.Join(valid, ", ")),
			"model", p.Model, false,
			"quorum fleet run --agent "+agent+" --model "+firstOr(valid, "<name>")+" --cwd "+p.Cwd+" --input "+p.Input+" --json"))
	}

	// Prompt arrives only via --input <file> or --input - (stdin).
	prompt, perr := readPrompt(p.Input, p.Stdin)
	if perr != nil {
		return fail(fleetAgentError(fleetRunCommand, errCodeFileNotFound,
			perr.Error(), "input", p.Input, false, ""))
	}

	// Effective timeout is computed BEFORE the vars map (FLEET-019): agy's
	// argv_template now references {print_timeout}, which must carry this
	// same effective value, so the ordering below is load-bearing.
	timeoutS := p.TimeoutS
	if timeoutS <= 0 {
		timeoutS = transport.Timeouts.DefaultS
	}

	// Substitute the transport argv template. agy uses {model_arg} and {prompt};
	// {reasoning_effort} resolves to empty for single-tier models. {print_timeout}
	// carries the same effective timeoutS used for the process-group hard-kill
	// below, so agy's own internal budget always matches the wrapper's. {cwd}
	// resolves to p.Cwd (FLEET-020) so opencode's --dir flag works identically
	// here and on the task-bound dispatch/smoke paths, unlike the dispatch-only
	// {worktree}/{out} tokens residualPlaceholder rejects below.
	vars := map[string]string{
		"prompt":           prompt,
		"cwd":              p.Cwd,
		"model_arg":        stringField(transport.Models[p.Model], "model_arg"),
		"reasoning_effort": stringField(transport.Models[p.Model], "reasoning_effort"),
		"print_timeout":    formatPrintTimeout(timeoutS),
	}
	argv := substituteFleetArgv(transport.ArgvTemplate, vars)

	// Reject any residual dispatch-only placeholder ({worktree}/{out}, etc.):
	// those transports are not runnable task-less, so we must not exec them.
	if resid := residualPlaceholder(argv); resid != "" {
		return fail(fleetAgentError(fleetRunCommand, errCodeInvalidArgument,
			fmt.Sprintf("transport %q argv references dispatch-only placeholder %q; not runnable via 'fleet run'", agent, resid),
			"agent", agent, false, ""))
	}

	// agy greedy --print/-p trap guard (reused unchanged from core).
	if agent == "agy" {
		if err := core.ValidateAgyArgv(argv); err != nil {
			return fail(fleetAgentError(fleetRunCommand, errCodeInvalidArgument,
				err.Error(), "argv", "", false, ""))
		}
	}

	if p.DryRun {
		emit.success(stdout, stderr, fleetSuccessEnvelope{
			OK: true, Command: fleetRunCommand,
			Summary: fmt.Sprintf("dry-run: would exec %s in %s (no process started)", transport.Binary, p.Cwd),
			Data: map[string]any{
				"dry_run": true, "binary": transport.Binary, "argv": argv,
				"cwd": p.Cwd, "model": p.Model, "timeout_s": timeoutS,
			},
			NextActions: []fleetNextAction{},
		})
		return 0
	}

	// Execute via the policy-free primitive; NO git/task/trace/result.json.
	stdinPrompt := prompt
	if transport.StdinEmpty {
		stdinPrompt = ""
	}
	res := core.RunDelegate(core.RunDelegateInput{
		Binary: transport.Binary, Argv: argv, Cwd: p.Cwd, StdinPrompt: stdinPrompt,
		TimeoutS: timeoutS, FailureSignatures: transport.FailureSignatures, OutputFormat: transport.OutputFormat,
	})

	if res.TimedOut {
		return fail(fleetAgentError(fleetRunCommand, errCodeTimeout,
			fmt.Sprintf("delegate exceeded --timeout of %ds", timeoutS), "timeout", "", true, ""))
	}

	data := map[string]any{
		"agent": agent, "model": p.Model, "cwd": p.Cwd,
		"exit_code": res.ExitCode, "killed": res.Killed,
		"quota_matched": res.QuotaMatched, "output_parse_ok": res.OutputParseOK,
	}
	summary := fmt.Sprintf("delegate %s exited with code %d", agent, res.ExitCode)

	// Large results may be redirected to --output; otherwise the output is inline.
	if p.Output != "" {
		if werr := os.WriteFile(p.Output, []byte(res.Output), 0o644); werr != nil {
			return fail(fleetAgentError(fleetRunCommand, errCodeInternal,
				fmt.Sprintf("cannot write --output %s: %v", p.Output, werr), "output", p.Output, false, ""))
		}
		data["result_file"] = p.Output
		summary = "result written to " + p.Output
	} else {
		data["output"] = res.Output
	}

	emit.success(stdout, stderr, fleetSuccessEnvelope{
		OK: true, Command: fleetRunCommand, Summary: summary, Data: data, NextActions: []fleetNextAction{},
	})
	return 0
}

// fleetRunSchema builds the --schema contract. The --model enum is derived from
// the transport models map at runtime (never a Go-side literal).
func fleetRunSchema(transport fleetTransport) map[string]any {
	return map[string]any{
		"command":     fleetRunCommand,
		"description": "NON-LIFECYCLE: run an agent transport in an explicit --cwd. No task, worktree, git, forensic ref, 07-trace, or result.json.",
		"input": map[string]any{
			"required": []string{"model", "cwd", "input"},
			"properties": map[string]any{
				"agent":   map[string]any{"type": "string", "default": "agy", "description": "transport name from .agents/fleet/agents.yaml"},
				"model":   map[string]any{"type": "string", "enum": sortedKeys(transport.Models), "description": "closed enum of canonical model names for the transport"},
				"cwd":     map[string]any{"type": "string", "description": "working directory the delegate runs in"},
				"input":   map[string]any{"type": "string", "description": "prompt file path, or - for stdin (never an inline prompt flag)"},
				"output":  map[string]any{"type": "string", "description": "optional file for large results; returned as data.result_file"},
				"timeout": map[string]any{"type": "integer", "description": "seconds before the delegate process group is killed"},
			},
		},
		"output": map[string]any{"type": "object", "required": []string{"ok", "command", "summary", "data"}},
		"errors": []string{errCodeMissingRequired, errCodeInvalidEnum, errCodeFileNotFound, errCodeTimeout, errCodeInvalidArgument, errCodeInternal},
	}
}

// readPrompt reads the prompt from a file, or from stdin when input == "-".
func readPrompt(input string, stdin io.Reader) (string, error) {
	if input == "-" {
		if stdin == nil {
			return "", nil
		}
		b, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("cannot read prompt from stdin: %v", err)
		}
		return string(b), nil
	}
	b, err := os.ReadFile(input)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("--input file does not exist: %s", input)
		}
		return "", fmt.Errorf("cannot read --input %s: %v", input, err)
	}
	return string(b), nil
}

// residualPlaceholder returns the first argv token still holding a {name}
// placeholder after substitution, or "" when none remain.
func residualPlaceholder(argv []string) string {
	for _, tok := range argv {
		if i := strings.IndexByte(tok, '{'); i >= 0 && strings.IndexByte(tok[i:], '}') > 0 {
			return tok
		}
	}
	return ""
}

func sortedKeys(m map[string]map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func firstOr(s []string, fallback string) string {
	if len(s) > 0 {
		return s[0]
	}
	return fallback
}

// fleet run flag bindings.
var (
	fleetRunAgent   string
	fleetRunModel   string
	fleetRunCwd     string
	fleetRunInput   string
	fleetRunOutput  string
	fleetRunTimeout int
	fleetRunJSON    bool
	fleetRunPlain   bool
	fleetRunQuiet   bool
	fleetRunDryRun  bool
	fleetRunNoInput bool
	fleetRunSchemaFlag  bool
)

var fleetRunCmd = &cobra.Command{
	Use:   "run",
	Short: "NON-LIFECYCLE: run an agent transport in an explicit --cwd (no task/worktree/trace)",
	Long: `quorum fleet run executes an agent transport (e.g. agy) in an explicit --cwd
via the policy-free RunDelegate primitive.

This is a NON-LIFECYCLE tool: it creates NO task, NO worktree, runs NO git
command, captures NO forensic ref, appends NO 07-trace entry, and writes NO
result.json. For the task-bound, forensic dispatch use 'quorum fleet dispatch'.

The prompt arrives only via --input <file> or --input - (stdin). --model is a
closed enum derived from the transport's models map; run with --schema to see it.`,
	Run: func(cmd *cobra.Command, args []string) {
		root, err := core.ProjectRoot()
		if err != nil {
			fmt.Fprintln(os.Stderr, "[!] cannot resolve project root:", err)
			os.Exit(1)
		}
		code := runFleetRun(fleetRunParams{
			Agent: fleetRunAgent, Model: fleetRunModel, Cwd: fleetRunCwd, Input: fleetRunInput,
			Output: fleetRunOutput, TimeoutS: fleetRunTimeout, JSON: fleetRunJSON, Plain: fleetRunPlain,
			Quiet: fleetRunQuiet, DryRun: fleetRunDryRun, NoInput: fleetRunNoInput, Schema: fleetRunSchemaFlag,
			ProjectRoot: root, Stdin: os.Stdin,
		}, os.Stdout, os.Stderr)
		if code != 0 {
			os.Exit(code)
		}
	},
}

func init() {
	f := fleetRunCmd.Flags()
	f.StringVar(&fleetRunAgent, "agent", "agy", "transport name from .agents/fleet/agents.yaml")
	f.StringVar(&fleetRunModel, "model", "", "canonical model name (closed enum; see --schema)")
	f.StringVar(&fleetRunCwd, "cwd", "", "working directory the delegate runs in")
	f.StringVar(&fleetRunInput, "input", "", "prompt file path, or - for stdin")
	f.StringVar(&fleetRunOutput, "output", "", "write large results to this file (returned as data.result_file)")
	f.IntVar(&fleetRunTimeout, "timeout", 0, "seconds before the delegate process group is killed (0 = transport default)")
	f.BoolVar(&fleetRunJSON, "json", false, "emit one JSON envelope on stdout")
	f.BoolVar(&fleetRunPlain, "plain", false, "plain text output for pipes")
	f.BoolVar(&fleetRunQuiet, "quiet", false, "suppress non-essential output")
	f.BoolVar(&fleetRunDryRun, "dry-run", false, "resolve and validate without starting the delegate")
	f.BoolVar(&fleetRunNoInput, "no-input", false, "never prompt interactively (agent default)")
	f.BoolVar(&fleetRunSchemaFlag, "schema", false, "print the input/output JSON contract and exit")
	fleetCmd.AddCommand(fleetRunCmd)
}
