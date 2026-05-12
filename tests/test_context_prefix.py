import os
import re
import sys
from pathlib import Path
from unittest.mock import MagicMock

import pytest

PROJECT_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
AGENTS_DIR = os.path.join(PROJECT_ROOT, ".agents")
if AGENTS_DIR not in sys.path:
    sys.path.insert(0, AGENTS_DIR)

from cli import main as cli_main
from cli.commands import task as task_cmd
from cli.core import task_manager


def _scripted_run(responses):
    """Build a fake subprocess.run that yields the next scripted response per call.

    Each entry is either a string (becomes .stdout) or an Exception (raised).
    """
    state = {"i": 0}

    def fake_run(cmd, check=False, capture_output=False, text=False, **kwargs):
        idx = state["i"]
        state["i"] += 1
        result = responses[idx]
        if isinstance(result, Exception):
            raise result
        return MagicMock(stdout=result, returncode=0)

    return fake_run


def test_get_execution_context_root_when_git_dir_matches_common_dir(monkeypatch, tmp_path):
    git_dir = tmp_path / "repo" / ".git"
    git_dir.mkdir(parents=True)
    monkeypatch.setattr(
        task_manager.subprocess,
        "run",
        _scripted_run([str(git_dir), str(git_dir)]),
    )

    mode, ident = task_manager.get_execution_context()

    assert mode == "root"
    assert ident is None


def test_get_execution_context_worktree_uses_toplevel_basename(monkeypatch, tmp_path):
    common_dir = tmp_path / "repo" / ".git"
    common_dir.mkdir(parents=True)
    git_dir = tmp_path / "repo" / ".git" / "worktrees" / "FEAT-005-d"
    git_dir.mkdir(parents=True)
    toplevel = tmp_path / "repo" / "worktrees" / "FEAT-005-d"
    toplevel.mkdir(parents=True)
    monkeypatch.setattr(
        task_manager.subprocess,
        "run",
        _scripted_run([str(git_dir), str(common_dir), str(toplevel)]),
    )

    mode, ident = task_manager.get_execution_context()

    assert mode == "worktree"
    assert ident == "FEAT-005-d"


def test_get_execution_context_returns_none_when_git_rev_parse_fails(monkeypatch):
    import subprocess as _sp

    monkeypatch.setattr(
        task_manager.subprocess,
        "run",
        _scripted_run([_sp.CalledProcessError(128, ["git", "rev-parse"])]),
    )

    mode, ident = task_manager.get_execution_context()

    assert mode is None
    assert ident is None


def test_render_context_prefix_root_format(monkeypatch):
    monkeypatch.setattr(task_manager, "get_execution_context", lambda: ("root", None))
    assert task_manager.render_context_prefix() == "[root]"


def test_render_context_prefix_worktree_format(monkeypatch):
    monkeypatch.setattr(
        task_manager,
        "get_execution_context",
        lambda: ("worktree", "FEAT-005-d"),
    )
    assert task_manager.render_context_prefix() == "[worktree:FEAT-005-d]"


def test_render_context_prefix_empty_when_context_is_none(monkeypatch):
    monkeypatch.setattr(task_manager, "get_execution_context", lambda: (None, None))
    assert task_manager.render_context_prefix() == ""


def test_main_prints_prefix_as_first_line_before_subcommand_output(monkeypatch, capsys):
    monkeypatch.setattr(task_manager, "render_context_prefix", lambda: "[root]")
    monkeypatch.setattr(task_cmd, "list_all", lambda: print("SUBCOMMAND_OUTPUT_MARKER"))
    monkeypatch.setattr(sys, "argv", ["quorum", "task", "list"])

    cli_main.main()

    out_lines = capsys.readouterr().out.splitlines()
    assert out_lines[0] == "[root]"
    assert "SUBCOMMAND_OUTPUT_MARKER" in out_lines


def test_main_prints_worktree_prefix_with_task_id(monkeypatch, capsys):
    monkeypatch.setattr(
        task_manager,
        "render_context_prefix",
        lambda: "[worktree:FEAT-005-d]",
    )
    monkeypatch.setattr(task_cmd, "list_all", lambda: print("SUBCOMMAND_OUTPUT_MARKER"))
    monkeypatch.setattr(sys, "argv", ["quorum", "task", "list"])

    cli_main.main()

    out_lines = capsys.readouterr().out.splitlines()
    assert out_lines[0] == "[worktree:FEAT-005-d]"


def test_main_omits_prefix_when_git_rev_parse_fails(monkeypatch, capsys):
    monkeypatch.setattr(task_manager, "render_context_prefix", lambda: "")
    monkeypatch.setattr(task_cmd, "list_all", lambda: print("SUBCOMMAND_OUTPUT_MARKER"))
    monkeypatch.setattr(sys, "argv", ["quorum", "task", "list"])

    cli_main.main()

    out_lines = capsys.readouterr().out.splitlines()
    assert out_lines and out_lines[0] == "SUBCOMMAND_OUTPUT_MARKER"


def test_task_manager_functions_do_not_emit_prefix(monkeypatch, capsys):
    """Only cli/main.main() emits the prefix; task_manager internals must not."""
    monkeypatch.setattr(task_manager, "render_context_prefix", lambda: "[root]")

    task_manager.find_task_dir  # touch attribute to confirm import-only side-effect free

    captured = capsys.readouterr().out
    assert "[root]" not in captured
    assert "[worktree:" not in captured


def test_all_ten_skills_mention_context_prefix_in_communication_protocol():
    skills_dir = Path(".agents/skills")
    skill_files = sorted(skills_dir.glob("q-*/SKILL.md"))

    assert len(skill_files) == 10, f"Expected 10 q-* skills, found {len(skill_files)}"

    protocol_header = "Communication Protocol"
    prefix_pattern = re.compile(
        r"\[root\].*\[worktree:",
        re.DOTALL,
    )

    for skill_file in skill_files:
        content = skill_file.read_text()
        assert protocol_header in content, f"Missing Communication Protocol in {skill_file}"

        before_handoff = content.split("## 🛑 Handoff", 1)[0]
        assert "Prefijo de contexto" in before_handoff, (
            f"Missing context prefix convention in Communication Protocol of {skill_file}"
        )
        assert prefix_pattern.search(before_handoff), (
            f"Context prefix convention in {skill_file} must mention both [root] and [worktree:"
        )
