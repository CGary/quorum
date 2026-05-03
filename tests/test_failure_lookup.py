import json
import sys
import os
from pathlib import Path

import pytest
import yaml

# Ensure .agents is in the python path for tests
PROJECT_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
AGENTS_DIR = os.path.join(PROJECT_ROOT, ".agents")
if AGENTS_DIR not in sys.path:
    sys.path.insert(0, AGENTS_DIR)

from cli.core.failure_lookup import find_related_failed_tasks


def _write_failed_task(root: Path, task_id: str, files: list[str], *, validation=None, review=None):
    d = root / "failed" / task_id
    d.mkdir(parents=True)
    (d / "01-blueprint.yaml").write_text(yaml.safe_dump({
        "task_id": task_id,
        "summary": "fixture",
        "affected_files": files,
        "symbols": [],
        "dependencies": [],
        "test_scenarios": ["x"],
    }))
    if validation is not None:
        (d / "05-validation.json").write_text(json.dumps(validation))
    if review is not None:
        (d / "06-review.json").write_text(json.dumps(review))


def test_returns_empty_when_failed_dir_missing(tmp_path):
    bp = {"affected_files": ["src/a.py"]}
    assert find_related_failed_tasks(bp, tmp_path) == []


def test_returns_empty_when_no_overlap(tmp_path):
    _write_failed_task(tmp_path, "OLD-001", ["src/x.py", "src/y.py"])
    bp = {"affected_files": ["src/a.py"]}
    assert find_related_failed_tasks(bp, tmp_path) == []


def test_returns_match_on_high_overlap(tmp_path):
    _write_failed_task(
        tmp_path, "OLD-002", ["src/a.py", "src/b.py"],
        validation={"task_id": "OLD-002", "summary": "x", "executed_at": "2026-05-03T00:00:00Z",
                    "commands": [{"command": "pytest", "exit_code": 1, "duration_s": 1.0,
                                  "output_excerpt": "AssertionError"}],
                    "overall_result": "failed"},
        review={"task_id": "OLD-002", "summary": "x", "verdict": "reject",
                "contract_compliance": False, "forbidden_files_touched": [],
                "unrequested_refactor": False, "missing_tests": [],
                "functional_risk": "high", "notes": ["see issue"],
                "fix_tasks": [{"slug": "patch-a", "scope": "src/a.py"}]},
    )
    bp = {"affected_files": ["src/a.py", "src/b.py"]}
    out = find_related_failed_tasks(bp, tmp_path)
    assert len(out) == 1
    assert out[0]["task_id"] == "OLD-002"
    assert out[0]["overlap_ratio"] == 1.0
    assert "AssertionError" in out[0]["validation_excerpt"]
    assert out[0]["fix_tasks"][0]["slug"] == "patch-a"
    assert out[0]["notes"] == ["see issue"]


def test_skips_below_threshold(tmp_path):
    _write_failed_task(tmp_path, "OLD-003", ["src/a.py", "src/b.py", "src/c.py", "src/d.py"])
    bp = {"affected_files": ["src/a.py", "src/z.py"]}
    # overlap = 1, union = 5, ratio = 0.20 -> below 0.50
    assert find_related_failed_tasks(bp, tmp_path) == []


def test_sorted_by_overlap_desc(tmp_path):
    _write_failed_task(tmp_path, "OLD-A", ["src/a.py"])
    _write_failed_task(tmp_path, "OLD-B", ["src/a.py", "src/b.py"])
    bp = {"affected_files": ["src/a.py", "src/b.py"]}
    out = find_related_failed_tasks(bp, tmp_path)
    assert [r["task_id"] for r in out] == ["OLD-B", "OLD-A"]


def test_purity_no_input_mutation(tmp_path):
    _write_failed_task(tmp_path, "OLD-X", ["src/a.py"])
    bp = {"affected_files": ["src/a.py"]}
    snapshot = {"affected_files": list(bp["affected_files"])}
    find_related_failed_tasks(bp, tmp_path)
    assert bp == snapshot


def test_handles_corrupt_yaml_gracefully(tmp_path):
    d = tmp_path / "failed" / "OLD-CORRUPT"
    d.mkdir(parents=True)
    (d / "01-blueprint.yaml").write_text("::not yaml::\n  - [")
    bp = {"affected_files": ["src/a.py"]}
    # should not raise; corrupt task is skipped
    assert find_related_failed_tasks(bp, tmp_path) == []
