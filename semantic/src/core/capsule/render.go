package capsule

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// RenderCapsule reads whatever artifacts exist in an archived task directory and renders
// a deterministic capsule in fixed ADR 0013 order:
// 1. Header (Task ID + State)
// 2. Intent (from 00-spec.yaml)
// 3. Key Decisions (from 01-blueprint.yaml)
// 4. Contract Surface (from 02-contract.yaml)
// 5. Outcome (from 05-validation.json and 06-review.json)
// 6. Failure Evidence (only when state == StateFailed)
//
// The only failure mode that returns a non-nil error is when the directory itself
// cannot be opened or statted on disk. Missing or malformed individual artifacts are handled
// gracefully and never fail the batch.
func RenderCapsule(dir string, state TaskState) (Capsule, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return Capsule{}, fmt.Errorf("task directory stat failed: %w", err)
	}
	if !info.IsDir() {
		return Capsule{}, fmt.Errorf("%s is not a directory", dir)
	}

	var taskID string
	var spec specDoc
	specParsed := false

	specPath := filepath.Join(dir, "00-spec.yaml")
	if specData, err := os.ReadFile(specPath); err == nil {
		if err := yaml.Unmarshal(specData, &spec); err == nil {
			specParsed = true
			if spec.TaskID != "" {
				taskID = spec.TaskID
			}
		}
	}

	if taskID == "" {
		taskID = filepath.Base(dir)
	}

	var b strings.Builder

	// 1. Header
	b.WriteString("Task ID: " + taskID + "\n")
	b.WriteString("State: " + string(state) + "\n")

	// 2. Intent (from 00-spec.yaml)
	if specParsed && (spec.Summary != "" || spec.Goal != "") {
		b.WriteString("\n--- Intent ---\n")
		if spec.Summary != "" {
			b.WriteString("Summary: " + spec.Summary + "\n")
		}
		if spec.Goal != "" {
			b.WriteString("Goal: " + spec.Goal + "\n")
		}
	}

	// 3. Key Decisions (from 01-blueprint.yaml)
	bpPath := filepath.Join(dir, "01-blueprint.yaml")
	if bpData, err := os.ReadFile(bpPath); err == nil {
		var bp blueprintDoc
		if err := yaml.Unmarshal(bpData, &bp); err == nil && len(bp.Strategy) > 0 {
			b.WriteString("\n--- Key Decisions ---\n")
			b.WriteString("Strategy:\n")
			for _, step := range bp.Strategy {
				fmt.Fprintf(&b, "  Step %d: %s\n", step.Step, step.Action)
				if len(step.Files) > 0 {
					fmt.Fprintf(&b, "    Files: %s\n", strings.Join(step.Files, ", "))
				}
			}
		}
	}

	// 4. Contract Surface (from 02-contract.yaml)
	ctPath := filepath.Join(dir, "02-contract.yaml")
	if ctData, err := os.ReadFile(ctPath); err == nil {
		var ct contractDoc
		if err := yaml.Unmarshal(ctData, &ct); err == nil && len(ct.Touch) > 0 {
			b.WriteString("\n--- Contract Surface ---\n")
			b.WriteString("Touch:\n")
			for _, f := range ct.Touch {
				b.WriteString("  - " + f + "\n")
			}
		}
	}

	// 5. Outcome (from 05-validation.json and 06-review.json)
	valPath := filepath.Join(dir, "05-validation.json")
	revPath := filepath.Join(dir, "06-review.json")

	var val validationDoc
	valParsed := false
	if valData, err := os.ReadFile(valPath); err == nil {
		if json.Unmarshal(valData, &val) == nil && val.OverallResult != "" {
			valParsed = true
		}
	}

	var rev reviewDoc
	revParsed := false
	if revData, err := os.ReadFile(revPath); err == nil {
		if json.Unmarshal(revData, &rev) == nil && rev.Verdict != "" {
			revParsed = true
		}
	}

	b.WriteString("\n--- Outcome ---\n")
	if !valParsed && !revParsed {
		b.WriteString("Outcome: unavailable\n")
	} else {
		if valParsed {
			fmt.Fprintf(&b, "Validation: %s\n", val.OverallResult)
		} else {
			b.WriteString("Validation: unavailable\n")
		}
		if revParsed {
			fmt.Fprintf(&b, "Review Verdict: %s\n", rev.Verdict)
		} else {
			b.WriteString("Review Verdict: unavailable\n")
		}
	}

	// 6. Failure Evidence (only when state == StateFailed)
	if state == StateFailed {
		b.WriteString("\n--- Failure Evidence ---\n")

		hasEvidence := false

		if valParsed && len(val.Commands) > 0 {
			var failedCmds []validationCommand
			for _, cmd := range val.Commands {
				if cmd.ExitCode != 0 || cmd.Error != "" {
					failedCmds = append(failedCmds, cmd)
				}
			}
			if len(failedCmds) > 0 {
				hasEvidence = true
				b.WriteString("Validation Command Failures:\n")
				for _, cmd := range failedCmds {
					fmt.Fprintf(&b, "  - Command: %s (Exit Code: %d)\n", cmd.Command, cmd.ExitCode)
					if cmd.Error != "" {
						fmt.Fprintf(&b, "    Error: %s\n", cmd.Error)
					}
					if cmd.Output != "" {
						fmt.Fprintf(&b, "    Output: %s\n", cmd.Output)
					}
				}
			}
		}

		if revParsed && (len(rev.Notes) > 0 || len(rev.FixTasks) > 0) {
			hasEvidence = true
			b.WriteString("Review Notes & Fix Tasks:\n")
			for _, note := range rev.Notes {
				fmt.Fprintf(&b, "  - Note: %s\n", note)
			}
			for _, ft := range rev.FixTasks {
				fmt.Fprintf(&b, "  - Fix Task: %s (%s)\n", ft.Slug, ft.Scope)
			}
		}

		trPath := filepath.Join(dir, "07-trace.json")
		if trData, err := os.ReadFile(trPath); err == nil {
			var tr traceDoc
			if json.Unmarshal(trData, &tr) == nil {
				var failedAttempts []traceAttempt
				for _, att := range tr.Attempts {
					if att.Result == "failed" || att.Result == "blocked" || att.Result == "attempt_failed" || att.Error != "" {
						failedAttempts = append(failedAttempts, att)
					}
				}
				if len(tr.Violations) > 0 || len(failedAttempts) > 0 {
					hasEvidence = true
					b.WriteString("Trace Failures:\n")
					for _, v := range tr.Violations {
						fmt.Fprintf(&b, "  - Violation: %s\n", v)
					}
					for _, att := range failedAttempts {
						fmt.Fprintf(&b, "  - Failed Attempt: Phase=%s, Result=%s, Error=%s\n", att.Phase, att.Result, att.Error)
					}
				}
			}
		}

		if !hasEvidence {
			b.WriteString("No additional failure details available\n")
		}
	}

	return Capsule{
		TaskID: taskID,
		State:  state,
		Text:   b.String(),
	}, nil
}
