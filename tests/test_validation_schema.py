import json
from pathlib import Path

import pytest
from jsonschema import validate, ValidationError

SCHEMA_PATH = Path(".agents/schemas/validation.schema.json")
SCHEMA = json.loads(SCHEMA_PATH.read_text())


def _base():
    return {
        "task_id": "TEST-001",
        "summary": "x",
        "executed_at": "2026-05-03T00:00:00Z",
        "commands": [
            {"command": "pytest", "exit_code": 0, "duration_s": 1.0, "output_excerpt": ""}
        ],
        "overall_result": "passed",
    }


def test_valid_without_error_category():
    validate(_base(), SCHEMA)  # backward compatibility


def test_valid_with_each_error_category():
    for cat in ["logic", "dependency", "environment", "flaky", "unknown"]:
        d = _base() | {"error_category": cat, "overall_result": "failed"}
        validate(d, SCHEMA)


def test_rejects_invalid_error_category():
    d = _base() | {"error_category": "made_up", "overall_result": "failed"}
    with pytest.raises(ValidationError):
        validate(d, SCHEMA)


def test_rejects_non_string_error_category():
    d = _base() | {"error_category": 42, "overall_result": "failed"}
    with pytest.raises(ValidationError):
        validate(d, SCHEMA)
