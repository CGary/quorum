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
    subprocess.run(cmd, cwd=str(cwd), check=True, capture_output=True)


def _setup_repo_with_worktree(monkeypatch, tmp_path, task_id):
    root = tmp_path / "repo"
    root.mkdir(parents=True, exist_ok=True)
    _run(["git", "init", "-q", "-b", "main", "."], root)
    _run(["git", "config", "user.email", "test@example.com"], root)
    _run(["git", "config", "user.name", "Test"], root)
    (root / "seed.txt").write_text("seed\n")
    _run(["git", "add", "seed.txt"], root)
    _run(["git", "commit", "-q", "-m", "init"], root)

    ai_tasks = root / ".ai" / "tasks"
    for loc in ["inbox", "active", "done", "failed"]:
        (ai_tasks / loc).mkdir(parents=True, exist_ok=True)
    (root / "worktrees").mkdir(parents=True, exist_ok=True)

    task_dir = ai_tasks / "active" / f"{task_id}-new-spec"
    task_dir.mkdir(parents=True, exist_ok=True)
    (task_dir / "00-spec.yaml").write_text(yaml.safe_dump({
        "task_id": task_id,
        "summary": "Test task",
        "goal": "Exercise clean_task behavior on a real worktree.",
        "invariants": ["Invariant."],
        "acceptance": ["Acceptance."],
    }, sort_keys=False))

    worktree_path = root / "worktrees" / task_id
    _run(["git", "worktree", "add", "-q", "-b", f"ai/{task_id}", str(worktree_path)], root)

    monkeypatch.setattr(task_manager, "PROJECT_ROOT", root)
    monkeypatch.setattr(task_manager, "AI_TASKS", ai_tasks)
    return root, ai_tasks, worktree_path, task_dir


def _make_dirty(worktree_path):
    (worktree_path / "wip.txt").write_text("uncommitted\n")


def test_clean_clean_worktree_archives_without_flags(monkeypatch, tmp_path):
    task_id = "FEAT-100"
    root, ai_tasks, worktree_path, task_dir = _setup_repo_with_worktree(monkeypatch, tmp_path, task_id)

    task_manager.clean_task(task_id)

    assert not worktree_path.exists()
    assert not task_dir.exists()
    assert (ai_tasks / "done" / f"{task_id}-new-spec").exists()


def test_clean_dirty_no_flags_aborts_with_message(monkeypatch, tmp_path, capsys):
    task_id = "FEAT-101"
    root, ai_tasks, worktree_path, task_dir = _setup_repo_with_worktree(monkeypatch, tmp_path, task_id)
    _make_dirty(worktree_path)

    task_manager.clean_task(task_id)

    out = capsys.readouterr().out
    assert "uncommitted changes" in out
    assert "--force" in out
    assert "--save" in out
    assert worktree_path.exists()
    assert task_dir.exists()
    assert not (ai_tasks / "done" / f"{task_id}-new-spec").exists()


def test_clean_dirty_force_discards_and_archives(monkeypatch, tmp_path):
    task_id = "FEAT-102"
    root, ai_tasks, worktree_path, task_dir = _setup_repo_with_worktree(monkeypatch, tmp_path, task_id)
    _make_dirty(worktree_path)

    task_manager.clean_task(task_id, force=True)

    assert not worktree_path.exists()
    assert (ai_tasks / "done" / f"{task_id}-new-spec").exists()
    stash_list = subprocess.run(
        ["git", "-C", str(root), "stash", "list"],
        capture_output=True, text=True, check=True
    ).stdout
    assert f"quorum:save:{task_id}" not in stash_list


def test_clean_dirty_save_stashes_then_archives(monkeypatch, tmp_path):
    task_id = "FEAT-103"
    root, ai_tasks, worktree_path, task_dir = _setup_repo_with_worktree(monkeypatch, tmp_path, task_id)
    _make_dirty(worktree_path)

    task_manager.clean_task(task_id, save=True)

    assert not worktree_path.exists()
    assert (ai_tasks / "done" / f"{task_id}-new-spec").exists()
    stash_list = subprocess.run(
        ["git", "-C", str(root), "stash", "list"],
        capture_output=True, text=True, check=True
    ).stdout
    assert f"quorum:save:{task_id}" in stash_list


def test_clean_force_and_save_together_aborts(monkeypatch, tmp_path, capsys):
    task_id = "FEAT-104"
    root, ai_tasks, worktree_path, task_dir = _setup_repo_with_worktree(monkeypatch, tmp_path, task_id)
    _make_dirty(worktree_path)

    task_manager.clean_task(task_id, force=True, save=True)

    out = capsys.readouterr().out
    assert "mutually exclusive" in out
    assert worktree_path.exists()
    assert task_dir.exists()


def test_cli_parser_accepts_force_and_save_flags():
    import argparse
    from cli import main as cli_main  # noqa: F401  (ensures import path works)

    parser = argparse.ArgumentParser()
    sub = parser.add_subparsers(dest="subcommand")
    clean_parser = sub.add_parser("clean")
    clean_parser.add_argument("task_id")
    clean_parser.add_argument("--force", action="store_true")
    clean_parser.add_argument("--save", action="store_true")

    args = parser.parse_args(["clean", "FEAT-200", "--force"])
    assert args.force is True
    assert args.save is False

    args = parser.parse_args(["clean", "FEAT-200", "--save"])
    assert args.save is True
    assert args.force is False

    args = parser.parse_args(["clean", "FEAT-200"])
    assert args.force is False
    assert args.save is False
