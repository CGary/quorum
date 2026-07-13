package cmd

import (
	"encoding/json"
	"fmt"
	"io"
)

// Shared agent-I/O envelope for the fleet CLI (mk-cli-EN.md). It gives fleet
// commands a stable, agent-parseable JSON contract with the discipline the
// guide requires: stdout carries exactly one result object, stderr carries
// logs, and error codes are stable strings. Used by 'quorum fleet run'; kept
// generic so future agent-facing fleet subcommands can reuse it.

// Stable mk-cli error codes (mk-cli-EN.md "Error Handling for Agents").
const (
	errCodeInvalidArgument = "INVALID_ARGUMENT"
	errCodeMissingRequired = "MISSING_REQUIRED_FLAG"
	errCodeInvalidEnum     = "INVALID_ENUM"
	errCodeFileNotFound    = "FILE_NOT_FOUND"
	errCodeTimeout         = "TIMEOUT"
	errCodeInternal        = "INTERNAL_ERROR"
)

// fleetNextAction is one mk-cli next_actions suggestion (max 3 per envelope).
type fleetNextAction struct {
	Command string `json:"command"`
	Reason  string `json:"reason"`
}

// fleetSuccessEnvelope is the mk-cli success shape {ok, command, summary, data,
// next_actions}.
type fleetSuccessEnvelope struct {
	OK          bool              `json:"ok"`
	Command     string            `json:"command"`
	Summary     string            `json:"summary"`
	Data        any               `json:"data"`
	NextActions []fleetNextAction `json:"next_actions"`
}

// fleetErrorBody is the mk-cli error object {code, message, field, received}.
type fleetErrorBody struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Field    string `json:"field,omitempty"`
	Received string `json:"received,omitempty"`
}

// fleetSuggestedFix is the mk-cli suggested_fix.
type fleetSuggestedFix struct {
	Command string `json:"command"`
}

// fleetErrorEnvelope is the mk-cli error shape {ok:false, command, error,
// retryable, suggested_fix}.
type fleetErrorEnvelope struct {
	OK           bool               `json:"ok"`
	Command      string             `json:"command"`
	Error        fleetErrorBody     `json:"error"`
	Retryable    bool               `json:"retryable"`
	SuggestedFix *fleetSuggestedFix `json:"suggested_fix,omitempty"`
}

// fleetAgentError constructs a stable-coded error envelope. A non-empty fix
// becomes the suggested_fix command.
func fleetAgentError(command, code, message, field, received string, retryable bool, fix string) fleetErrorEnvelope {
	env := fleetErrorEnvelope{
		OK: false, Command: command,
		Error:     fleetErrorBody{Code: code, Message: message, Field: field, Received: received},
		Retryable: retryable,
	}
	if fix != "" {
		env.SuggestedFix = &fleetSuggestedFix{Command: fix}
	}
	return env
}

// fleetEmit carries the output-discipline flags for one fleet agent command.
// Under JSON, exactly one JSON object is written to stdout and nothing decorative
// (banner/emoji/spinner) ever reaches it; diagnostics go to stderr.
type fleetEmit struct {
	JSON  bool
	Plain bool
	Quiet bool
}

// success writes a success result to stdout honoring --json/--plain/--quiet.
func (e fleetEmit) success(stdout, _ io.Writer, env fleetSuccessEnvelope) {
	if e.JSON {
		writeCompactJSON(stdout, env)
		return
	}
	if e.Quiet {
		return
	}
	fmt.Fprintln(stdout, env.Summary)
}

// failure writes an error result to stdout honoring --json/--plain/--quiet.
// The error object is still the primary result, so it goes to stdout (not
// stderr) so an agent parsing stdout always gets a structured answer.
func (e fleetEmit) failure(stdout, _ io.Writer, env fleetErrorEnvelope) {
	if e.JSON {
		writeCompactJSON(stdout, env)
		return
	}
	fmt.Fprintf(stdout, "ERROR %s: %s\n", env.Error.Code, env.Error.Message)
}

// schema prints the input/output JSON contract for a command without executing.
func (e fleetEmit) schema(stdout io.Writer, schema any) {
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(schema)
}

// writeCompactJSON writes one compact JSON object followed by a newline.
func writeCompactJSON(w io.Writer, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintf(w, "{\"ok\":false,\"error\":{\"code\":%q,\"message\":\"could not marshal result\"}}\n", errCodeInternal)
		return
	}
	fmt.Fprintf(w, "%s\n", b)
}
