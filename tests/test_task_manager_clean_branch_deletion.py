import os
import subprocess
import sys
from pathlib import Path

import pytest
import yaml

PROJECT_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
AGENTS_DIR = os.path.join(PROJECT_ROOT, ".agents")
if AGENTS_DIR not in sys.path:
    sys.path.insert(0, AGENTS_DIR)

from cli.core import task_manager


def _run(cmd, cwd):
    subprocess.run(cmd, cwd=str(cwd), check=True, capture_output=True, text=True)


def _setup_repo(monkeypatch, tmp_path, task_id, base="main"):
    root = tmp_path / "repo"
    root.mkdir(parents=True, exist_ok=True)
    _run(["git", "init", "-q", "-b", "main", "."], root)
    _run(["git", "config", "user.email", "test@example.com"], root)
    _run(["git", "config", "user.name", "Test"], root)
    (root / "seed.txt").write_text("seed\n")
    _run(["git", "add", "seed.txt"], root)
    _run(["git", "commit", "-q", "-m", "init"], root)
    if base != "main":
        _run(["git", "checkout", "-q", "-b", base], root)

    ai_tasks = root / ".ai" / "tasks"
    for loc in ["inbox", "active", "done", "failed"]:
        (ai_tasks / loc).mkdir(parents=True, exist_ok=True)
    (root / "worktrees").mkdir(parents=True, exist_ok=True)

    task_dir = ai_tasks / "active" / f"{task_id}-new-spec"
    task_dir.mkdir(parents=True, exist_ok=True)
    (task_dir / "00-spec.yaml").write_text(yaml.safe_dump({
        "task_id": task_id,
        "summary": "Test task",
        "goal": "Exercise safe branch deletion during clean.",
        "invariants": ["Invariant."],
        "acceptance": ["Acceptance."],
    }, sort_keys=False))

    worktree_path = root / "worktrees" / task_id
    _run(["git", "worktree", "add", "-q", "-b", f"ai/{task_id}", str(worktree_path), base], root)

    monkeypatch.setattr(task_manager, "PROJECT_ROOT", root)
    monkeypatch.setattr(task_manager, "AI_TASKS", ai_tasks)
    monkeypatch.setattr(task_manager, "get_base_branch", lambda: base)
    return root, ai_tasks, worktree_path, task_dir


def _commit_on_task_branch(worktree_path, filename="feature.txt"):
    (worktree_path / filename).write_text("feature\n")
    _run(["git", "add", filename], worktree_path)
    _run(["git", "commit", "-q", "-m", "feature"], worktree_path)


def _branch_exists(root, branch):
    return subprocess.run(
        ["git", "-C", str(root), "show-ref", "--verify", f"refs/heads/{branch}"],
        capture_output=True,
        text=True,
    ).returncode == 0


def test_clean_deletes_task_branch_when_merged(monkeypatch, tmp_path):
    task_id = "FEAT-200"
    root, ai_tasks, worktree_path, task_dir = _setup_repo(monkeypatch, tmp_path, task_id)
    _commit_on_task_branch(worktree_path)
    _run(["git", "merge", "-q", "--no-ff", f"ai/{task_id}", "-m", "merge task"], root)

    task_manager.clean_task(task_id)

    assert not worktree_path.exists()
    assert (ai_tasks / "done" / f"{task_id}-new-spec").exists()
    assert not _branch_exists(root, f"ai/{task_id}")


def test_clean_preserves_task_branch_with_unmerged_commits(monkeypatch, tmp_path, capsys):
    task_id = "FEAT-201"
    root, ai_tasks, worktree_path, task_dir = _setup_repo(monkeypatch, tmp_path, task_id)
    _commit_on_task_branch(worktree_path)

    task_manager.clean_task(task_id)

    out = capsys.readouterr().out
    assert "Preserving local branch" in out
    assert f"ai/{task_id}" in out
    assert not worktree_path.exists()
    assert (ai_tasks / "done" / f"{task_id}-new-spec").exists()
    assert _branch_exists(root, f"ai/{task_id}")


def test_clean_missing_task_branch_is_noop_success(monkeypatch, tmp_path, capsys):
    task_id = "FEAT-202"
    root, ai_tasks, worktree_path, task_dir = _setup_repo(monkeypatch, tmp_path, task_id)
    _run(["git", "worktree", "remove", str(worktree_path)], root)
    _run(["git", "branch", "-D", f"ai/{task_id}"], root)

    task_manager.clean_task(task_id)

    out = capsys.readouterr().out
    assert "Branch ai/FEAT-202 is absent" in out
    assert (ai_tasks / "done" / f"{task_id}-new-spec").exists()


def test_clean_uses_dynamic_base_branch_for_merged_check(monkeypatch, tmp_path):
    task_id = "FEAT-203"
    root, ai_tasks, worktree_path, task_dir = _setup_repo(monkeypatch, tmp_path, task_id, base="develop")
    _commit_on_task_branch(worktree_path)
    _run(["git", "merge", "-q", "--no-ff", f"ai/{task_id}", "-m", "merge task"], root)

    task_manager.clean_task(task_id)

    assert not _branch_exists(root, f"ai/{task_id}")


def test_branch_deletion_helper_uses_only_safe_local_delete(monkeypatch, tmp_path):
    calls = []
    monkeypatch.setattr(task_manager, "PROJECT_ROOT", tmp_path)
    monkeypatch.setattr(task_manager, "_branch_exists", lambda branch: True)

    def fake_run(args, **kwargs):
        calls.append(args)
        if args[3:5] == ["merge-base", "--is-ancestor"]:
            return type("Result", (), {"returncode": 0, "stdout": "", "stderr": ""})()
        if args[3:5] == ["branch", "-d"]:
            return type("Result", (), {"returncode": 0, "stdout": "", "stderr": ""})()
        raise AssertionError(args)

    monkeypatch.setattr(task_manager.subprocess, "run", fake_run)

    assert task_manager._delete_branch_if_merged("ai/FEAT-204", "main") is True

    flattened = [part for call in calls for part in call]
    assert "push" not in flattened
    assert "-D" not in flattened
    assert ["git", "-C", str(tmp_path), "branch", "-d", "ai/FEAT-204"] in calls
