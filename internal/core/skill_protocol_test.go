package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"
)

// Go port of the former tests/test_skill_protocol.py. These are static-analysis assertions
// over the .agents/skills/q-*/SKILL.md files: they enforce the skill output protocol
// (conditional wait indicator, single-phase boundary, Spanish chat / English artifacts,
// child-task decompose omission, semantic-feedback human authority, execution-context
// prefixes). They share no code with the skills they inspect.

const protocolWaitIndicator = "ESPERANDO RESPUESTA DEL USUARIO..."

func protocolSkillsDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(sourceRoot(t), ".agents", "skills")
}

func protocolSkillNames(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(protocolSkillsDir(t))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "q-") {
			names = append(names, e.Name())
		}
	}
	return names
}

func readProtocolSkill(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(protocolSkillsDir(t), name, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func containsAny(s string, markers []string) bool {
	for _, m := range markers {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}

// fenced ```text ... ``` blocks. Built from a double-quoted string so the backticks can be
// embedded; (?s) lets the captured group span newlines, *? keeps it non-greedy.
var protocolFencedTextBlock = regexp.MustCompile("(?s)```text\\n(.*?)\\n```")

func TestSkillProtocolWaitIndicatorIsConditional(t *testing.T) {
	pattern := regexp.MustCompile(`(?i)- \*\*Waiting indicator\*\*.*(only when|when|if).*ESPERANDO RESPUESTA DEL USUARIO\.\.\.`)
	for _, name := range protocolSkillNames(t) {
		content := readProtocolSkill(t, name)
		if !strings.Contains(content, "Communication Protocol") {
			t.Errorf("%s: missing Communication Protocol", name)
		}
		if !pattern.MatchString(content) {
			t.Errorf("%s: wait indicator is not conditional; it must mention 'only when', 'when', or 'if'", name)
		}
		if strings.Contains(strings.ToLower(content), "close every turn") {
			t.Errorf("%s: found absolute 'close every turn'; it must be conditional", name)
		}
	}
}

func TestSkillProtocolSinglePhaseBoundaryPreserved(t *testing.T) {
	for _, name := range protocolSkillNames(t) {
		content := readProtocolSkill(t, name)
		if !strings.Contains(content, "ALWAYS respond in Spanish") &&
			!strings.Contains(content, "user-facing output is always in Spanish") {
			t.Errorf("%s: missing Spanish-output instruction", name)
		}
		if !strings.Contains(strings.ToLower(content), "single-phase") {
			t.Errorf("%s: missing single-phase declaration", name)
		}
		if !strings.Contains(content, "Do NOT activate any other skill") &&
			!strings.Contains(content, "Auto-chaining violates Rule #9") {
			t.Errorf("%s: missing no-auto-activation instruction", name)
		}
	}
}

func TestSkillProtocolBriefAndStatusOmitDecomposeForChildren(t *testing.T) {
	for _, name := range []string{"q-brief", "q-status"} {
		content := readProtocolSkill(t, name)
		if !strings.Contains(content, "parent_task") {
			t.Errorf("%s: must mention parent_task in its logic or handoff", name)
		}
		if !strings.Contains(content, "omite /q-decompose") &&
			!strings.Contains(content, "sin sugerir /q-decompose") &&
			!strings.Contains(content, "omita /q-decompose") &&
			!strings.Contains(content, "omit `/q-decompose`") {
			t.Errorf("%s: must omit q-decompose for children", name)
		}
	}
}

func TestSkillProtocolAnalyzeDocumentsParentChildCoverage(t *testing.T) {
	content := readProtocolSkill(t, "q-analyze")
	for _, want := range []string{
		"Parent Decomposition Coverage",
		"decomposition-coverage",
		"parent_task",
		"depends_on",
		"strictly read-only",
		"do not run `quorum task back`, `blueprint`, `start`, `split`, `clean`",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("q-analyze: missing %q", want)
		}
	}
}

func TestSkillProtocolBriefGeneratedSpecContentIsEnglish(t *testing.T) {
	content := strings.ToLower(readProtocolSkill(t, "q-brief"))
	if !strings.Contains(content, "english") {
		t.Error("q-brief: must require english for generated YAML")
	}
	if !strings.Contains(content, "00-spec.yaml") && !strings.Contains(content, "yaml") {
		t.Error("q-brief: must reference the generated yaml")
	}
}

func TestSkillProtocolSuccessHandoffOmitsWaitIndicator(t *testing.T) {
	successMarkers := []string{
		"Artefacto producido:",
		"Artefactos producidos:",
		"Transición de estado ejecutada:",
		"Resultado: DONE",
		"Resultado: NO decomponer",
		"Resultado: decompuesto",
		"Veredicto: ready",
		"Veredicto: approve",
		"Reporte: emitido",
	}
	conditionalMarkers := []string{
		"BLOCKED",
		"Razón específica:",
		"not_ready",
		"Bloqueantes:",
		"¿Confirmás",
		"Respondé:",
	}
	for _, name := range protocolSkillNames(t) {
		content := readProtocolSkill(t, name)
		for _, m := range protocolFencedTextBlock.FindAllStringSubmatch(content, -1) {
			block := m[1]
			endsWithWait := strings.HasSuffix(strings.TrimRightFunc(block, unicode.IsSpace), protocolWaitIndicator)
			if endsWithWait && !containsAny(block, conditionalMarkers) {
				t.Errorf("%s: wait indicator is not in a conditional handoff block", name)
			}
			if !containsAny(block, successMarkers) {
				continue
			}
			if containsAny(block, conditionalMarkers) {
				continue
			}
			if endsWithWait {
				t.Errorf("%s: successful handoff must omit the wait indicator", name)
			}
		}
	}
}

func TestSkillProtocolArtifactProducingSkillsRequireEnglish(t *testing.T) {
	producers := map[string][]string{
		"q-brief":     {"00-spec.yaml"},
		"q-blueprint": {"01-blueprint.yaml", "02-contract.yaml"},
		"q-decompose": {"00-spec.yaml"},
		"q-implement": {"04-implementation-log.yaml"},
		"q-verify":    {"05-validation.json"},
		"q-review":    {"06-review.json"},
		"q-memory":    {"memory.schema.json", "quorum memory save"},
	}
	for name, artifacts := range producers {
		content := strings.ToLower(readProtocolSkill(t, name))
		if !strings.Contains(content, "english") {
			t.Errorf("%s: must require English artifact content", name)
		}
		if !strings.Contains(content, "field values") {
			t.Errorf("%s: must scope English to artifact field values", name)
		}
		for _, art := range artifacts {
			if !strings.Contains(content, strings.ToLower(art)) {
				t.Errorf("%s: English rule must reference %s", name, art)
			}
		}
	}
}

// TestReportCatalogDocsInSyncWithSchema closes the drift risk created by making
// the q-report SKILL.md "Component catalog" the authoritative in-skill contract:
// every component in report.schema.json (except `meta`) MUST be documented both
// in the skill catalog (as `name`) and in the seed template menu, so the docs an
// agent reads can never silently fall behind the schema.
func TestReportCatalogDocsInSyncWithSchema(t *testing.T) {
	root := sourceRoot(t)

	schemaRaw, err := os.ReadFile(filepath.Join(root, ".agents", "schemas", "report.schema.json"))
	if err != nil {
		t.Fatalf("read report schema: %v", err)
	}
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schemaRaw, &schema); err != nil {
		t.Fatalf("parse report schema: %v", err)
	}

	skill := readProtocolSkill(t, "q-report")
	tmpl, err := os.ReadFile(filepath.Join(root, ".agents", "templates", "report.yaml"))
	if err != nil {
		t.Fatalf("read report template: %v", err)
	}
	tmplStr := string(tmpl)

	for name := range schema.Properties {
		if name == "meta" {
			continue
		}
		if !strings.Contains(skill, "`"+name+"`") {
			t.Errorf("q-report SKILL.md catalog is missing component %q (schema/doc drift)", name)
		}
		if !strings.Contains(tmplStr, name) {
			t.Errorf("seed template menu is missing component %q (schema/template drift)", name)
		}
	}
}

// TestSkillProtocolReportFollowsUserLanguage guards the q-report exception to
// the English mandate: reports are human-facing deliverables rendered in the
// viewer, so their field values follow the user's prompt language (unless the
// user requests a specific one), instead of the machine-interop English rule
// that binds lifecycle artifacts (00-07) and SQLite memory.
func TestSkillProtocolReportFollowsUserLanguage(t *testing.T) {
	content := readProtocolSkill(t, "q-report")
	if !strings.Contains(content, "match the language of the user's prompt") {
		t.Error("q-report: field-value language rule must follow the user's prompt language")
	}
	if !strings.Contains(content, ".ai/reports/") {
		t.Error("q-report: language rule must scope to .ai/reports/ field values")
	}
}

func TestSkillProtocolQMemoryUsesSQLiteSave(t *testing.T) {
	content := readProtocolSkill(t, "q-memory")
	lower := strings.ToLower(content)

	for _, want := range []string{
		"quorum memory save",
		"cat <payload>.json | quorum memory save",
		"quorum memory save --file <payload>.json",
		".tmp/",
		"quorum init",
		"BLOCKED",
		"IDs guardados",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("q-memory: missing SQLite-save protocol marker %q", want)
		}
	}

	for _, forbiddenFinal := range []string{
		"memory/decisions",
		"memory/patterns",
		"memory/lessons",
	} {
		if strings.Contains(lower, forbiddenFinal+" as output") || strings.Contains(lower, forbiddenFinal+" como salida") {
			t.Errorf("q-memory: must not document %s as a final output location", forbiddenFinal)
		}
	}

	for _, want := range []string{
		"do not create, recreate, or write durable outputs under `memory/`",
		"do not write a fallback file under `memory/`",
		"never execute `quorum init` from this skill",
		"not local file paths",
	} {
		if !strings.Contains(lower, strings.ToLower(want)) {
			t.Errorf("q-memory: missing prohibition %q", want)
		}
	}
}

func TestSkillProtocolUserVisibleOutputTemplatesAreSpanish(t *testing.T) {
	for _, name := range protocolSkillNames(t) {
		content := readProtocolSkill(t, name)
		parts := strings.Split(content, "## Output")
		if len(parts) < 2 {
			continue
		}
		section := parts[1]
		for _, marker := range []string{"## Rules", "## 🛑 Handoff"} {
			if before, _, ok := strings.Cut(section, marker); ok {
				section = before
			}
		}
		// Instruction prose around the template is now English; only the emitted
		// user-facing template (the fenced ```text``` block) must stay Spanish.
		// Scan the fenced blocks for English label tokens that would mean the
		// template itself leaked into English.
		for _, m := range protocolFencedTextBlock.FindAllStringSubmatch(section, -1) {
			block := m[1]
			for _, forbidden := range []string{
				"Report:",
				"Task:",
				"Findings:",
				"Recommended fixes:",
				"Required human action:",
				"Blocking issues:",
				"Validation:",
				"Failed commands:",
				"Status:",
				"Next:",
				"Verdict:",
				"Location:",
				"Artifacts:",
			} {
				if strings.Contains(block, forbidden) {
					t.Errorf("%s: user-visible Output template contains English label %q", name, forbidden)
				}
			}
		}
		// The instruction prose must still explicitly require Spanish for the
		// user-visible report.
		if strings.Contains(section, "user-visible") && !strings.Contains(section, "Spanish") {
			t.Errorf("%s: user-visible Output section must explicitly require Spanish", name)
		}
	}
}

func TestSkillProtocolSemanticFeedbackNotAutoApplied(t *testing.T) {
	reSurface := regexp.MustCompile(`(?i)semantic[^\n]*(surface|surface the semantic feedback findings|human)`)
	reNoAutoApply := regexp.MustCompile(`(?i)semantic[^\n]*do NOT auto-apply semantic findings`)
	reNoConsume := regexp.MustCompile("(?i)semantic[^\\n]*do NOT consume `feedback\\.json`")
	for _, name := range []string{"q-brief", "q-blueprint"} {
		content := readProtocolSkill(t, name)
		if !strings.Contains(content, "feedback.json") {
			t.Errorf("%s: must mention feedback.json", name)
		}
		if !strings.Contains(content, "feedback-partition") {
			t.Errorf("%s: must mention feedback-partition", name)
		}
		if !reSurface.MatchString(content) {
			t.Errorf("%s: must surface semantic feedback to the human", name)
		}
		if !reNoAutoApply.MatchString(content) {
			t.Errorf("%s: must not auto-apply semantic feedback", name)
		}
		if !reNoConsume.MatchString(content) {
			t.Errorf("%s: must leave semantic feedback.json in place", name)
		}
		found := false
		for _, line := range strings.Split(content, "\n") {
			if strings.Contains(strings.ToLower(line), "semantic") && strings.Contains(line, "feedback.json") {
				found = true
				if strings.Contains(line, "quorum task feedback-consume") {
					t.Errorf("%s: semantic feedback line must not call feedback-consume", name)
				}
				break
			}
		}
		if !found {
			t.Errorf("%s: expected a line mentioning both semantic and feedback.json", name)
		}
	}
}

func TestSkillProtocolFencedCommandContextPrefix(t *testing.T) {
	cmdLine := regexp.MustCompile(`^\s*(?:-\s|\d+\.\s(?:\[.*?\]\s*)?)(quorum|git)`)
	for _, name := range protocolSkillNames(t) {
		content := readProtocolSkill(t, name)
		for _, m := range protocolFencedTextBlock.FindAllStringSubmatch(content, -1) {
			for _, line := range strings.Split(m[1], "\n") {
				if cmdLine.MatchString(line) {
					if !strings.Contains(line, "[ROOT]") && !strings.Contains(line, "[WORKTREE") {
						t.Errorf("%s: missing context prefix in handoff line: %s", name, line)
					}
				}
			}
		}
	}
}
