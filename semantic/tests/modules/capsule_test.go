package modules

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/hsme/core/src/core/capsule"
)

func TestRenderCapsule_Determinism(t *testing.T) {
	fullDir := filepath.Join("testdata", "capsule", "full")

	cap1, err := capsule.RenderCapsule(fullDir, capsule.StateDone)
	if err != nil {
		t.Fatalf("expected no error rendering full fixture, got: %v", err)
	}

	cap2, err := capsule.RenderCapsule(fullDir, capsule.StateDone)
	if err != nil {
		t.Fatalf("expected no error on second render, got: %v", err)
	}

	if cap1.Text != cap2.Text {
		t.Errorf("expected byte-identical output across double-render, got diff:\n1:\n%s\n2:\n%s", cap1.Text, cap2.Text)
	}

	if cap1.TaskID != "HSME-001" {
		t.Errorf("expected TaskID HSME-001, got: %s", cap1.TaskID)
	}

	// Verify fixed section order
	headerIdx := strings.Index(cap1.Text, "Task ID:")
	intentIdx := strings.Index(cap1.Text, "--- Intent ---")
	decisionsIdx := strings.Index(cap1.Text, "--- Key Decisions ---")
	contractIdx := strings.Index(cap1.Text, "--- Contract Surface ---")
	outcomeIdx := strings.Index(cap1.Text, "--- Outcome ---")

	if headerIdx == -1 || intentIdx == -1 || decisionsIdx == -1 || contractIdx == -1 || outcomeIdx == -1 {
		t.Fatalf("missing one or more required sections in rendered capsule:\n%s", cap1.Text)
	}

	if !(headerIdx < intentIdx && intentIdx < decisionsIdx && decisionsIdx < contractIdx && contractIdx < outcomeIdx) {
		t.Errorf("section ordering out of spec order: header=%d, intent=%d, decisions=%d, contract=%d, outcome=%d",
			headerIdx, intentIdx, decisionsIdx, contractIdx, outcomeIdx)
	}
}

func TestRenderCapsule_LegacyFixture(t *testing.T) {
	legacyDir := filepath.Join("testdata", "capsule", "legacy")

	cap, err := capsule.RenderCapsule(legacyDir, capsule.StateDone)
	if err != nil {
		t.Fatalf("expected no error rendering legacy fixture (missing 05/06), got: %v", err)
	}

	if cap.TaskID != "F-01" {
		t.Errorf("expected TaskID F-01, got: %s", cap.TaskID)
	}

	if !strings.Contains(cap.Text, "Outcome: unavailable") {
		t.Errorf("expected outcome section to be marked unavailable for legacy fixture missing 05/06, got:\n%s", cap.Text)
	}
}

func TestRenderCapsule_MalformedFixture(t *testing.T) {
	malformedDir := filepath.Join("testdata", "capsule", "malformed")

	cap, err := capsule.RenderCapsule(malformedDir, capsule.StateDone)
	if err != nil {
		t.Fatalf("expected no error rendering malformed fixture, got: %v", err)
	}

	if cap.TaskID != "malformed" {
		t.Errorf("expected fallback TaskID 'malformed', got: %s", cap.TaskID)
	}

	if strings.Contains(cap.Text, "--- Intent ---") {
		t.Errorf("expected intent section to be omitted for malformed spec, got:\n%s", cap.Text)
	}
}

func TestRenderCapsule_FailedFixture(t *testing.T) {
	failedDir := filepath.Join("testdata", "capsule", "failed")

	cap, err := capsule.RenderCapsule(failedDir, capsule.StateFailed)
	if err != nil {
		t.Fatalf("expected no error rendering failed fixture, got: %v", err)
	}

	if cap.TaskID != "FAILED-001" {
		t.Errorf("expected TaskID FAILED-001, got: %s", cap.TaskID)
	}

	if !strings.Contains(cap.Text, "--- Failure Evidence ---") {
		t.Fatalf("expected failure evidence section for failed state, got:\n%s", cap.Text)
	}

	if !strings.Contains(cap.Text, "Validation Command Failures:") {
		t.Errorf("expected validation command failures in evidence, got:\n%s", cap.Text)
	}

	if !strings.Contains(cap.Text, "Review Notes & Fix Tasks:") {
		t.Errorf("expected review notes/fix tasks in evidence, got:\n%s", cap.Text)
	}

	if !strings.Contains(cap.Text, "Trace Failures:") {
		t.Errorf("expected trace failures in evidence, got:\n%s", cap.Text)
	}
}

func TestRenderCapsule_NonExistentDirectory(t *testing.T) {
	_, err := capsule.RenderCapsule("/nonexistent/directory/path/xyz", capsule.StateDone)
	if err == nil {
		t.Fatal("expected error when rendering non-existent directory, got nil")
	}
}
