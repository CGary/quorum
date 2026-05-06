import pytest
from pathlib import Path
import re

SKILLS_DIR = Path(".agents/skills")
SKILL_FILES = list(SKILLS_DIR.glob("q-*/SKILL.md"))

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
