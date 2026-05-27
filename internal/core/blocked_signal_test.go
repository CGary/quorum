package core

import (
	"testing"
)

func TestParseBlockedSignal_Success(t *testing.T) {
	msg := "BLOCKED: missing_file=src/foo.py; reason=Need to add to touch; severity=critical"
	sig, err := ParseBlockedSignal(msg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if sig.Path != "src/foo.py" {
		t.Errorf("expected path 'src/foo.py', got %q", sig.Path)
	}
	if sig.Reason != "Need to add to touch" {
		t.Errorf("expected reason 'Need to add to touch', got %q", sig.Reason)
	}
	if sig.Severity != "critical" {
		t.Errorf("expected severity 'critical', got %q", sig.Severity)
	}
}

func TestParseBlockedSignal_Failure(t *testing.T) {
	cases := []string{
		"BLOCKED: missing_file=; reason=foo; severity=critical",
		"BLOCKED: missing_file=foo; reason=; severity=critical",
		"BLOCKED: missing_file=foo; reason=bar; severity=unknown",
		"Just an error",
	}

	for _, tc := range cases {
		_, err := ParseBlockedSignal(tc)
		if err == nil {
			t.Errorf("expected error for message: %q", tc)
		}
	}
}
