package core

import (
	"fmt"
	"regexp"
)

// ReportIDPattern is the canonical identifier shape for report artifacts. It is
// intentionally a safe filename slug: it forbids path separators and traversal
// (".."), so a report ID can never escape the .ai/reports/ write directory. It
// must start with an alphanumeric and then allow alphanumerics, hyphen, and
// underscore.
var ReportIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// ValidateReportID enforces ReportIDPattern at the write point. It is the hard
// regex invariant a probabilistic prompt cannot provide.
func ValidateReportID(id string) error {
	if !ReportIDPattern.MatchString(id) {
		return fmt.Errorf("invalid report id %q: must match %s", id, ReportIDPattern.String())
	}
	return nil
}

// CheckReportIDMatches enforces report identity: the payload's meta.id MUST equal
// the canonical filename id. This turns the server's filename-derived ID into a
// structural invariant instead of a prompt expectation.
func CheckReportIDMatches(payload any, id string) error {
	root, ok := payload.(map[string]any)
	if !ok {
		return fmt.Errorf("invalid report payload: expected a mapping at the root")
	}
	meta, ok := root["meta"].(map[string]any)
	if !ok {
		return fmt.Errorf("invalid report payload: missing meta object")
	}
	metaID, ok := meta["id"].(string)
	if !ok {
		return fmt.Errorf("invalid report payload: meta.id is missing or not a string")
	}
	if metaID != id {
		return fmt.Errorf("report identity mismatch: meta.id=%q does not match filename id=%q", metaID, id)
	}
	return nil
}
