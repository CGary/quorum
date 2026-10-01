package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"quorum/internal/core"
)

// 'quorum fleet pick' is a NON-LIFECYCLE, read-only, advisory router: it asks
// the Jev classifier which OpenCode Go model should run a task and reports the
// pick. It never executes the task and creates no task/worktree/git/trace
// state. It fails open: a Jev error (unreachable, bad response, missing key,
// oversized state) is still an ok:true envelope carrying a fallback decision,
// never a command error.

const fleetPickCommand = "fleet.pick"

// fleetPickParams is the data-only input to the testable runner core. The cobra
// command binds flags into this struct; tests construct it directly with an
// injected Getenv so they never mutate the process environment.
type fleetPickParams struct {
	Input       string
	Provider    string
	TimeoutS    int
	JSON        bool
	Plain       bool
	Quiet       bool
	DryRun      bool
	NoInput     bool
	Schema      bool
	ProjectRoot string
	Stdin       io.Reader
	Getenv      func(string) string
	Now         func() time.Time
}

// knownPickProviders is the closed --provider flag enum, mirroring the closed
// set core.ValidateJevRouterPolicy accepts (core keeps its own unexported copy,
// so the flag surface re-declares the same three values).
var knownPickProviders = map[string]bool{
	"typesafe":             true,
	"openrouter_decisions": true,
	"openrouter_systemone": true,
}

// runFleetPick is the testable core: it returns a process exit code and writes
// exactly one result envelope to stdout (logs to stderr). It never touches
// .ai/tasks, git, a worktree, a forensic ref, 07-trace, or result.json.
func runFleetPick(p fleetPickParams, stdout, stderr io.Writer) int {
	emit := fleetEmit{JSON: p.JSON, Plain: p.Plain, Quiet: p.Quiet}
	fail := func(env fleetErrorEnvelope) int {
		emit.failure(stdout, stderr, env)
		return 1
	}

	if p.Schema {
		emit.schema(stdout, fleetPickSchema())
		return 0
	}

	root := p.ProjectRoot
	if root == "" {
		var err error
		root, err = core.ProjectRoot()
		if err != nil {
			return fail(fleetAgentError(fleetPickCommand, errCodeInternal,
				"cannot resolve project root: "+err.Error(), "", "", false, ""))
		}
	}
	getenv := p.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	now := p.Now
	if now == nil {
		now = time.Now
	}

	// Required flag (mk-cli MISSING_REQUIRED_FLAG).
	if p.Input == "" {
		return fail(fleetAgentError(fleetPickCommand, errCodeMissingRequired,
			"Missing required flag: --input (JSON file path, or - for stdin)", "input", "", true, ""))
	}
	raw, rerr := readPrompt(p.Input, p.Stdin)
	if rerr != nil {
		return fail(fleetAgentError(fleetPickCommand, errCodeFileNotFound,
			rerr.Error(), "input", p.Input, false, ""))
	}
	prompt, ctxStr, perr := parsePickInput(raw)
	if perr != nil {
		return fail(fleetAgentError(fleetPickCommand, errCodeInvalidArgument,
			"cannot parse --input JSON: "+perr.Error(), "input", "", false, ""))
	}
	if prompt == "" {
		return fail(fleetAgentError(fleetPickCommand, errCodeInvalidArgument,
			"input.prompt must not be empty", "prompt", "", false, ""))
	}

	policy, lerr := loadJevPolicy(root, getenv)
	if lerr != nil {
		if errors.Is(lerr, errJevPolicyNotFound) {
			return fail(fleetAgentError(fleetPickCommand, errCodeFileNotFound,
				lerr.Error(), "policy", "", false, ""))
		}
		return fail(fleetAgentError(fleetPickCommand, errCodeInvalidArgument,
			lerr.Error(), "", "", false, ""))
	}

	provider := p.Provider
	if provider == "" {
		provider = policy.Provider
	}
	if !knownPickProviders[provider] {
		return fail(fleetAgentError(fleetPickCommand, errCodeInvalidEnum,
			"--provider must be one of: typesafe, openrouter_decisions, openrouter_systemone",
			"provider", provider, false, ""))
	}

	chain := []string{provider}
	if policy.FallbackProvider != "" && policy.FallbackProvider != provider {
		chain = append(chain, policy.FallbackProvider)
	}

	transport, terr := loadPickTransport(root, getenv, policy.Transport)
	if terr != nil {
		return fail(fleetAgentError(fleetPickCommand, errCodeInvalidArgument,
			terr.Error(), "transport", policy.Transport, false, ""))
	}

	control, cerr := core.LoadFleetControlState(root)
	if cerr != nil {
		return fail(fleetAgentError(fleetPickCommand, errCodeInvalidArgument,
			"cannot load fleet control state: "+cerr.Error(), "", "", false, ""))
	}

	transportModels := make(map[string]bool, len(transport.Models))
	for m := range transport.Models {
		transportModels[m] = true
	}
	live, dropped := core.FilterLiveCandidates(policy, transport.Active, transportModels, control)
	if len(live) == 0 {
		return fail(fleetAgentError(fleetPickCommand, errCodeInvalidArgument,
			noLiveCandidatesMessage(dropped), "", "", false, ""))
	}

	req, trimmed, est, berr := core.BuildJevRequest(policy, provider, live, prompt, ctxStr)
	if errors.Is(berr, core.ErrJevStateTooLarge) {
		return emitPickFailOpen(emit, stdout, stderr, policy, provider, live, dropped, trimmed, est,
			root, p.Input, nil, &core.JevError{Status: 413, Message: berr.Error()})
	}

	if p.DryRun {
		url := policy.Providers[provider].URL
		if override := getenv("QUORUM_JEV_URL"); override != "" {
			url = override
		}
		emit.success(stdout, stderr, fleetSuccessEnvelope{
			OK:      true,
			Command: fleetPickCommand,
			Summary: fmt.Sprintf("dry-run: jev request for provider %s (no HTTP call)", provider),
			Data: map[string]any{
				"provider":         provider,
				"provider_chain":   chain,
				"url":              url,
				"request":          req,
				"candidates":       liveKeys(live),
				"dropped":          dropped,
				"state_trimmed":    trimmed,
				"estimated_tokens": est,
			},
			NextActions: []fleetNextAction{},
		})
		return 0
	}

	timeoutS := p.TimeoutS
	if timeoutS <= 0 {
		timeoutS = policy.TimeoutS
	}

	var attempts []core.ProviderAttempt
	var (
		successProvider string
		successResp     *core.JevResponse
		successClient   core.JevClient
		successTrimmed  bool
		successEst      int
		successLatency  int64
		lastErr         *core.JevError
	)

	for _, prov := range chain {
		preq, ptrimmed, pest, berr := core.BuildJevRequest(policy, prov, live, prompt, ctxStr)
		if errors.Is(berr, core.ErrJevStateTooLarge) {
			lastErr = &core.JevError{Status: 413, Message: berr.Error()}
			attempts = append(attempts, core.ProviderAttempt{Provider: prov, Status: 413, Message: lastErr.Message})
			break
		}

		urlOverride := getenv("QUORUM_JEV_FALLBACK_URL")
		if prov == provider {
			urlOverride = getenv("QUORUM_JEV_URL")
		}
		client, nerr := core.NewJevClient(policy, prov, urlOverride, getenv)
		if nerr != nil {
			lastErr = &core.JevError{Message: nerr.Error()}
			attempts = append(attempts, core.ProviderAttempt{Provider: prov, Message: nerr.Error()})
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutS)*time.Second)
		start := now()
		resp, jerr := client.Decide(ctx, preq)
		latency := now().Sub(start).Milliseconds()
		cancel()

		if jerr != nil {
			lastErr = jerr
			attempts = append(attempts, core.ProviderAttempt{
				Provider:  prov,
				Status:    jerr.Status,
				Message:   jerr.Message,
				LatencyMS: latency,
			})
			if !core.JevErrorFallsBack(jerr) {
				break
			}
			continue
		}

		successProvider = prov
		successResp = resp
		successClient = client
		successTrimmed = ptrimmed
		successEst = pest
		successLatency = latency
		break
	}

	if successProvider == "" {
		return emitPickFailOpen(emit, stdout, stderr, policy, provider, live, dropped, trimmed, est,
			root, p.Input, attempts, lastErr)
	}

	decision := core.DecidePick(policy, live, successResp, nil)
	decision = finalizePickDecision(decision, successProvider, attempts, dropped, successTrimmed, successEst, successLatency)

	secondCall := resolvePickBackup(&decision, live, policy, successProvider, prompt, ctxStr, timeoutS, successClient, now)

	emit.success(stdout, stderr, fleetSuccessEnvelope{
		OK:      true,
		Command: fleetPickCommand,
		Summary: pickSummary(decision),
		Data: fleetPickData{
			PickDecision: decision,
			SecondCall:   secondCall,
		},
		NextActions: pickNextActions(policy.Transport, root, p.Input, decision),
	})
	return 0
}

// emitPickFailOpen emits an ok:true decision for a Jev error. Jev errors are
// never command errors: an unreachable/oversized/bad classifier degrades to the
// fallback pair, and the human is still told which model would run.
func emitPickFailOpen(emit fleetEmit, stdout, stderr io.Writer, policy core.JevRouterPolicy, provider string, live []core.JevCandidate, dropped []core.DroppedCandidate, trimmed bool, est int, dir, inputPath string, attempts []core.ProviderAttempt, jerr *core.JevError) int {
	decision := finalizePickDecision(core.DecidePick(policy, live, nil, jerr), provider, attempts, dropped, trimmed, est, 0)
	emit.success(stdout, stderr, fleetSuccessEnvelope{
		OK:      true,
		Command: fleetPickCommand,
		Summary: pickSummary(decision),
		Data: fleetPickData{
			PickDecision: decision,
		},
		NextActions: pickNextActions(policy.Transport, dir, inputPath, decision),
	})
	return 0
}

// finalizePickDecision attaches the surrounding request-lifecycle fields that
// core.DecidePick does not know about (dropped candidates, trimming, token
// estimate, latency, provider attempts) and pins Provider to the policy
// provider NAME that produced the answer (the classifier's own echoed provider
// string lives in UpstreamProvider, set by core.DecidePick).
func finalizePickDecision(decision core.PickDecision, provider string, attempts []core.ProviderAttempt, dropped []core.DroppedCandidate, trimmed bool, est int, latencyMS int64) core.PickDecision {
	decision.Provider = provider
	decision.ProviderAttempts = attempts
	decision.Dropped = dropped
	decision.StateTrimmed = trimmed
	decision.EstimatedTokens = est
	decision.LatencyMS = latencyMS
	return decision
}

// fleetPickData is the success-envelope data payload: the core PickDecision
// (json-inlined, including backup_source) plus, when a degenerate first answer
// forced a second Jev call, the details of that second call.
type fleetPickData struct {
	core.PickDecision
	SecondCall *fleetPickSecondCall `json:"second_call,omitempty"`
}

// fleetPickSecondCall records the outcome of the optional second Jev call made
// when the first answer's rank-2 backup is degenerate (tied with rank 3 or
// lower, so it was chosen only by policy order).
type fleetPickSecondCall struct {
	Choice        string             `json:"choice"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Confidence    *float64           `json:"confidence,omitempty"`
	Usage         *core.JevUsage     `json:"usage,omitempty"`
	LatencyMS     int64              `json:"latency_ms"`
	Error         *core.JevError     `json:"error,omitempty"`
}

// resolvePickBackup performs the single allowed second Jev call when the first
// decision's backup came from ranking ("rank2") and is degenerate (tied with
// rank 3 or lower, so it was chosen only by policy order). It mutates
// decision.BackupSource in place: "second_call" on a successful second answer,
// "rank2_tiebreak" when the second call fails. A paired backup ("paired") never
// triggers a second call. It returns the details of any second call, or nil.
func resolvePickBackup(decision *core.PickDecision, live []core.JevCandidate, policy core.JevRouterPolicy, provider, prompt, ctxStr string, timeoutS int, client core.JevClient, now func() time.Time) *fleetPickSecondCall {
	if decision.BackupSource != "rank2" || len(live) < 3 || !pickBackupDegenerate(*decision, live) {
		return nil
	}
	live2 := liveWithout(live, decision.Primary.Key)
	req2, _, _, berr := core.BuildJevRequest(policy, provider, live2, prompt, ctxStr)
	if berr != nil {
		decision.BackupSource = "rank2_tiebreak"
		return nil
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Duration(timeoutS)*time.Second)
	start2 := now()
	resp2, jerr2 := client.Decide(ctx2, req2)
	latency2 := now().Sub(start2).Milliseconds()
	cancel2()
	dec2 := core.DecidePick(policy, live2, resp2, jerr2)
	second := &fleetPickSecondCall{
		Choice:        dec2.JevChoice,
		Probabilities: dec2.Probabilities,
		Confidence:    dec2.Confidence,
		Usage:         dec2.Usage,
		LatencyMS:     latency2,
		Error:         dec2.Error,
	}
	if dec2.Reason == "jev" {
		decision.Backup = &dec2.Primary
		decision.BackupSource = "second_call"
		return second
	}
	decision.BackupSource = "rank2_tiebreak"
	return second
}

// pickBackupDegenerate reports whether the first decision's backup is tied (or
// worse) with a lower-ranked non-primary candidate, meaning the backup was
// chosen by policy-order tie-break rather than by Jev. A missing probability is
// treated as 0.
func pickBackupDegenerate(d core.PickDecision, live []core.JevCandidate) bool {
	if d.Backup == nil {
		return false
	}
	backupProb := d.Probabilities[d.Backup.Key]
	for _, c := range live {
		if c.Key == d.Primary.Key || c.Key == d.Backup.Key {
			continue
		}
		if d.Probabilities[c.Key] >= backupProb {
			return true
		}
	}
	return false
}

// liveWithout returns the live candidates with the named key removed, keeping
// policy order.
func liveWithout(live []core.JevCandidate, key string) []core.JevCandidate {
	out := make([]core.JevCandidate, 0, len(live)-1)
	for _, c := range live {
		if c.Key != key {
			out = append(out, c)
		}
	}
	return out
}

// pickSummary renders the one-line human summary. On a classifier hit it names
// the picked pair; on a Jev error it reports the fallback and why.
func pickSummary(d core.PickDecision) string {
	backup := "none"
	if d.Backup != nil {
		backup = d.Backup.Key
	}
	if d.Reason == "jev" {
		return fmt.Sprintf("jev picked %s (backup %s), difficulty %s", d.Primary.Key, backup, d.Difficulty)
	}
	reason := d.Reason
	if d.Error != nil && d.Error.Message != "" {
		reason = d.Error.Message
	}
	return fmt.Sprintf("jev unavailable (%s): fallback %s/%s", reason, d.Primary.Key, backup)
}

// pickNextActions returns the run suggestions for the primary and backup cells.
func pickNextActions(transport, dir, input string, d core.PickDecision) []fleetNextAction {
	actions := make([]fleetNextAction, 0, 2)
	if d.Primary.Model != "" {
		actions = append(actions, fleetNextAction{
			Command: fmt.Sprintf("quorum fleet run --agent %s --model %s --cwd %s --input %s --no-input --json",
				transport, d.Primary.Model, dir, input),
			Reason: "run the Jev primary",
		})
	}
	if d.Backup != nil && d.Backup.Model != "" {
		actions = append(actions, fleetNextAction{
			Command: fmt.Sprintf("quorum fleet run --agent %s --model %s --cwd %s --input %s --no-input --json",
				transport, d.Backup.Model, dir, input),
			Reason: "run the Jev backup",
		})
	}
	return actions
}

// liveKeys returns the live candidate keys in policy order.
func liveKeys(live []core.JevCandidate) []string {
	keys := make([]string, 0, len(live))
	for _, c := range live {
		keys = append(keys, c.Key)
	}
	return keys
}

// noLiveCandidatesMessage renders the "no live candidates" error with each
// dropped candidate and the reason it was excluded.
func noLiveCandidatesMessage(dropped []core.DroppedCandidate) string {
	parts := make([]string, 0, len(dropped))
	for _, d := range dropped {
		parts = append(parts, fmt.Sprintf("%s (%s): %s", d.Key, d.Model, d.Reason))
	}
	return "no live candidates: " + strings.Join(parts, "; ")
}

// parsePickInput decodes the --input JSON contract: {"prompt": string required,
// "context": string optional}.
func parsePickInput(raw string) (prompt, context string, err error) {
	var in struct {
		Prompt  string `json:"prompt"`
		Context string `json:"context"`
	}
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		return "", "", err
	}
	return in.Prompt, in.Context, nil
}

// errJevPolicyNotFound marks a missing policy file so the caller can map it to
// FILE_NOT_FOUND rather than INVALID_ARGUMENT.
var errJevPolicyNotFound = errors.New("jev policy not found")

// loadJevPolicy reads and validates the Jev router policy from
// QUORUM_JEV_POLICY or <root>/.agents/policies/jev-router.yaml.
func loadJevPolicy(root string, getenv func(string) string) (core.JevRouterPolicy, error) {
	path := getenv("QUORUM_JEV_POLICY")
	if path == "" {
		path = filepath.Join(root, ".agents", "policies", "jev-router.yaml")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return core.JevRouterPolicy{}, fmt.Errorf("%w: %s", errJevPolicyNotFound, path)
		}
		return core.JevRouterPolicy{}, fmt.Errorf("cannot read jev policy %s: %w", path, err)
	}
	var policy core.JevRouterPolicy
	if err := yaml.Unmarshal(raw, &policy); err != nil {
		return core.JevRouterPolicy{}, fmt.Errorf("cannot parse jev policy %s: %w", path, err)
	}
	if err := core.ValidateJevRouterPolicy(policy); err != nil {
		return core.JevRouterPolicy{}, err
	}
	return policy, nil
}

// pickTransport is the minimal agents.yaml shape 'fleet pick' needs: whether
// the transport is active and which models its catalog declares. It mirrors the
// "transports" top-level key used by cmd.fleet_dispatch.go's loadFleetTransport.
type pickTransport struct {
	Active bool           `yaml:"active"`
	Models map[string]any `yaml:"models"`
}

// loadPickTransport reads the transport's active flag and model catalog from
// QUORUM_FLEET_AGENTS or <root>/.agents/fleet/agents.yaml, resolving the
// environment override through the injected getenv so tests stay offline.
func loadPickTransport(root string, getenv func(string) string, transportName string) (pickTransport, error) {
	path := getenv("QUORUM_FLEET_AGENTS")
	if path == "" {
		path = filepath.Join(root, ".agents", "fleet", "agents.yaml")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return pickTransport{}, fmt.Errorf("cannot read agents catalog %s: %w", path, err)
	}
	var file struct {
		Transports map[string]pickTransport `yaml:"transports"`
	}
	if err := yaml.Unmarshal(raw, &file); err != nil {
		return pickTransport{}, fmt.Errorf("cannot parse agents catalog %s: %w", path, err)
	}
	t, ok := file.Transports[transportName]
	if !ok {
		return pickTransport{}, fmt.Errorf("unknown fleet transport %q", transportName)
	}
	return t, nil
}

// fleetPickSchema builds the --schema contract. The --provider enum mirrors the
// three classifiers core accepts; the output properties are the PickDecision
// data fields.
func fleetPickSchema() map[string]any {
	return map[string]any{
		"command":     fleetPickCommand,
		"description": "NON-LIFECYCLE, read-only, advisory: ask Jev which OpenCode Go model should run a task. Never executes the task.",
		"input": map[string]any{
			"required": []string{"input"},
			"properties": map[string]any{
				"input": map[string]any{
					"type":        "string",
					"description": "JSON file path, or - for stdin; {\"prompt\": string required, \"context\": string optional}",
				},
				"provider": map[string]any{
					"type":        "string",
					"enum":        []string{"typesafe", "openrouter_decisions", "openrouter_systemone"},
					"description": "override policy.provider; on failure the policy's fallback_provider (if set) is tried",
				},
				"timeout": map[string]any{
					"type":        "integer",
					"description": "seconds; overrides policy.timeout_s",
				},
			},
		},
		"output": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"primary":           map[string]any{"type": "object", "description": "primary PickCell {key, model, probability}"},
				"backup":            map[string]any{"type": "object", "description": "backup PickCell {key, model, probability}"},
				"claude_fallback":   map[string]any{"type": "string"},
				"difficulty":        map[string]any{"type": "string"},
				"reason":            map[string]any{"type": "string", "enum": []string{"jev", "error", "invalid_response"}},
				"candidates":        map[string]any{"type": "array"},
				"dropped":           map[string]any{"type": "array"},
				"state_trimmed":     map[string]any{"type": "boolean"},
				"estimated_tokens":  map[string]any{"type": "integer"},
				"latency_ms":        map[string]any{"type": "integer"},
				"provider":          map[string]any{"type": "string", "description": "the policy provider name that produced the answer"},
				"upstream_provider": map[string]any{"type": "string", "description": "the provider string the classifier echoed in its response body"},
				"provider_attempts": map[string]any{"type": "array", "description": "each provider that failed before an answer was obtained; {provider, status, message, latency_ms}"},
				"error":             map[string]any{"type": "object"},
				"usage":             map[string]any{"type": "object"},
				"backup_source":     map[string]any{"type": "string", "enum": []string{"paired", "rank2", "second_call", "rank2_tiebreak", "fallback_pair", "none"}, "description": "how the backup was decided"},
				"second_call":       map[string]any{"type": "object", "description": "present only when a degenerate first answer forced a second Jev call; {choice, probabilities, confidence, usage, latency_ms, error}"},
			},
		},
		"errors": []string{errCodeMissingRequired, errCodeFileNotFound, errCodeInvalidArgument, errCodeInvalidEnum, errCodeInternal},
	}
}

var (
	fleetPickInput      string
	fleetPickProvider   string
	fleetPickTimeout    int
	fleetPickJSON       bool
	fleetPickPlain      bool
	fleetPickQuiet      bool
	fleetPickDryRun     bool
	fleetPickNoInput    bool
	fleetPickSchemaFlag bool
)

var fleetPickCmd = &cobra.Command{
	Use:   "pick",
	Short: "NON-LIFECYCLE: ask Jev which OpenCode Go model should run a task",
	Long: `quorum fleet pick is a read-only, advisory router: it asks the Jev classifier
which OpenCode Go model should run a task and reports the pick. It never
executes the task and creates no task/worktree/git/trace state.

The prompt arrives as --input <file|->: a JSON object {"prompt": string,
"context": string}. The policy comes from QUORUM_JEV_POLICY or
.agents/policies/jev-router.yaml. Jev errors fail open: the command still
returns ok:true with a fallback decision, never a command error.`,
	Run: func(cmd *cobra.Command, args []string) {
		root, err := core.ProjectRoot()
		if err != nil {
			fmt.Fprintln(os.Stderr, "[!] cannot resolve project root:", err)
			os.Exit(1)
		}
		code := runFleetPick(fleetPickParams{
			Input: fleetPickInput, Provider: fleetPickProvider, TimeoutS: fleetPickTimeout,
			JSON: fleetPickJSON, Plain: fleetPickPlain, Quiet: fleetPickQuiet,
			DryRun: fleetPickDryRun, NoInput: fleetPickNoInput, Schema: fleetPickSchemaFlag,
			ProjectRoot: root, Stdin: os.Stdin,
		}, os.Stdout, os.Stderr)
		if code != 0 {
			os.Exit(code)
		}
	},
}

func init() {
	f := fleetPickCmd.Flags()
	f.StringVar(&fleetPickInput, "input", "", "JSON prompt file path, or - for stdin")
	f.StringVar(&fleetPickProvider, "provider", "", "override policy.provider (typesafe|openrouter_decisions|openrouter_systemone)")
	f.IntVar(&fleetPickTimeout, "timeout", 0, "seconds; overrides policy.timeout_s")
	f.BoolVar(&fleetPickJSON, "json", false, "emit one JSON envelope on stdout")
	f.BoolVar(&fleetPickPlain, "plain", false, "plain text output for pipes")
	f.BoolVar(&fleetPickQuiet, "quiet", false, "suppress non-essential output")
	f.BoolVar(&fleetPickDryRun, "dry-run", false, "resolve and build the request without calling Jev")
	f.BoolVar(&fleetPickNoInput, "no-input", false, "never prompt interactively (agent default)")
	f.BoolVar(&fleetPickSchemaFlag, "schema", false, "print the input/output JSON contract and exit")
	fleetCmd.AddCommand(fleetPickCmd)
}
