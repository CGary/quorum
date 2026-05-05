import subprocess
import sys
import os
import pytest
from pathlib import Path

def test_task_run_not_in_help():
    """Verify 'run' is not in 'quorum task --help' output."""
    env = os.environ.copy()
    env["PYTHONPATH"] = str(Path(__file__).parent.parent / ".agents")
    result = subprocess.run(
        [sys.executable, "-m", "cli.main", "task", "--help"],
        capture_output=True,
        text=True,
        env=env
    )
    assert result.returncode == 0
    # argparse usually uses "run" if it exists, or omits it if not.
    assert " run " not in result.stdout

def test_task_run_fails_with_invalid_subcommand():
    """Verify 'quorum task run <ID>' exits non-zero as an invalid choice."""
    env = os.environ.copy()
    env["PYTHONPATH"] = str(Path(__file__).parent.parent / ".agents")
    result = subprocess.run(
        [sys.executable, "-m", "cli.main", "task", "run", "FEAT-001"],
        capture_output=True,
        text=True,
        env=env
    )
    # Argparse exits with 2 for invalid choices
    assert result.returncode != 0
    # Check that it mentions 'run' is invalid or not a choice
    assert "invalid choice: 'run'" in result.stderr or "argument subcommand: invalid choice: 'run'" in result.stderr

def test_task_run_no_side_effects(tmp_path, monkeypatch):
    """Verify that attempting to call 'run' doesn't touch any task artifacts."""
    # We use a dummy project layout
    repo = tmp_path / "repo"
    ai_tasks = repo / ".ai" / "tasks"
    active = ai_tasks / "active"
    task_dir = active / "FEAT-001-test"
    task_dir.mkdir(parents=True)
    (task_dir / "00-spec.yaml").write_text("task_id: FEAT-001\nsummary: test")
    
    env = os.environ.copy()
    env["PYTHONPATH"] = str(Path(__file__).parent.parent / ".agents")
    
    result = subprocess.run(
        [sys.executable, "-m", "cli.main", "task", "run", "FEAT-001"],
        capture_output=True,
        text=True,
        env=env,
        cwd=repo # Run inside the dummy repo
    )
    
    assert result.returncode != 0
    # Verify no new files were created in the task directory
    files = list(task_dir.iterdir())
    assert len(files) == 1
    assert files[0].name == "00-spec.yaml"
