package core

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const FleetDispatchResultSchemaVersion = "fleet-dispatch-result/v1"

type DispatchOutcome struct {
	Class   string         `json:"class"`
	Noop    bool           `json:"noop"`
	Cause   *string        `json:"cause"`
	Blocked *BlockedSignal `json:"blocked"`
}

type DispatchDiffStat struct {
	Empty        bool `json:"empty"`
	FilesChanged int  `json:"files_changed"`
	Insertions   int  `json:"insertions"`
	Deletions    int  `json:"deletions"`
}

type DispatchUsage struct {
	Source    string `json:"source"`
	TokensIn  *int   `json:"tokens_in"`
	TokensOut *int   `json:"tokens_out"`
	Requests  *int   `json:"requests"`
}

type DispatchResult struct {
	SchemaVersion  string           `json:"schema_version"`
	DispatchID     string           `json:"dispatch_id"`
	TaskID         string           `json:"task_id"`
	Agent          string           `json:"agent"`
	Model          string           `json:"model"`
	Phase          string           `json:"phase"`
	StartedAt      string           `json:"started_at"`
	EndedAt        string           `json:"ended_at"`
	DurationS      float64          `json:"duration_s"`
	ExitCode       *int             `json:"exit_code"`
	TimedOut       bool             `json:"timed_out"`
	Killed         bool             `json:"killed"`
	Outcome        DispatchOutcome  `json:"outcome"`
	Diff           DispatchDiffStat `json:"diff"`
	ForensicRef    *string          `json:"forensic_ref"`
	BaselineCommit string           `json:"baseline_commit"`
	Applied        bool             `json:"applied"`
	Usage          DispatchUsage    `json:"usage"`
	NotesPath      string           `json:"notes_path"`
	TraceEvents    []string         `json:"trace_events"`
	ResetError     *string          `json:"reset_error"`
}

func normalizeResult(resultPath string, result DispatchResult) error {
	if result.SchemaVersion == "" {
		result.SchemaVersion = FleetDispatchResultSchemaVersion
	}
	if result.Phase == "" {
		result.Phase = "execute"
	}
	if result.Usage.Source == "" {
		result.Usage.Source = "none"
	}
	if result.TraceEvents == nil {
		result.TraceEvents = []string{}
	}
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(resultPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(resultPath, append(b, '\n'), 0o644)
}
