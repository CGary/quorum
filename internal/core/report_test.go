package core_test

import (
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
