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
VALID_BLUEPRINT = {
    "task_id": "FEAT-001",
    "summary": "valid blueprint",
    "affected_files": ["src/a.py"],
    "symbols": [],
    "dependencies": [],
    "test_scenarios": ["works"],
}
VALID_TRACE = {
    "task_id": "FEAT-001",
    "summary": "trace",
    "started_at": "2026-05-03T00:00:00Z",
    "execution_mode": "worktree_edit",
    "attempts": [
        {"phase": "blueprint", "result": "passed", "duration_s": 1.0}
    ],
    "total_cost_usd": 0.0,
    "violations": [],
    "context_overflows": [],
}
def _setup_repo(monkeypatch, tmp_path):
    root = tmp_path / "repo"
    ai_tasks = root / ".ai" / "tasks"
    for loc in ["inbox", "active", "done", "failed"]:
        (ai_tasks / loc).mkdir(parents=True, exist_ok=True)
    (root / "worktrees").mkdir(parents=True, exist_ok=True)
    (root / "memory" / "patterns").mkdir(parents=True, exist_ok=True)
    monkeypatch.setattr(task_manager, "PROJECT_ROOT", root)
    monkeypatch.setattr(task_manager, "AI_TASKS", ai_tasks)
    return root, ai_tasks
def test_cli_artifact_save_rejects_invalid_blueprint_and_preserves_existing(monkeypatch, tmp_path, capsys):
    root, ai_tasks = _setup_repo(monkeypatch, tmp_path)
    task_dir = ai_tasks / "active" / "FEAT-001-new-spec"
    task_dir.mkdir(parents=True, exist_ok=True)
    (task_dir / "00-spec.yaml").write_text(yaml.safe_dump({
        "task_id": "FEAT-001",
        "summary": "x",
        "goal": "goal long enough",
        "invariants": ["Invariant."],
        "acceptance": ["Acceptance."],
    }, sort_keys=False))
    blueprint_path = task_dir / "01-blueprint.yaml"
    blueprint_path.write_text(yaml.safe_dump(VALID_BLUEPRINT, sort_keys=False))
    monkeypatch.setattr(sys, "stdin", io.StringIO("task_id: FEAT-001\nsummary: broken\naffected_files: []\n"))
    with pytest.raises(SystemExit) as exc:
        task.artifact_save("FEAT-001", "01-blueprint.yaml")
    assert exc.value.code == 1
    out = capsys.readouterr().out
    assert "artifact=" in out
    assert "01-blueprint.yaml" in out
    assert "field=$" in out or "field=$." in out
    assert "reason=" in out
    assert yaml.safe_load(blueprint_path.read_text()) == VALID_BLUEPRINT
def test_save_artifact_rejects_invalid_validation_and_review_json(monkeypatch, tmp_path):
    root, _ = _setup_repo(monkeypatch, tmp_path)
    invalid_validation = {
        "task_id": "FEAT-001",
        "summary": "invalid",
        "executed_at": "2026-05-03T00:00:00Z",
        "commands": [],
        "overall_result": "passed",
    }
    with pytest.raises(task_manager.ArtifactValidationError):
        task_manager.save_artifact(root / "05-validation.json", invalid_validation)
    assert not (root / "05-validation.json").exists()
    invalid_review = {
        "task_id": "FEAT-001",
        "summary": "invalid",
        "verdict": "approve",
        "contract_compliance": True,
        "forbidden_files_touched": [],
        "unrequested_refactor": False,
        "missing_tests": [],
        "functional_risk": "medium",
    }
    with pytest.raises(task_manager.ArtifactValidationError):
        task_manager.save_artifact(root / "06-review.json", invalid_review)
    assert not (root / "06-review.json").exists()
def test_save_artifact_rejects_invalid_memory_entry(monkeypatch, tmp_path):
    root, _ = _setup_repo(monkeypatch, tmp_path)
    invalid_memory = {
        "id": "PAT-2026-05-04-1",
        "source_task": "FEAT-001",
        "type": "pattern",
        "title": "bad",
        "context": "ctx",
        "content": "too short",
        "created_at": "2026-05-04",
    }
    with pytest.raises(task_manager.ArtifactValidationError):
        task_manager.save_artifact(root / "memory" / "patterns" / "PAT-2026-05-04-1.json", invalid_memory)
def test_trace_append_only_preserves_existing_attempts(monkeypatch, tmp_path):
    root, _ = _setup_repo(monkeypatch, tmp_path)
    trace_path = root / "07-trace.json"
    task_manager.save_artifact(trace_path, VALID_TRACE)
    updated = json.loads(json.dumps(VALID_TRACE))
    updated["attempts"].append({"phase": "execute", "result": "passed", "duration_s": 2.0})
    updated["total_cost_usd"] = 1.25
    task_manager.save_artifact(trace_path, updated)
    persisted = json.loads(trace_path.read_text())
    assert persisted["attempts"][0] == VALID_TRACE["attempts"][0]
    assert len(persisted["attempts"]) == 2
def test_trace_append_only_rejects_mutation(monkeypatch, tmp_path):
    root, _ = _setup_repo(monkeypatch, tmp_path)
    trace_path = root / "07-trace.json"
    task_manager.save_artifact(trace_path, VALID_TRACE)
    mutated = json.loads(json.dumps(VALID_TRACE))
    mutated["attempts"][0]["result"] = "failed"
    with pytest.raises(task_manager.ArtifactValidationError):
        task_manager.save_artifact(trace_path, mutated)
    persisted = json.loads(trace_path.read_text())
    assert persisted == VALID_TRACE
def test_initialize_and_start_keep_spec_and_contract_validation_paths(monkeypatch, tmp_path):
    root, ai_tasks = _setup_repo(monkeypatch, tmp_path)
    task_dir = task_manager.initialize_specify("FEAT-010")
    spec = yaml.safe_load((task_dir / "00-spec.yaml").read_text())
    assert spec["task_id"] == "FEAT-010"
    active_dir = ai_tasks / "active" / "FEAT-010-new-spec"
    task_dir.rename(active_dir)
    contract = {
        "task_id": "FEAT-010",
        "summary": "valid contract",
        "goal": "Implement enough detail to validate contract start.",
        "read": [".agents/cli/core/task_manager.py"],
        "touch": [".agents/cli/core/task_manager.py"],
        "forbid": {"files": [], "behaviors": []},
        "verify": {"commands": ["pytest"]},
        "limits": {"max_files_changed": 1, "max_diff_lines": 20},
        "execution": {"mode": "worktree_edit"},
        "retry_policy": {"max_attempts": 1},
    }
    (active_dir / "02-contract.yaml").write_text(yaml.safe_dump(contract, sort_keys=False))
    called = {}
    def fake_run(args, **kwargs):
        if args[:3] == ["git", "worktree", "add"]:
            called["worktree_add"] = True
            return type("Result", (), {"returncode": 0, "stdout": b"", "stderr": b""})()
        if args[:3] == ["git", "rev-parse", "--verify"]:
            return type("Result", (), {"returncode": 0, "stdout": b"", "stderr": b""})()
        if args[:3] == ["git", "rev-parse", "--abbrev-ref"]:
            return type("Result", (), {"returncode": 0, "stdout": "main\n", "stderr": ""})()
        raise AssertionError(args)
    monkeypatch.setattr(task_manager, "get_base_branch", lambda: "main")
    monkeypatch.setattr(task_manager.subprocess, "run", fake_run)
    task_manager.start_task("FEAT-010")
    trace = json.loads((active_dir / "07-trace.json").read_text())
    assert trace["task_id"] == "FEAT-010"
    assert called["worktree_add"] is True
