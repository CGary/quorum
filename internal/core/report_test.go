package core_test

import (
	"path/filepath"
	"quorum/internal/core"
	"testing"
)

func TestReportSchemaValidation(t *testing.T) {
	// Let's create a minimal valid report
	validPayload := map[string]any{
		"meta": map[string]any{
			"id":            "test-1",
			"schemaVersion": "1.0",
			"date":          "2026-06-01T12:00:00Z",
		},
		"summary": "Valid summary",
		"findings": []any{
			map[string]any{
				"id":          "F1",
				"description": "desc",
				"severity":    "high",
			},
		},
		"evidence": []any{
			map[string]any{
				"findingId": "F1",
				"path":      "src/foo.go",
				"details":   "details",
			},
		},
		"risks": []any{
			map[string]any{
				"id":          "R1",
				"description": "desc",
				"impact":      "high",
			},
		},
		"actionPlan": []any{
			map[string]any{
				"step":   1,
				"action": "fix it",
				"owner":  "gary",
			},
		},
	}

	err := core.ValidateAgainstSchema("report.schema.json", "dummy-report.yaml", validPayload)
	if err != nil {
		t.Fatalf("expected valid payload to pass schema, got: %v", err)
	}

	// Invalid payload: missing required field
	invalidPayload := map[string]any{
		"meta": map[string]any{
			"id": "test-1",
		},
	}

	err = core.ValidateAgainstSchema("report.schema.json", "dummy-report.yaml", invalidPayload)
	if err == nil {
		t.Fatalf("expected invalid payload to fail schema validation")
	}
}

func TestReportIDPatternAcceptsAndRejects(t *testing.T) {
	valid := []string{"report", "audit-01", "report_2026_05_21", "A", "x1"}
	for _, id := range valid {
		if err := core.ValidateReportID(id); err != nil {
			t.Errorf("expected %q to be a valid report id, got error: %v", id, err)
		}
	}

	invalid := []string{"", "bad/id", "..", "../escape", "-leading", "with space", "with.dot", "id$"}
	for _, id := range invalid {
		if err := core.ValidateReportID(id); err == nil {
			t.Errorf("expected %q to be rejected by ValidateReportID, but it passed", id)
		}
	}
}

func TestReportCheckIDMatches(t *testing.T) {
	good := map[string]any{"meta": map[string]any{"id": "audit-01"}}
	if err := core.CheckReportIDMatches(good, "audit-01"); err != nil {
		t.Errorf("expected matching meta.id to pass, got: %v", err)
	}

	mismatch := map[string]any{"meta": map[string]any{"id": "other"}}
	if err := core.CheckReportIDMatches(mismatch, "audit-01"); err == nil {
		t.Error("expected mismatched meta.id to be rejected")
	}

	noMeta := map[string]any{"summary": "x"}
	if err := core.CheckReportIDMatches(noMeta, "audit-01"); err == nil {
		t.Error("expected missing meta to be rejected")
	}

	noID := map[string]any{"meta": map[string]any{"schemaVersion": "1.0"}}
	if err := core.CheckReportIDMatches(noID, "audit-01"); err == nil {
		t.Error("expected missing meta.id to be rejected")
	}

	if err := core.CheckReportIDMatches("not-a-map", "audit-01"); err == nil {
		t.Error("expected non-mapping payload to be rejected")
	}
}

// TestSeedTemplateValidAgainstSchema guards the latent coupling introduced by
// validate-before-write: the shipped seed .agents/templates/report.yaml MUST be
// valid by construction, otherwise `quorum report new` would fail at runtime.
func TestSeedTemplateValidAgainstSchema(t *testing.T) {
	root, err := core.ProjectRoot()
	if err != nil {
		t.Fatalf("ProjectRoot failed: %v", err)
	}
	tmplPath := filepath.Join(root, ".agents", "templates", "report.yaml")
	payload, err := core.LoadArtifactPayload(tmplPath)
	if err != nil {
		t.Fatalf("failed to load seed template %s: %v", tmplPath, err)
	}

	// Use a virtual reports/ path so the dynamic matching resolves report.schema.json.
	if err := core.ValidateArtifact(".ai/reports/seed.yaml", payload); err != nil {
		t.Errorf("seed template is not valid against report.schema.json: %v", err)
	}
}
