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

// baseSemantic returns a fresh, fully valid semantic report on every call so a
// table case can mutate one field in isolation without leaking into siblings.
func baseSemantic() map[string]any {
	return map[string]any{
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
			"language": "en",
		},
		"content": map[string]any{
			"title":   "A Semantic Report",
			"verdict": map[string]any{"text": "Bottom line."},
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
}

func semContent(m map[string]any) map[string]any { return m["content"].(map[string]any) }

func semSetSections(m map[string]any, sections ...any) { semContent(m)["sections"] = sections }

// TestReportSchemaSemanticCases is the regression net for proposal §11.1: the
// schema and the Go post-schema hook already enforce these rules; this locks the
// behavior so a future schema edit cannot silently loosen it. Cases #1, #11,
// #16(legacy half), #19, #31 from §11.1 are intentionally absent: they assume a
// legacy model / root if-then discriminator removed in fbb5b27 (semantic-pure
// v1.1), so they are impossible by construction. #11 (mix legacy+semantic) is
// still covered by TestReportSemanticModelValidation via additionalProperties.
func TestReportSchemaSemanticCases(t *testing.T) {
	cases := []struct {
		name        string // §11.1 case number in parentheses
		mutate      func(m map[string]any)
		wantErr     bool
		errContains string
	}{
		{
			name:   "minimal valid (positive)",
			mutate: func(m map[string]any) {},
		},
		{
			name: "unknown role rejected by enum (#4/#33)",
			mutate: func(m map[string]any) {
				semSetSections(m, map[string]any{"id": "x", "role": "foobar", "title": "T"})
			},
			wantErr:     true,
			errContains: "is not one of",
		},
		{
			name: "verification without check (#5)",
			mutate: func(m map[string]any) {
				semSetSections(m, map[string]any{
					"id": "v", "role": "verification", "title": "V",
					"items": []any{map[string]any{"what": "X", "why": "Y"}},
				})
			},
			wantErr: true,
		},
		{
			name: "analysis without body (#6)",
			mutate: func(m map[string]any) {
				semSetSections(m, map[string]any{"id": "a", "role": "analysis", "title": "A"})
			},
			wantErr: true,
		},
		{
			name:    "unknown presentation.profile (#8)",
			mutate:  func(m map[string]any) { m["presentation"].(map[string]any)["profile"] = "fancy" },
			wantErr: true,
		},
		{
			name:    "unknown kind (#9)",
			mutate:  func(m map[string]any) { m["kind"] = "novel" },
			wantErr: true,
		},
		{
			name:        "missing content.sections (#14)",
			mutate:      func(m map[string]any) { delete(semContent(m), "sections") },
			wantErr:     true,
			errContains: "sections",
		},
		{
			name: "callout.kind out of enum (#15)",
			mutate: func(m map[string]any) {
				semSetSections(m, map[string]any{
					"id": "c", "role": "callout", "title": "C", "body": "B", "kind": "urgent",
				})
			},
			wantErr: true,
		},
		{
			name: "findings severity out of enum (#17)",
			mutate: func(m map[string]any) {
				semSetSections(m, map[string]any{
					"id": "f", "role": "findings", "title": "F",
					"items": []any{map[string]any{"id": "f1", "finding": "x", "severity": "blocker"}},
				})
			},
			wantErr: true,
		},
		{
			name: "risks impact out of enum (#17)",
			mutate: func(m map[string]any) {
				semSetSections(m, map[string]any{
					"id": "r", "role": "risks", "title": "R",
					"items": []any{map[string]any{"risk": "x", "impact": "catastrophic"}},
				})
			},
			wantErr: true,
		},
		{
			name:        "schemaVersion not 1.1 (#18)",
			mutate:      func(m map[string]any) { m["meta"].(map[string]any)["schemaVersion"] = "2.0" },
			wantErr:     true,
			errContains: "schemaVersion",
		},
		{
			name:        "missing kind (#21)",
			mutate:      func(m map[string]any) { delete(m, "kind") },
			wantErr:     true,
			errContains: "kind",
		},
		{
			name:        "missing presentation (#22)",
			mutate:      func(m map[string]any) { delete(m, "presentation") },
			wantErr:     true,
			errContains: "presentation",
		},
		{
			name:        "missing content.verdict (#23)",
			mutate:      func(m map[string]any) { delete(semContent(m), "verdict") },
			wantErr:     true,
			errContains: "verdict",
		},
		{
			name: "diagram.type not mermaid (#24)",
			mutate: func(m map[string]any) {
				semSetSections(m, map[string]any{
					"id": "d", "role": "diagram", "title": "D",
					"diagram": map[string]any{"type": "graphviz", "code": "x"},
				})
			},
			wantErr: true,
		},
		{
			name: "decision_surface.body non-string value (#25)",
			mutate: func(m map[string]any) {
				semSetSections(m, map[string]any{
					"id": "s", "role": "decision_surface", "title": "S",
					"body": map[string]any{"k": 42},
				})
			},
			wantErr: true,
		},
		{
			name: "metrics value non-numeric (#26)",
			mutate: func(m map[string]any) {
				semSetSections(m, map[string]any{
					"id": "mx", "role": "metrics", "title": "M",
					"items": []any{map[string]any{"label": "L", "value": "lots"}},
				})
			},
			wantErr: true,
		},
		{
			name: "autonomous evidence without findingId (#28, positive)",
			mutate: func(m map[string]any) {
				semSetSections(m, map[string]any{
					"id": "ev", "role": "evidence", "title": "E",
					"items": []any{map[string]any{"path": "main.go", "details": "line 1"}},
				})
			},
		},
		{
			name:    "presentation missing language (#30)",
			mutate:  func(m map[string]any) { delete(m["presentation"].(map[string]any), "language") },
			wantErr: true,
		},
		{
			name: "verification zero items (#32 minItems)",
			mutate: func(m map[string]any) {
				semSetSections(m, map[string]any{
					"id": "v", "role": "verification", "title": "V", "items": []any{},
				})
			},
			wantErr: true,
		},
		{
			name: "verification five items (#32 maxItems)",
			mutate: func(m map[string]any) {
				item := map[string]any{"what": "w", "why": "y", "check": "c"}
				semSetSections(m, map[string]any{
					"id": "v", "role": "verification", "title": "V",
					"items": []any{item, item, item, item, item},
				})
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := baseSemantic()
			tc.mutate(payload)
			err := core.ValidateAgainstSchema("report.schema.json", "reports/semantic.yaml", payload)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected validation error, got nil")
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("expected error containing %q, got: %v", tc.errContains, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected payload to validate, got: %v", err)
			}
		})
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

