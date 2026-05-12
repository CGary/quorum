import os
import sys
import json
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


def _write_trace(task_dir: Path, task_id: str, attempts=None):
    task_dir.mkdir(parents=True, exist_ok=True)
    trace = {
        "task_id": task_id,
        "summary": f"Trace for {task_id}",
        "started_at": "2026-05-12T00:00:00Z",
        "execution_mode": "worktree_edit",
        "attempts": attempts or [{"phase": "verify", "result": "failed", "duration_s": 1.0}],
        "total_cost_usd": 0.0,
        "violations": [],
        "context_overflows": [],
    }
    (task_dir / "07-trace.json").write_text(json.dumps(trace))
    return trace


def test_prepare_failed_child_retry_preserves_trace_and_restores_active(monkeypatch, tmp_path, capsys):
    _, ai_tasks = _setup_tasks(monkeypatch, tmp_path)
    _setup_parent_with_children(
        ai_tasks,
        "FEAT-020",
        [
            ("FEAT-020-a", "failed", "Slice A."),
            ("FEAT-020-b", "done", "Slice B."),
        ],
    )
    (ai_tasks / "done" / "FEAT-020").parent.mkdir(parents=True, exist_ok=True)
    (ai_tasks / "active" / "FEAT-020").rename(ai_tasks / "done" / "FEAT-020")
    failed_dir = ai_tasks / "failed" / "FEAT-020-a"
    original_trace = _write_trace(
        failed_dir,
        "FEAT-020-a",
        [{"phase": "verify", "result": "failed", "duration_s": 2.0, "notes": "old failure"}],
    )
    (failed_dir / "05-validation.json").write_text("{}")
    (failed_dir / "06-review.json").write_text("{}")
    monkeypatch.setattr(task_manager, "_ensure_retry_worktree", lambda task_id: True)

    assert task_manager.prepare_failed_child_retry("FEAT-020-a") is True

    out = capsys.readouterr().out
    active_child = ai_tasks / "active" / "FEAT-020-a"
    assert "restored to active" in out
    assert active_child.exists()
    assert not failed_dir.exists()
    assert (ai_tasks / "active" / "FEAT-020").exists()
    assert not (ai_tasks / "done" / "FEAT-020").exists()
    assert not (active_child / "05-validation.json").exists()
    assert not (active_child / "06-review.json").exists()
    retry_trace = json.loads((active_child / "07-trace.json").read_text())
    assert retry_trace["attempts"] == original_trace["attempts"]

    task_manager.show_status("FEAT-020")
    assert "parent_state: active" in capsys.readouterr().out


def test_prepare_failed_child_retry_blocks_dirty_worktree(monkeypatch, tmp_path, capsys):
    root, ai_tasks = _setup_tasks(monkeypatch, tmp_path)
    _setup_parent_with_children(
        ai_tasks,
        "FEAT-021",
        [("FEAT-021-a", "failed", "Slice A.")],
    )
    failed_dir = ai_tasks / "failed" / "FEAT-021-a"
    _write_trace(failed_dir, "FEAT-021-a")
    (failed_dir / "05-validation.json").write_text("{}")
    worktree_path = root / "worktrees" / "FEAT-021-a"
    worktree_path.mkdir(parents=True)
    monkeypatch.setattr(task_manager, "_is_worktree_dirty", lambda path: True)

    assert task_manager.prepare_failed_child_retry("FEAT-021-a") is False

    out = capsys.readouterr().out
    assert "uncommitted changes" in out
    assert (ai_tasks / "failed" / "FEAT-021-a").exists()
    assert not (ai_tasks / "active" / "FEAT-021-a").exists()
    assert (failed_dir / "05-validation.json").exists()


def test_prepare_failed_child_retry_rejects_failed_standalone(monkeypatch, tmp_path, capsys):
    _, ai_tasks = _setup_tasks(monkeypatch, tmp_path)
    _write_spec(
        ai_tasks / "failed" / "FEAT-022",
        {
            "task_id": "FEAT-022",
            "summary": "Standalone failed task",
            "goal": "Exercise retry rejection for standalone tasks.",
            "invariants": ["Invariant."],
            "acceptance": ["Acceptance."],
        },
    )

    assert task_manager.prepare_failed_child_retry("FEAT-022") is False

    out = capsys.readouterr().out
    assert "only authorized for failed child tasks" in out
    assert (ai_tasks / "failed" / "FEAT-022").exists()
    assert not (ai_tasks / "active" / "FEAT-022").exists()
