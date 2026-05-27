package core

import (
	"errors"
	"regexp"
	"strings"
)

var blockedSignalRe = regexp.MustCompile(`(?m)^BLOCKED:\s*missing_file=([^;\n]+);\s*reason=([^;\n]+);\s*severity=(critical|minor)\s*$`)

// BlockedSignal represents a standardized BLOCKED contract signal.
type BlockedSignal struct {
	Path     string `json:"path"`
	Reason   string `json:"reason"`
	Severity string `json:"severity"`
}

// ParseBlockedSignal parses a standardized BLOCKED contract signal.
// Returns an error when the message is not in the standardized format.
func ParseBlockedSignal(message string) (*BlockedSignal, error) {
	message = strings.TrimSpace(message)
	matches := blockedSignalRe.FindStringSubmatch(message)
	if matches == nil {
		return nil, errors.New("blocked signal must match 'BLOCKED: missing_file=<path>; reason=<text>; severity=<critical|minor>'")
	}

	path := strings.TrimSpace(matches[1])
	reason := strings.TrimSpace(matches[2])
	severity := matches[3]

	if path == "" {
		return nil, errors.New("blocked signal missing_file must not be empty")
	}
	if reason == "" {
		return nil, errors.New("blocked signal reason must not be empty")
	}

	return &BlockedSignal{
		Path:     path,
		Reason:   reason,
		Severity: severity,
	}, nil
}
