import os
import sys
from pathlib import Path

import yaml

PROJECT_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
AGENTS_DIR = os.path.join(PROJECT_ROOT, ".agents")
if AGENTS_DIR not in sys.path:
    sys.path.insert(0, AGENTS_DIR)

from cli.core import task_manager


def _setup_tasks(monkeypatch, tmp_path):
    root = tmp_path / "repo"
    ai_tasks = root / ".ai" / "tasks"
    for loc in ["inbox", "active", "done", "failed"]:
        (ai_tasks / loc).mkdir(parents=True, exist_ok=True)
    (root / "worktrees").mkdir(parents=True, exist_ok=True)
    monkeypatch.setattr(task_manager, "PROJECT_ROOT", root)
    monkeypatch.setattr(task_manager, "AI_TASKS", ai_tasks)
    return root, ai_tasks


def _write_spec(task_dir: Path, spec: dict):
    task_dir.mkdir(parents=True, exist_ok=True)
    (task_dir / "00-spec.yaml").write_text(yaml.safe_dump(spec, sort_keys=False))


def test_find_task_dir_does_not_confuse_parent_with_child(monkeypatch, tmp_path):
    _, ai_tasks = _setup_tasks(monkeypatch, tmp_path)
    _write_spec(
        ai_tasks / "inbox" / "FEAT-001-a-new-spec",
        {
            "task_id": "FEAT-001-a",
            "summary": "Child task",
            "goal": "Implement the first child slice.",
            "invariants": ["Keep behavior stable."],
            "acceptance": ["Child slice works."],
        },
    )

    assert task_manager.find_task_dir("FEAT-001", ["inbox"]) == (None, None)
    child_dir, loc = task_manager.find_task_dir("FEAT-001-a", ["inbox"])
    assert loc == "inbox"
    assert child_dir.name == "FEAT-001-a-new-spec"


def test_split_task_materialises_valid_children(monkeypatch, tmp_path):
    _, ai_tasks = _setup_tasks(monkeypatch, tmp_path)
    _write_spec(
        ai_tasks / "active" / "FEAT-001-new-spec",
        {
            "task_id": "FEAT-001",
            "summary": "Parent feature",
            "goal": "Deliver a feature that is large enough to split.",
            "invariants": ["Existing users keep working."],
            "acceptance": ["The full feature is observable."],
            "risk": "high",
            "non_goals": ["Do not redesign unrelated flows."],
            "constraints": ["No new runtime dependency."],
            "decomposition": [
                {"child_id": "FEAT-001-a", "summary": "Implement the data slice."},
                {"child_id": "FEAT-001-b", "summary": "Implement the UI slice.", "depends_on": ["FEAT-001-a"]},
            ],
        },
    )

    task_manager.split_task("FEAT-001")

    child_a = yaml.safe_load((ai_tasks / "inbox" / "FEAT-001-a" / "00-spec.yaml").read_text())
    child_b = yaml.safe_load((ai_tasks / "inbox" / "FEAT-001-b" / "00-spec.yaml").read_text())
    assert child_a["parent_task"] == "FEAT-001"
    assert child_a["risk"] == "high"
    assert child_a["constraints"] == ["No new runtime dependency."]
    assert child_b["depends_on"] == ["FEAT-001-a"]


def test_split_task_requires_active_parent(monkeypatch, tmp_path, capsys):
    _, ai_tasks = _setup_tasks(monkeypatch, tmp_path)
    _write_spec(
        ai_tasks / "inbox" / "FEAT-002-new-spec",
        {
            "task_id": "FEAT-002",
            "summary": "Parent feature",
            "goal": "Deliver a feature that is large enough to split.",
            "invariants": ["Invariant."],
            "acceptance": ["Acceptance."],
            "decomposition": [{"child_id": "FEAT-002-a", "summary": "Slice."}],
        },
    )

    task_manager.split_task("FEAT-002")

    out = capsys.readouterr().out
    assert "must be in active/" in out
    assert not (ai_tasks / "inbox" / "FEAT-002-a").exists()


def test_clean_parent_waits_for_children_done(monkeypatch, tmp_path, capsys):
    _, ai_tasks = _setup_tasks(monkeypatch, tmp_path)
    _write_spec(
        ai_tasks / "active" / "FEAT-003-new-spec",
        {
            "task_id": "FEAT-003",
            "summary": "Parent feature",
            "goal": "Coordinate child tasks until completion.",
            "invariants": ["Invariant."],
            "acceptance": ["Acceptance."],
            "decomposition": [{"child_id": "FEAT-003-a", "summary": "Slice."}],
        },
    )
    _write_spec(
        ai_tasks / "inbox" / "FEAT-003-a-new-spec",
        {
            "task_id": "FEAT-003-a",
            "summary": "Child feature",
            "goal": "Implement one child task slice.",
            "invariants": ["Invariant."],
            "acceptance": ["Acceptance."],
            "parent_task": "FEAT-003",
        },
    )

    task_manager.clean_task("FEAT-003")
    out = capsys.readouterr().out
    assert "unfinished children" in out
    assert (ai_tasks / "active" / "FEAT-003-new-spec").exists()

    child_inbox = ai_tasks / "inbox" / "FEAT-003-a-new-spec"
    child_done = ai_tasks / "done" / "FEAT-003-a-new-spec"
    child_done.parent.mkdir(parents=True, exist_ok=True)
    child_inbox.rename(child_done)

    task_manager.clean_task("FEAT-003")
    assert not (ai_tasks / "active" / "FEAT-003-new-spec").exists()
    assert (ai_tasks / "done" / "FEAT-003-new-spec").exists()
