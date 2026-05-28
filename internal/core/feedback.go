package core

// PartitionedFeedback represents the separated mechanical and semantic findings.
type PartitionedFeedback struct {
	Mechanical []map[string]any `json:"mechanical"`
	Semantic   []map[string]any `json:"semantic"`
}

// PartitionFeedbackFindings partitions feedback findings by authority.
// Only explicit category "mechanical" findings are machine-applicable.
// Unknown or malformed categories are treated as semantic so the human remains
// the authority for meaning-changing corrections.
func PartitionFeedbackFindings(payload map[string]any) PartitionedFeedback {
	result := PartitionedFeedback{
		Mechanical: []map[string]any{},
		Semantic:   []map[string]any{},
	}

	if payload == nil {
		return result
	}

	findings, ok := payload["findings"].([]any)
	if !ok {
		return result
	}

	for _, f := range findings {
		finding, isMap := f.(map[string]any)
		if !isMap {
			continue
		}

		category, hasCategory := finding["category"].(string)
		if hasCategory && category == "mechanical" {
			result.Mechanical = append(result.Mechanical, finding)
		} else {
			result.Semantic = append(result.Semantic, finding)
		}
	}
	return result
}
