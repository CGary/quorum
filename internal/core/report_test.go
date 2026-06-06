package core_test

import (
	"os"
	"path/filepath"
	"quorum/internal/core"
	"strings"
	"testing"
	"testing/fstest"
)

// TestEmbeddedAgentsExtractionAndRead exercises the hermetic fallback helpers
// with a synthetic bundle (the real embed is injected by package main, not
// available to this unit test).
func TestEmbeddedAgentsExtractionAndRead(t *testing.T) {
	mapfs := fstest.MapFS{
		"templates/report.yaml":      {Data: []byte("meta:\n  id: x\n")},
		"schemas/report.schema.json": {Data: []byte("{}")},
		"skills/q-report/SKILL.md":   {Data: []byte("# skill")},
	}
	core.SetEmbeddedAgents(mapfs)
	t.Cleanup(func() { core.SetEmbeddedAgents(nil) })

	if b, ok := core.EmbeddedAgentFile("templates/report.yaml"); !ok || !strings.Contains(string(b), "id: x") {
		t.Fatalf("EmbeddedAgentFile failed: ok=%v b=%q", ok, b)
	}
	if _, ok := core.EmbeddedAgentFile("does-not-exist.yaml"); ok {
		t.Error("EmbeddedAgentFile must report missing files as not-found")
	}

	dir, ok := core.EmbeddedAgentsDir()
	if !ok {
		t.Fatal("EmbeddedAgentsDir should extract the synthetic bundle")
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	if b, err := os.ReadFile(filepath.Join(dir, "schemas", "report.schema.json")); err != nil || string(b) != "{}" {
		t.Fatalf("extracted file mismatch: err=%v b=%q", err, b)
	}
}

func TestReportSchemaValidation(t *testing.T) {
	// Let's create a minimal valid report
	validPayload := map[string]any{
		"meta": map[string]any{
			"id":            "test-1",
			"schemaVersion": "1.1",
			"date":          "2026-06-01T12:00:00Z",
		},
		"kind": "generic",
		"presentation": map[string]any{
			"profile":  "cognitive",
			"density":  "medium",
			"audience": "engineer",
			"language": "en",
		},
		"content": map[string]any{
			"title": "Valid Report",
			"verdict": map[string]any{
				"text": "Valid",
			},
			"sections": []any{
				map[string]any{
					"id":    "sec-1",
					"role":  "analysis",
					"title": "Analysis Section",
					"body":  "Some body text",
				},
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

func TestReportSemanticModelValidation(t *testing.T) {
	// 1. Semantic report minimo valida
	validSemantic := map[string]any{
		"meta": map[string]any{
			"id":            "semantic-01",
			"schemaVersion": "1.1",
			"date":          "2026-06-01T12:00:00Z",
		},
		"kind": "generic",
		"presentation": map[string]any{
			"profile":  "cognitive",
			"density":  "medium",
			"audience": "engineer",
			"language": "es",
		},
		"content": map[string]any{
			"title": "A Semantic Report",
			"verdict": map[string]any{
				"text": "Passed validation",
			},
			"sections": []any{
				map[string]any{
					"id":    "sec-1",
					"role":  "analysis",
					"title": "Analysis Section",
					"body":  "Some body text",
				},
			},
		},
	}
	if err := core.ValidateAgainstSchema("report.schema.json", "reports/semantic.yaml", validSemantic); err != nil {
		t.Fatalf("expected valid semantic report to pass, got: %v", err)
	}

	// 2. Mezclar content con cualquier componente legacy top-level falla
	mixed := map[string]any{
		"meta": map[string]any{
			"id":            "semantic-01",
			"schemaVersion": "1.1",
			"date":          "2026-06-01T12:00:00Z",
		},
		"kind": "generic",
		"presentation": map[string]any{
			"profile":  "cognitive",
			"density":  "medium",
			"audience": "engineer",
			"language": "es",
		},
		"summary": "This is a legacy property", // legacy property mixed with semantic content
		"content": map[string]any{
			"title": "A Semantic Report",
			"verdict": map[string]any{
				"text": "Passed validation",
			},
			"sections": []any{
				map[string]any{
					"id":    "sec-1",
					"role":  "analysis",
					"title": "Analysis Section",
					"body":  "Some body text",
				},
			},
		},
	}
	if err := core.ValidateAgainstSchema("report.schema.json", "reports/semantic.yaml", mixed); err == nil {
		t.Fatal("expected mixed legacy + semantic report to fail validation")
	}

	// 3. content.sections: [] falla por minItems: 1
	emptySections := map[string]any{
		"meta": map[string]any{
			"id":            "semantic-01",
			"schemaVersion": "1.1",
			"date":          "2026-06-01T12:00:00Z",
		},
		"kind": "generic",
		"presentation": map[string]any{
			"profile":  "cognitive",
			"density":  "medium",
			"audience": "engineer",
			"language": "es",
		},
		"content": map[string]any{
			"title": "A Semantic Report",
			"verdict": map[string]any{
				"text": "Passed validation",
			},
			"sections": []any{},
		},
	}
	if err := core.ValidateAgainstSchema("report.schema.json", "reports/semantic.yaml", emptySections); err == nil {
		t.Fatal("expected empty sections list to fail validation")
	}

	// 4. IDs duplicados en content.sections[].id fallan
	duplicateSections := map[string]any{
		"meta": map[string]any{
			"id":            "semantic-01",
			"schemaVersion": "1.1",
			"date":          "2026-06-01T12:00:00Z",
		},
		"kind": "generic",
		"presentation": map[string]any{
			"profile":  "cognitive",
			"density":  "medium",
			"audience": "engineer",
			"language": "es",
		},
		"content": map[string]any{
			"title": "A Semantic Report",
			"verdict": map[string]any{
				"text": "Passed validation",
			},
			"sections": []any{
				map[string]any{
					"id":    "sec-1",
					"role":  "analysis",
					"title": "Analysis Section 1",
					"body":  "Some body text",
				},
				map[string]any{
					"id":    "sec-1", // duplicate id
					"role":  "analysis",
					"title": "Analysis Section 2",
					"body":  "Some other body text",
				},
			},
		},
	}
	err := core.ValidateAgainstSchema("report.schema.json", "reports/semantic.yaml", duplicateSections)
	if err == nil {
		t.Fatal("expected duplicate section IDs to fail validation")
	}
	if !strings.Contains(err.Error(), "duplicate section id") {
		t.Errorf("expected duplicate section ID error message, got: %v", err)
	}

	// 5. findings.items[].id duplicados a través del reporte fallan
	duplicateFindings := map[string]any{
		"meta": map[string]any{
			"id":            "semantic-01",
			"schemaVersion": "1.1",
			"date":          "2026-06-01T12:00:00Z",
		},
		"kind": "generic",
		"presentation": map[string]any{
			"profile":  "cognitive",
			"density":  "medium",
			"audience": "engineer",
			"language": "es",
		},
		"content": map[string]any{
			"title": "A Semantic Report",
			"verdict": map[string]any{
				"text": "Passed validation",
			},
			"sections": []any{
				map[string]any{
					"id":    "findings-1",
					"role":  "findings",
					"title": "Findings 1",
					"items": []any{
						map[string]any{
							"id":          "f1",
							"finding":      "First finding",
							"severity":    "high",
						},
					},
				},
				map[string]any{
					"id":    "findings-2",
					"role":  "findings",
					"title": "Findings 2",
					"items": []any{
						map[string]any{
							"id":          "f1", // duplicate finding ID across sections
							"finding":      "Second finding",
							"severity":    "low",
						},
					},
				},
			},
		},
	}
	err = core.ValidateAgainstSchema("report.schema.json", "reports/semantic.yaml", duplicateFindings)
	if err == nil {
		t.Fatal("expected duplicate finding IDs to fail validation")
	}
	if !strings.Contains(err.Error(), "duplicate finding id") {
		t.Errorf("expected duplicate finding ID error message, got: %v", err)
	}

	// 6. evidence.items[].findingId que no existe en findings.items[].id falla
	badEvidence := map[string]any{
		"meta": map[string]any{
			"id":            "semantic-01",
			"schemaVersion": "1.1",
			"date":          "2026-06-01T12:00:00Z",
		},
		"kind": "generic",
		"presentation": map[string]any{
			"profile":  "cognitive",
			"density":  "medium",
			"audience": "engineer",
			"language": "es",
		},
		"content": map[string]any{
			"title": "A Semantic Report",
			"verdict": map[string]any{
				"text": "Passed validation",
			},
			"sections": []any{
				map[string]any{
					"id":    "findings-1",
					"role":  "findings",
					"title": "Findings 1",
					"items": []any{
						map[string]any{
							"id":          "f1",
							"finding":      "First finding",
							"severity":    "high",
						},
					},
				},
				map[string]any{
					"id":    "evidence-1",
					"role":  "evidence",
					"title": "Evidence 1",
					"items": []any{
						map[string]any{
							"findingId": "f2", // does not exist
							"details":   "details of f2",
						},
					},
				},
			},
		},
	}
	err = core.ValidateAgainstSchema("report.schema.json", "reports/semantic.yaml", badEvidence)
	if err == nil {
		t.Fatal("expected evidence referencing unknown finding to fail validation")
	}
	if !strings.Contains(err.Error(), "unknown finding id") {
		t.Errorf("expected unknown finding ID error message, got: %v", err)
	}
}

