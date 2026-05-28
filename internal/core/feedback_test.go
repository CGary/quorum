package core

import (
	"testing"
)

func TestPartitionFeedbackFindings(t *testing.T) {
	payload := map[string]any{
		"findings": []any{
			map[string]any{"category": "mechanical", "detail": "foo"},
			map[string]any{"category": "semantic", "detail": "bar"},
			map[string]any{"detail": "missing category"},
			map[string]any{"category": "unknown", "detail": "baz"},
		},
	}

	result := PartitionFeedbackFindings(payload)

	if len(result.Mechanical) != 1 {
		t.Errorf("Expected 1 mechanical finding, got %d", len(result.Mechanical))
	}
	if result.Mechanical[0]["detail"] != "foo" {
		t.Errorf("Expected mechanical detail 'foo', got %v", result.Mechanical[0]["detail"])
	}

	if len(result.Semantic) != 3 {
		t.Errorf("Expected 3 semantic findings, got %d", len(result.Semantic))
	}
}

func TestPartitionFeedbackFindingsEmpty(t *testing.T) {
	result := PartitionFeedbackFindings(nil)
	if len(result.Mechanical) != 0 || len(result.Semantic) != 0 {
		t.Errorf("Expected empty results for nil payload")
	}

	result2 := PartitionFeedbackFindings(map[string]any{})
	if len(result2.Mechanical) != 0 || len(result2.Semantic) != 0 {
		t.Errorf("Expected empty results for empty payload")
	}
}
