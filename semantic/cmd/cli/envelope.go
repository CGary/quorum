//go:build sqlite_fts5 && sqlite_vec

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	errInvalidArgument  = "INVALID_ARGUMENT"
	errMissingRequired  = "MISSING_REQUIRED_FLAG"
	errInvalidEnum      = "INVALID_ENUM"
	errFileNotFound     = "FILE_NOT_FOUND"
	errValidationFailed = "VALIDATION_FAILED"
	errPermissionDenied = "PERMISSION_DENIED"
	errConflict         = "CONFLICT"
	errTimeout          = "TIMEOUT"
	errNetworkError     = "NETWORK_ERROR"
	errInternal         = "INTERNAL_ERROR"
)

type NextAction struct {
	Command string `json:"command"`
	Reason  string `json:"reason"`
}

type SuccessEnvelope struct {
	OK          bool         `json:"ok"`
	Command     string       `json:"command"`
	Summary     string       `json:"summary"`
	Data        any          `json:"data"`
	NextActions []NextAction `json:"next_actions"`
}

type ErrorBody struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Field    string `json:"field,omitempty"`
	Received string `json:"received,omitempty"`
}

type SuggestedFix struct {
	Command string `json:"command"`
}

type ErrorEnvelope struct {
	OK           bool          `json:"ok"`
	Command      string        `json:"command"`
	Error        ErrorBody     `json:"error"`
	Retryable    bool          `json:"retryable"`
	SuggestedFix *SuggestedFix `json:"suggested_fix,omitempty"`
}

func NewErrorEnvelope(command, code, message, field, received string, retryable bool, fix string) ErrorEnvelope {
	env := ErrorEnvelope{
		OK:      false,
		Command: command,
		Error: ErrorBody{
			Code:     code,
			Message:  message,
			Field:    field,
			Received: received,
		},
		Retryable: retryable,
	}
	if fix != "" {
		env.SuggestedFix = &SuggestedFix{Command: fix}
	}
	return env
}

func WriteSuccessEnvelope(w io.Writer, env SuccessEnvelope) {
	if env.NextActions == nil {
		env.NextActions = []NextAction{}
	}
	b, err := json.Marshal(env)
	if err != nil {
		fmt.Fprintf(w, "{\"ok\":false,\"command\":%q,\"error\":{\"code\":%q,\"message\":\"could not marshal result\"},\"retryable\":false}\n", env.Command, errInternal)
		return
	}
	fmt.Fprintf(w, "%s\n", b)
}

func WriteErrorEnvelope(w io.Writer, env ErrorEnvelope) {
	b, err := json.Marshal(env)
	if err != nil {
		fmt.Fprintf(w, "{\"ok\":false,\"command\":%q,\"error\":{\"code\":%q,\"message\":\"could not marshal result\"},\"retryable\":false}\n", env.Command, errInternal)
		return
	}
	fmt.Fprintf(w, "%s\n", b)
}

func WriteHumanError(w io.Writer, env ErrorEnvelope) {
	fmt.Fprintf(w, "error: %s\n", env.Error.Message)
	if env.SuggestedFix != nil && env.SuggestedFix.Command != "" {
		fmt.Fprintf(w, "try: %s\n", env.SuggestedFix.Command)
	}
}

func WriteSchemaEnvelope(w io.Writer, schema any) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(schema)
}

func exitCodeForError(code string) int {
	switch code {
	case errMissingRequired, errInvalidEnum, errInvalidArgument, errValidationFailed:
		return exitUsage
	default:
		return exitRuntime
	}
}

func emitErrorAndExit(command, code, message, field, received string, retryable bool, fix string, jsonMode bool) {
	env := NewErrorEnvelope(command, code, message, field, received, retryable, fix)
	if jsonMode {
		WriteErrorEnvelope(os.Stdout, env)
	} else {
		WriteHumanError(os.Stderr, env)
	}
	os.Exit(exitCodeForError(code))
}

func writeOutputFile(path string, data any) error {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func parseFields(fieldsStr string) []string {
	if strings.TrimSpace(fieldsStr) == "" {
		return nil
	}
	parts := strings.Split(fieldsStr, ",")
	var fields []string
	for _, p := range parts {
		f := strings.TrimSpace(p)
		if f != "" {
			fields = append(fields, f)
		}
	}
	return fields
}

func projectItem(item any, fields []string) map[string]any {
	if len(fields) == 0 {
		return nil
	}
	data, err := json.Marshal(item)
	if err != nil {
		return map[string]any{}
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return map[string]any{}
	}
	proj := make(map[string]any)
	for _, f := range fields {
		if v, ok := raw[f]; ok {
			proj[f] = v
		}
	}
	return proj
}

func projectSlice[T any](items []T, fields []string) []map[string]any {
	if len(fields) == 0 {
		return nil
	}
	res := make([]map[string]any, len(items))
	for i, item := range items {
		res[i] = projectItem(item, fields)
	}
	return res
}
