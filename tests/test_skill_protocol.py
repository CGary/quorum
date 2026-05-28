import pytest
from pathlib import Path
import re

SKILLS_DIR = Path(".agents/skills")
SKILL_FILES = list(SKILLS_DIR.glob("q-*/SKILL.md"))
WAIT_INDICATOR = "ESPERANDO RESPUESTA DEL USUARIO..."

ARTIFACT_PRODUCING_SKILLS = {
    "q-brief": ["00-spec.yaml"],
    "q-blueprint": ["01-blueprint.yaml", "02-contract.yaml"],
    "q-decompose": ["00-spec.yaml"],
    "q-implement": ["04-implementation-log.yaml"],
    "q-verify": ["05-validation.json"],
    "q-review": ["06-review.json"],
    "q-memory": ["memory/"],
}

def test_communication_protocol_is_conditional():
    """Verify that all skills have a conditional wait indicator instruction."""
    pattern = re.compile(
        r"- \*\*Indicador de espera\*\*.*(cuando|si).*ESPERANDO RESPUESTA DEL USUARIO\.\.\.",
        re.IGNORECASE
    )
    
    for skill_file in SKILL_FILES:
        content = skill_file.read_text()
        assert "Communication Protocol" in content, f"Missing protocol in {skill_file}"
        assert pattern.search(content), f"Wait indicator is not conditional in {skill_file}. It must mention 'cuando' or 'si'."
        assert "cerrá cada turno" not in content.lower(), f"Found absolute 'cerrá cada turno' in {skill_file}. It must be conditional."

def test_single_phase_boundary_preserved():
    """Verify that skills still maintain the single-phase boundary and Spanish output."""
    for skill_file in SKILL_FILES:
        content = skill_file.read_text()
        assert "SIEMPRE respondé en español" in content or "output al usuario es siempre en español" in content
        assert "single-phase" in content.lower()
        assert "NO actives ningún otro skill" in content or "NO auto-activa otro /q-* skill" in content or "Auto-encadenar viola la Regla #9" in content

def test_q_brief_handoff_omits_decompose_for_children():
    """Verify q-brief has a specific handoff case for child tasks that omits q-decompose."""
    skill_file = SKILLS_DIR / "q-brief" / "SKILL.md"
    content = skill_file.read_text()
    
    # Check for presence of parent_task logic in handoff or next steps
    assert "parent_task" in content, "q-brief must mention parent_task in its logic or handoff"
    
    # Look for a section or instruction that omits q-decompose when it's a child
    assert "omite /q-decompose" in content or "sin sugerir /q-decompose" in content or "omita /q-decompose" in content

def test_q_analyze_documents_parent_child_coverage():
    """Verify q-analyze documents the read-only parent/child coverage pass."""
    skill_file = SKILLS_DIR / "q-analyze" / "SKILL.md"
    content = skill_file.read_text()

    assert "Parent Decomposition Coverage" in content
    assert "decomposition-coverage" in content
    assert "parent_task" in content
    assert "depends_on" in content
    assert "strictly read-only" in content
    assert "do not run `quorum task back`, `blueprint`, `start`, `split`, `clean`" in content

def test_q_brief_generated_spec_content_is_english():
    """Verify q-brief explicitly requires English generated YAML field content."""
    skill_file = SKILLS_DIR / "q-brief" / "SKILL.md"
    content = skill_file.read_text()

    assert "english" in content.lower(), "q-brief MUST require english for generated YAML."
    assert "00-spec.yaml" in content.lower() or "yaml" in content.lower()


def test_success_handoff_omits_waiting_indicator():
    """Verify successful/informational handoff templates do not end with a wait indicator."""
    success_markers = (
        "Artefacto producido:",
        "Artefactos producidos:",
        "Transición de estado ejecutada:",
        "Resultado: DONE",
        "Resultado: NO decomponer",
        "Resultado: decompuesto",
        "Veredicto: ready",
        "Veredicto: approve",
        "Reporte: emitido",
    )
    conditional_markers = (
        "BLOCKED",
        "Razón específica:",
        "not_ready",
        "Bloqueantes:",
        "¿Confirmás",
        "Respondé:",
    )

    fenced_text_block = re.compile(r"```text\n(.*?)\n```", re.DOTALL)
    for skill_file in SKILL_FILES:
        content = skill_file.read_text()
        for block in fenced_text_block.findall(content):
            if block.rstrip().endswith(WAIT_INDICATOR):
                assert any(marker in block for marker in conditional_markers), (
                    f"Wait indicator in {skill_file} is not in a conditional handoff block."
                )
            if not any(marker in block for marker in success_markers):
                continue
            if any(marker in block for marker in conditional_markers):
                continue
            assert not block.rstrip().endswith(WAIT_INDICATOR), (
                f"Successful handoff in {skill_file} must omit the wait indicator."
            )


def test_artifact_producing_skills_require_english():
    """Verify every persisted-artifact producer declares English artifact content."""
    for skill_name, expected_artifacts in ARTIFACT_PRODUCING_SKILLS.items():
        skill_file = SKILLS_DIR / skill_name / "SKILL.md"
        content = skill_file.read_text()
        lower_content = content.lower()

        assert "english" in lower_content, f"{skill_name} MUST require English artifact content."
        assert "field values" in lower_content, f"{skill_name} MUST scope English to artifact field values."
        for artifact in expected_artifacts:
            assert artifact.lower() in lower_content, (
                f"{skill_name} English rule must reference {artifact}."
            )


def test_semantic_feedback_findings_not_auto_applied_instruction():
    """q-brief and q-blueprint must preserve human authority for semantic feedback."""
    for skill_name in ["q-brief", "q-blueprint"]:
        content = (SKILLS_DIR / skill_name / "SKILL.md").read_text()

        assert "feedback.json" in content
        assert "feedback-partition" in content
        assert re.search(
            r"semantic[^\n]*(surface|surface the semantic feedback findings|human)",
            content,
            re.IGNORECASE,
        ), f"{skill_name} must surface semantic feedback to the human"
        assert re.search(
            r"semantic[^\n]*do NOT auto-apply semantic findings",
            content,
            re.IGNORECASE,
        ), f"{skill_name} must not auto-apply semantic feedback"
        assert re.search(
            r"semantic[^\n]*do NOT consume `feedback\.json`",
            content,
            re.IGNORECASE,
        ), f"{skill_name} must leave semantic feedback.json in place"
        semantic_line = next(
            line for line in content.splitlines()
            if "semantic" in line.lower() and "feedback.json" in line
        )
        assert "quorum task feedback-consume" not in semantic_line


def test_fenced_command_context_prefix():
    """Verify that every quorum or git CLI command in the handoff block has an execution context prefix."""
    fenced_text_block = re.compile(r"```text\n(.*?)\n```", re.DOTALL)
    for skill_file in SKILL_FILES:
        content = skill_file.read_text()
        blocks = fenced_text_block.findall(content)
        for block in blocks:
            for line in block.splitlines():
                if re.match(r'^\s*(?:-\s|\d+\.\s(?:\[.*?\]\s*)?)(quorum|git)', line):
                    assert "[ROOT]" in line or "[WORKTREE" in line, (
                        f"Missing context prefix in {skill_file.name} handoff block: {line}"
                    )
