import os
import sys
from pathlib import Path

import pytest
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


def _setup_parent_with_children(ai_tasks, parent_id, children_specs):
    parent_decomposition = [
        {"child_id": cid, "summary": summary}
        for cid, _, summary in children_specs
    ]
    _write_spec(
        ai_tasks / "active" / parent_id,
        {
            "task_id": parent_id,
            "summary": f"Parent {parent_id}",
            "goal": "Coordinate child tasks until completion.",
            "invariants": ["Invariant."],
            "acceptance": ["Acceptance."],
            "decomposition": parent_decomposition,
        },
    )
    for child_id, location, summary in children_specs:
        _write_spec(
            ai_tasks / location / child_id,
            {
                "task_id": child_id,
                "summary": summary,
                "goal": "Implement one child task slice.",
                "invariants": ["Invariant."],
                "acceptance": ["Acceptance."],
                "parent_task": parent_id,
            },
        )


def test_clean_last_child_auto_archives_parent(monkeypatch, tmp_path, capsys):
    _, ai_tasks = _setup_tasks(monkeypatch, tmp_path)
    _setup_parent_with_children(
        ai_tasks,
        "FEAT-010",
        [
            ("FEAT-010-a", "done", "Slice A."),
            ("FEAT-010-b", "active", "Slice B."),
        ],
    )

    task_manager.clean_task("FEAT-010-b")

    out = capsys.readouterr().out
    assert (ai_tasks / "done" / "FEAT-010-b").exists()
    assert not (ai_tasks / "active" / "FEAT-010").exists()
    assert (ai_tasks / "done" / "FEAT-010").exists()
    assert "auto-archiving parent" in out


def test_clean_non_last_child_keeps_parent_active(monkeypatch, tmp_path, capsys):
    _, ai_tasks = _setup_tasks(monkeypatch, tmp_path)
    _setup_parent_with_children(
        ai_tasks,
        "FEAT-011",
        [
            ("FEAT-011-a", "active", "Slice A."),
            ("FEAT-011-b", "active", "Slice B."),
        ],
    )

    task_manager.clean_task("FEAT-011-a")

    out = capsys.readouterr().out
    assert (ai_tasks / "done" / "FEAT-011-a").exists()
    assert (ai_tasks / "active" / "FEAT-011").exists()
    assert not (ai_tasks / "done" / "FEAT-011").exists()
    assert "auto-archiving parent" not in out


@pytest.mark.parametrize(
    ("task_id", "children", "expected"),
    [
        ("FEAT-012", [("FEAT-012-a", "failed"), ("FEAT-012-b", "active")], "partial"),
        ("FEAT-013", [("FEAT-013-a", "done"), ("FEAT-013-b", "done")], "completed"),
        ("FEAT-014", [("FEAT-014-a", "done"), ("FEAT-014-b", "active")], "active"),
    ],
)
def test_show_status_derives_parent_state(monkeypatch, tmp_path, capsys, task_id, children, expected):
    _, ai_tasks = _setup_tasks(monkeypatch, tmp_path)
    _setup_parent_with_children(
        ai_tasks,
        task_id,
        [(child_id, location, "Slice.") for child_id, location in children],
    )

    task_manager.show_status(task_id)

    assert f"parent_state: {expected}" in capsys.readouterr().out
    assert (ai_tasks / "active" / task_id).exists()


def test_show_status_omits_parent_state_for_standalone_task(monkeypatch, tmp_path, capsys):
    _, ai_tasks = _setup_tasks(monkeypatch, tmp_path)
    _write_spec(
        ai_tasks / "active" / "FEAT-015",
        {
            "task_id": "FEAT-015",
            "summary": "Standalone task",
            "goal": "Implement a standalone feature.",
            "invariants": ["Invariant."],
            "acceptance": ["Acceptance."],
        },
    )

    task_manager.show_status("FEAT-015")

    assert "parent_state" not in capsys.readouterr().out

def test_clean_task_idempotent_after_parent_auto_archived(monkeypatch, tmp_path, capsys):
    _, ai_tasks = _setup_tasks(monkeypatch, tmp_path)
    _setup_parent_with_children(
        ai_tasks,
        "FEAT-016",
        [
            ("FEAT-016-a", "done", "Slice A."),
        ],
    )
    _write_spec(
        ai_tasks / "active" / "FEAT-016-b",
        {
            "task_id": "FEAT-016-b",
            "summary": "Slice B.",
            "goal": "Implement one child task slice.",
            "invariants": ["Invariant."],
            "acceptance": ["Acceptance."],
            "parent_task": "FEAT-016",
        },
    )

    # Add child b to the parent's decomposition so it really is the last child.
    parent_spec_path = ai_tasks / "active" / "FEAT-016" / "00-spec.yaml"
    parent_spec = yaml.safe_load(parent_spec_path.read_text())
    parent_spec["decomposition"].append({"child_id": "FEAT-016-b", "summary": "Slice B."})
    parent_spec_path.write_text(yaml.safe_dump(parent_spec, sort_keys=False))

    task_manager.clean_task("FEAT-016-b")
    assert (ai_tasks / "done" / "FEAT-016").exists()

    # Re-running clean on the already-archived child must not break and must not
    # try to move the parent again.
    capsys.readouterr()
    task_manager.clean_task("FEAT-016-b")
    out = capsys.readouterr().out
    assert "auto-archiving parent" not in out
    assert (ai_tasks / "done" / "FEAT-016").exists()


def test_clean_standalone_task_does_not_touch_others(monkeypatch, tmp_path):
    _, ai_tasks = _setup_tasks(monkeypatch, tmp_path)
    _write_spec(
        ai_tasks / "active" / "FEAT-017",
        {
            "task_id": "FEAT-017",
            "summary": "Standalone task",
            "goal": "Implement a standalone feature.",
            "invariants": ["Invariant."],
            "acceptance": ["Acceptance."],
        },
    )

    task_manager.clean_task("FEAT-017")

    assert (ai_tasks / "done" / "FEAT-017").exists()
    assert not (ai_tasks / "active" / "FEAT-017").exists()


def test_clean_parent_direct_still_blocks_with_unfinished_children(monkeypatch, tmp_path, capsys):
    _, ai_tasks = _setup_tasks(monkeypatch, tmp_path)
    _setup_parent_with_children(
        ai_tasks,
        "FEAT-018",
        [
            ("FEAT-018-a", "failed", "Slice A."),
            ("FEAT-018-b", "done", "Slice B."),
        ],
    )

    task_manager.clean_task("FEAT-018")

    out = capsys.readouterr().out
    assert "unfinished children" in out
    assert (ai_tasks / "active" / "FEAT-018").exists()
