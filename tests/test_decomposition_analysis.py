import os
import sys
from pathlib import Path

import yaml

PROJECT_ROOT = Path(__file__).resolve().parents[1]
AGENTS_DIR = PROJECT_ROOT / ".agents"
if str(AGENTS_DIR) not in sys.path:
    sys.path.insert(0, str(AGENTS_DIR))

from cli.core.decomposition_analysis import analyze_parent_child_coverage

SPEC_SCHEMA = PROJECT_ROOT / ".agents/schemas/spec.schema.json"

def _write(root, loc, name, spec):
    path = root / loc / name / "00-spec.yaml"
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(spec if isinstance(spec, str) else yaml.safe_dump(spec, sort_keys=False, allow_unicode=True))
    return path

def _parent(**overrides):
    spec = {
        "task_id": "FEAT-010",
        "summary": "Parent spec.",
        "goal": "Coordinate decomposed task coverage analysis.",
        "invariants": ["Invariant A remains true."],
        "acceptance": ["Acceptance A is externally visible."],
        "risk": "medium",
        "decomposition": [
            {"child_id": "FEAT-010-a", "summary": "Child A."},
            {"child_id": "FEAT-010-b", "summary": "Child B.", "depends_on": ["FEAT-010-a"]},
        ],
    }
    spec.update(overrides)
    return spec

def _child(child_id, **overrides):
    spec = {
        "task_id": child_id,
        "summary": f"{child_id} child spec.",
        "goal": f"Implement coverage slice for {child_id}.",
        "invariants": ["Invariant A remains true."],
        "acceptance": ["Acceptance A is externally visible."],
        "risk": "medium",
        "parent_task": "FEAT-010",
    }
    spec.update(overrides)
    return spec

def test_parent_child_coverage_complete(tmp_path):
    root = tmp_path / ".ai/tasks"
    parent = _write(root, "active", "FEAT-010", _parent())
    _write(root, "done", "FEAT-010-a", _child("FEAT-010-a", acceptance=[]))
    _write(root, "active", "FEAT-010-b-new-spec", _child("FEAT-010-b", invariants=[], depends_on=["FEAT-010-a"]))
    result = analyze_parent_child_coverage(parent, root, SPEC_SCHEMA)
    assert result["status"] == "pass"
    assert result["findings"] == []
    assert result["coverage"]["invariants"][0]["covered_by"] == ["FEAT-010-a"]
    assert result["coverage"]["acceptance"][0]["covered_by"] == ["FEAT-010-b"]

def test_parent_child_coverage_reports_partial_gaps(tmp_path):
    root = tmp_path / ".ai/tasks"
    parent = _write(root, "active", "FEAT-010", _parent(
        invariants=["Invariant A remains true.", "Invariant B remains true."],
        acceptance=["Acceptance A is externally visible.", "Acceptance B is externally visible."],
    ))
    _write(root, "done", "FEAT-010-a", _child("FEAT-010-a"))
    _write(root, "done", "FEAT-010-b", _child("FEAT-010-b", depends_on=["FEAT-010-a"], invariants=[], acceptance=[]))
    issues = [finding["issue"] for finding in analyze_parent_child_coverage(parent, root, SPEC_SCHEMA)["gaps"]]
    assert any("Invariant B remains true" in issue for issue in issues)
    assert any("Acceptance B is externally visible" in issue for issue in issues)

def test_parent_child_coverage_reports_missing_child_and_invalid_links(tmp_path):
    root = tmp_path / ".ai/tasks"
    parent = _write(root, "active", "FEAT-010", _parent(decomposition=[
        {"child_id": "FEAT-010-a", "summary": "Child A."},
        {"child_id": "FEAT-010-b", "summary": "Child B.", "depends_on": ["FEAT-010-a"]},
        {"child_id": "FEAT-010-c", "summary": "Missing child."},
        {"child_id": "FEAT-010-d", "summary": "Invalid child."},
    ]))
    _write(root, "done", "FEAT-010-a", _child("FEAT-010-a", parent_task="FEAT-999"))
    _write(root, "done", "FEAT-010-b", _child("FEAT-010-b", depends_on=["FEAT-010-x"]))
    _write(root, "done", "FEAT-010-d", "task_id: [\n")
    issues = [finding["issue"] for finding in analyze_parent_child_coverage(parent, root, SPEC_SCHEMA)["findings"]]
    assert any("FEAT-010-c is declared but no 00-spec.yaml was found" in issue for issue in issues)
    assert any("invalid 00-spec.yaml" in issue for issue in issues)
    assert any("expected 'FEAT-010'" in issue for issue in issues)
    assert any("undeclared sibling" in issue for issue in issues)
    assert any("does not match parent decomposition" in issue for issue in issues)

def test_parent_child_coverage_non_decomposed_task_is_compatible(tmp_path):
    root = tmp_path / ".ai/tasks"
    spec = _parent(); spec.pop("decomposition")
    result = analyze_parent_child_coverage(_write(root, "active", "FEAT-010", spec), root, SPEC_SCHEMA)
    assert result["applies"] is False
    assert result["status"] == "not_decomposed"
    assert result["findings"] == []
