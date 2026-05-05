import pytest
from pathlib import Path
import yaml
import sys
import os

# Quorum test pattern: ensure .agents is in sys.path
PROJECT_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
AGENTS_DIR = os.path.join(PROJECT_ROOT, ".agents")
if AGENTS_DIR not in sys.path:
    sys.path.insert(0, AGENTS_DIR)

from cli.core import task_manager

@pytest.fixture
def mock_project(tmp_path):
    # Setup a mock Quorum project structure
    ai_tasks = tmp_path / ".ai" / "tasks"
    active = ai_tasks / "active"
    active.mkdir(parents=True)
    
    # Copy schemas from the worktree to tmp_path
    schemas_dir = tmp_path / ".agents" / "schemas"
    schemas_dir.mkdir(parents=True)
    
    # In the test environment, we need to point task_manager to these schemas
    # But task_manager uses a fixed path relative to its __file__.
    # We might need to monkeypatch task_manager.SCHEMAS_DIR or similar if possible.
    # However, for this test, let's just ensure the schemas exist where task_manager expects them
    # OR monkeypatch get_project_root.
    
    current_schemas = Path(__file__).parent.parent / ".agents" / "schemas"
    for schema_file in current_schemas.glob("*.json"):
        (schemas_dir / schema_file.name).write_text(schema_file.read_text())
        
    return tmp_path

def test_start_task_initializes_implementation_log(tmp_path, monkeypatch):
    monkeypatch.setattr(task_manager, "PROJECT_ROOT", tmp_path)
    monkeypatch.setattr(task_manager, "AI_TASKS", tmp_path / ".ai" / "tasks")
    # Point SCHEMAS_DIR to the schemas in the worktree for validation during test
    monkeypatch.setattr(task_manager, "SCHEMAS_DIR", Path(__file__).parent.parent / ".agents" / "schemas")
    
    ai_tasks = tmp_path / ".ai" / "tasks"
    active_dir = ai_tasks / "active" / "FEAT-001"
    active_dir.mkdir(parents=True)
    
    # Create required artifacts 00 and 02
    spec = {
        "task_id": "FEAT-001",
        "summary": "Test task",
        "goal": "Test goal",
        "invariants": ["Invar"],
        "acceptance": ["Accept"]
    }
    with open(active_dir / "00-spec.yaml", "w") as f:
        yaml.safe_dump(spec, f)
        
    contract = {
        "task_id": "FEAT-001",
        "summary": "Test contract summary",
        "goal": "Test goal must be at least 10 chars.",
        "read": ["file.py"],
        "touch": ["file.py"],
        "forbid": {"files": [], "behaviors": []},
        "verify": {"commands": ["ls"]},
        "limits": {"max_files_changed": 1, "max_diff_lines": 10},
        "execution": {"mode": "patch_only"},
        "retry_policy": {"max_attempts": 1}
    }
    with open(active_dir / "02-contract.yaml", "w") as f:
        yaml.safe_dump(contract, f)
        
    # Mock get_base_branch to avoid git calls
    monkeypatch.setattr(task_manager, "get_base_branch", lambda: "main")
    # Mock subprocess.run for git worktree add
    class MockRes:
        returncode = 0
        stderr = b""
        stdout = b""
    monkeypatch.setattr("subprocess.run", lambda *args, **kwargs: MockRes())

    # Run start_task
    task_manager.start_task("FEAT-001")
    
    # Verify 04-implementation-log.yaml exists and is valid
    log_path = active_dir / "04-implementation-log.yaml"
    assert log_path.exists()
    
    with open(log_path, "r") as f:
        log = yaml.safe_load(f)
        
    assert log["task_id"] == "FEAT-001"
    assert log["summary"] == "Test contract summary"
    assert log["entries"] == []
    
    # Validation against schema (using task_manager helper)
    task_manager._validate_implementation_log(log)

def test_start_task_does_not_overwrite_existing_log(tmp_path, monkeypatch):
    monkeypatch.setattr(task_manager, "PROJECT_ROOT", tmp_path)
    monkeypatch.setattr(task_manager, "AI_TASKS", tmp_path / ".ai" / "tasks")
    monkeypatch.setattr(task_manager, "SCHEMAS_DIR", Path(__file__).parent.parent / ".agents" / "schemas")
    
    ai_tasks = tmp_path / ".ai" / "tasks"
    active_dir = ai_tasks / "active" / "FEAT-001"
    active_dir.mkdir(parents=True)
    
    with open(active_dir / "00-spec.yaml", "w") as f:
        yaml.safe_dump({"task_id": "FEAT-001", "summary": "S", "goal": "Goal must be at least 10 chars.", "invariants": ["I"], "acceptance": ["A"]}, f)
    with open(active_dir / "02-contract.yaml", "w") as f:
        yaml.safe_dump({
            "task_id": "FEAT-001",
            "summary": "S",
            "goal": "Goal must be at least 10 chars.",
            "read": [],
            "touch": ["file.py"],
            "forbid": {"files": [], "behaviors": []},
            "verify": {"commands": ["ls"]},
            "limits": {"max_files_changed": 1, "max_diff_lines": 10},
            "execution": {"mode": "patch_only"},
            "retry_policy": {"max_attempts": 1}
        }, f)
        
    # Create an existing log with content
    log_path = active_dir / "04-implementation-log.yaml"
    existing_log = {
        "task_id": "FEAT-001",
        "summary": "Original summary",
        "entries": [{"changed_files": ["a.py"], "notes": ["note"], "verify_pending": True}]
    }
    with open(log_path, "w") as f:
        yaml.safe_dump(existing_log, f)
        
    monkeypatch.setattr(task_manager, "get_base_branch", lambda: "main")
    monkeypatch.setattr("subprocess.run", lambda *args, **kwargs: None)

    # Run start_task
    task_manager.start_task("FEAT-001")
    
    # Verify it was not overwritten
    with open(log_path, "r") as f:
        log = yaml.safe_load(f)
    assert log["summary"] == "Original summary"
    assert len(log["entries"]) == 1
