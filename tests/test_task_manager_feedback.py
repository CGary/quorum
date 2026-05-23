import io
import json
import os
import sys
from pathlib import Path

import pytest
import yaml

PROJECT_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
AGENTS_DIR = os.path.join(PROJECT_ROOT, ".agents")
if AGENTS_DIR not in sys.path:
    sys.path.insert(0, AGENTS_DIR)

from cli.commands import task
from cli.core import task_manager


VALID_FEEDBACK = {
    "task_id": "FEAT-001",
    "summary": "q-analyze found reusable feedback.",
    "produced_by": "q-analyze",
    "generated_at": "2026-05-22T20:00:00Z",
    "findings": [
        {
            "severity": "low",
            "category": "mechanical",
            "artifact": "00-spec.yaml",
            "path": "$.summary",
            "issue": "Typo in summary.",
            "suggested_fix": "Fix the typo.",
        },
        {
            "severity": "high",
            "category": "semantic",
            "artifact": "02-contract.yaml",
            "path": "$.touch",
            "issue": "Contract omits a required implementation file.",
            "suggested_fix": "Ask the human whether the contract should expand.",
        },
    ],
}


def _setup_repo(monkeypatch, tmp_path):
    root = tmp_path / "repo"
    ai_tasks = root / ".ai" / "tasks"
    for loc in ["inbox", "active", "done", "failed"]:
        (ai_tasks / loc).mkdir(parents=True, exist_ok=True)
    monkeypatch.setattr(task_manager, "PROJECT_ROOT", root)
    monkeypatch.setattr(task_manager, "AI_TASKS", ai_tasks)
    task_dir = ai_tasks / "active" / "FEAT-001-new-spec"
    task_dir.mkdir(parents=True, exist_ok=True)
    (task_dir / "00-spec.yaml").write_text(yaml.safe_dump({
        "task_id": "FEAT-001",
        "summary": "Test task",
        "goal": "Exercise feedback artifact handling.",
        "invariants": ["Invariant."],
        "acceptance": ["Acceptance."],
    }, sort_keys=False))
    return task_dir


def test_save_feedback_validates_and_round_trips(monkeypatch, tmp_path):
    task_dir = _setup_repo(monkeypatch, tmp_path)

    path = task_manager.save_feedback(task_dir, VALID_FEEDBACK)

    assert path == task_dir / "feedback.json"
    assert task_manager._load_artifact_payload(path) == VALID_FEEDBACK


def test_save_feedback_rejects_invalid_payload(monkeypatch, tmp_path):
    task_dir = _setup_repo(monkeypatch, tmp_path)
    payload = dict(VALID_FEEDBACK)
    payload.pop("summary")

    with pytest.raises(task_manager.ArtifactValidationError) as exc:
        task_manager.save_feedback(task_dir, payload)

    assert "feedback.json" in str(exc.value)
    assert "field=$" in str(exc.value) or "field=$." in str(exc.value)
    assert "reason=" in str(exc.value)
    assert not (task_dir / "feedback.json").exists()


def test_artifact_save_supports_feedback_json(monkeypatch, tmp_path, capsys):
    task_dir = _setup_repo(monkeypatch, tmp_path)
    monkeypatch.setattr(sys, "stdin", io.StringIO(json.dumps(VALID_FEEDBACK)))

    task.artifact_save("FEAT-001", "feedback.json")

    out = capsys.readouterr().out
    assert "Saved artifact" in out
    assert json.loads((task_dir / "feedback.json").read_text()) == VALID_FEEDBACK


def test_consume_feedback_is_idempotent(monkeypatch, tmp_path):
    task_dir = _setup_repo(monkeypatch, tmp_path)
    task_manager.save_feedback(task_dir, VALID_FEEDBACK)

    assert task_manager.consume_feedback(task_dir) is True
    assert task_manager.consume_feedback(task_dir) is False
    assert not (task_dir / "feedback.json").exists()


def test_partition_feedback_findings_defaults_unknown_to_semantic():
    payload = json.loads(json.dumps(VALID_FEEDBACK))
    payload["findings"].append({
        "severity": "medium",
        "category": "unknown",
        "artifact": "01-blueprint.yaml",
        "path": "$.strategy",
        "issue": "Malformed category from legacy writer.",
        "suggested_fix": "Escalate to human.",
    })

    result = task_manager.partition_feedback_findings(payload)

    assert [item["category"] for item in result["mechanical"]] == ["mechanical"]
    assert [item["category"] for item in result["semantic"]] == ["semantic", "unknown"]


def test_cli_feedback_consume_dispatches_to_task_manager(monkeypatch, tmp_path, capsys):
    task_dir = _setup_repo(monkeypatch, tmp_path)
    task_manager.save_feedback(task_dir, VALID_FEEDBACK)

    task.feedback_consume("FEAT-001")

    out = capsys.readouterr().out
    assert "Consumed feedback" in out
    assert not (task_dir / "feedback.json").exists()
