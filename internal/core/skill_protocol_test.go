package core

import (
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
	pattern := regexp.MustCompile(`(?i)- \*\*Indicador de espera\*\*.*(cuando|si).*ESPERANDO RESPUESTA DEL USUARIO\.\.\.`)
	for _, name := range protocolSkillNames(t) {
		content := readProtocolSkill(t, name)
		if !strings.Contains(content, "Communication Protocol") {
			t.Errorf("%s: missing Communication Protocol", name)
		}
		if !pattern.MatchString(content) {
			t.Errorf("%s: wait indicator is not conditional; it must mention 'cuando' or 'si'", name)
		}
		if strings.Contains(strings.ToLower(content), "cerrá cada turno") {
			t.Errorf("%s: found absolute 'cerrá cada turno'; it must be conditional", name)
		}
	}
}

func TestSkillProtocolSinglePhaseBoundaryPreserved(t *testing.T) {
	for _, name := range protocolSkillNames(t) {
		content := readProtocolSkill(t, name)
		if !strings.Contains(content, "SIEMPRE respondé en español") &&
			!strings.Contains(content, "output al usuario es siempre en español") {
			t.Errorf("%s: missing Spanish-output instruction", name)
		}
		if !strings.Contains(strings.ToLower(content), "single-phase") {
			t.Errorf("%s: missing single-phase declaration", name)
		}
		if !strings.Contains(content, "NO actives ningún otro skill") &&
			!strings.Contains(content, "NO auto-activa otro /q-* skill") &&
			!strings.Contains(content, "Auto-encadenar viola la Regla #9") {
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
		"q-memory":    {"memory/"},
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
