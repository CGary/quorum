package core

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// Aider adapter (FLEET-017): the third fleet transport and the first
// quota_class:api one. Unlike codex/agy (autonomous agents), aider is a pure
// editor-CLI -- it receives a bounded file set, a message, and a backend, and
// applies a localized edit. This file is process-free and never invokes
// aider itself; the generic FLEET-006-a dispatch engine (internal/core/
// fleet_dispatch.go) stays untouched and reused as-is (D2 reconciliation).

// aiderProviderEnvVars maps a litellm provider prefix (the first path segment
// of a model_arg, e.g. "openrouter" out of "openrouter/openrouter/free") to
// the process environment variable that must hold its API key. Keys are
// NEVER read from agents.yaml or any versioned artifact -- only from the
// process environment -- and CheckAiderPreflight only ever checks presence,
// never the value.
var aiderProviderEnvVars = map[string]string{
	"openrouter": "OPENROUTER_API_KEY",
}

// aiderRequiredArgvFlags are the mandatory flags ValidateAiderArgv guards:
// their absence means the argv is broken (e.g. an agents.yaml edit dropped a
// flag), which must classify wrapper_broken, never reroute_quota (AC-6).
var aiderRequiredArgvFlags = []string{
	"--message-file",
	"--no-auto-commits",
	"--no-attribute-co-authored-by",
	"--yes-always",
	"--model",
}

// RenderAiderArgv expands an aider argv_template into a concrete argv slice.
// It is pure and process-free: {prompt_file} and any other placeholder key
// present in vars are substituted token-by-token (same substitution style as
// substituteFleetArgv in cmd/fleet_dispatch.go), while the single "{files}"
// token is expanded into N positional argv tokens from files (02-contract.
// yaml's touch list) -- never treated as one opaque token like the generic
// engine's substituteFleetArgv does.
func RenderAiderArgv(argvTemplate []string, vars map[string]string, files []string) []string {
	out := make([]string, 0, len(argvTemplate)+len(files))
	for _, tok := range argvTemplate {
		if tok == "{files}" {
			out = append(out, files...)
			continue
		}
		for k, v := range vars {
			tok = strings.ReplaceAll(tok, "{"+k+"}", v)
		}
		out = append(out, tok)
	}
	return out
}

// ValidateAiderArgv guards that every mandatory flag (--message-file,
// --no-auto-commits, --no-attribute-co-authored-by, --yes-always, --model)
// is present in the rendered argv. A missing flag means the wiring itself is
// broken (a bad agents.yaml edit, a template regression), so a dispatch built
// from this argv must classify wrapper_broken, never reroute_quota (AC-6).
// This is a pure check over the argv slice; it never invokes aider.
func ValidateAiderArgv(argv []string) error {
	for _, required := range aiderRequiredArgvFlags {
		found := false
		for _, tok := range argv {
			if tok == required {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("aider argv guard: missing mandatory flag %q in argv %v", required, argv)
		}
	}
	return nil
}

// AiderUsage returns the DispatchUsage for an aider dispatch. aider emits no
// structured token/usage report, only free text -- Source is always "none",
// mirroring the agy precedent (FLEET-007); an aider dispatch must never
// report Source "cli_reported" and never fabricates TokensIn/TokensOut/
// Requests.
func AiderUsage() DispatchUsage {
	return DispatchUsage{Source: "none"}
}

// AiderRequiredEnvVar maps an aider model_arg (e.g.
// "openrouter/openrouter/free") to the process environment variable that
// must hold that provider's API key, by reading the first "/"-delimited
// segment as the litellm provider prefix. ok is false for an unrecognized
// provider so callers never silently skip the preflight guard.
func AiderRequiredEnvVar(modelArg string) (envVar string, ok bool) {
	provider := modelArg
	if i := strings.Index(modelArg, "/"); i >= 0 {
		provider = modelArg[:i]
	}
	envVar, ok = aiderProviderEnvVars[provider]
	return envVar, ok
}

// CheckAiderPreflight fails noisily when the API key environment variable a
// model_arg's provider requires is absent from the process environment
// (AC-8, analogous to the ADR 0010 join preflight). It checks PRESENCE ONLY
// via os.LookupEnv -- it never reads, logs, or returns the key's value.
func CheckAiderPreflight(modelArg string) error {
	envVar, ok := AiderRequiredEnvVar(modelArg)
	if !ok {
		return fmt.Errorf("aider preflight: no known provider API key mapping for model_arg %q", modelArg)
	}
	if _, present := os.LookupEnv(envVar); !present {
		return fmt.Errorf("aider preflight: required environment variable %s is not set", envVar)
	}
	return nil
}

// aiderSessionCostRe matches aider's free-text session cost report, e.g.
// "Cost: $0.01 message, $0.0456 session." -- the only structured-ish number
// aider's plain-text output offers.
var aiderSessionCostRe = regexp.MustCompile(`\$([0-9]+(?:\.[0-9]+)?)\s+session`)

// ParseAiderReportedCost extracts the session cost (USD) aider prints to its
// text output. ok is false when no session-cost figure is found -- callers
// must never fabricate a cost when parsing fails.
func ParseAiderReportedCost(output string) (cost float64, ok bool) {
	m := aiderSessionCostRe.FindStringSubmatch(output)
	if m == nil {
		return 0, false
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// CostExceedsCeiling reports whether a parsed session cost exceeds a model's
// max_cost_per_call_usd ceiling. This is a POST-DISPATCH detect-and-alert
// (ratified: aider v0.86.2 exposes no pre-spend cutoff flag) -- it never
// blocks the exec, only flags a cost_exceeded condition for the caller to
// surface as an alert. No new outcome class is introduced (the motor's
// classifyOutcome is untouched).
func CostExceedsCeiling(cost, ceiling float64) bool {
	return cost > ceiling
}
