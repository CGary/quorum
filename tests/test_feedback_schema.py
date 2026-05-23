import json
from pathlib import Path

import pytest
from jsonschema import validate, ValidationError


SCHEMA = json.loads(Path(".agents/schemas/feedback.schema.json").read_text())


VALID_FEEDBACK = {
    "task_id": "FEAT-008",
    "summary": "q-analyze found mechanical planning feedback.",
    "produced_by": "q-analyze",
    "generated_at": "2026-05-22T20:00:00Z",
    "findings": [
        {
            "severity": "medium",
            "category": "mechanical",
            "artifact": "01-blueprint.yaml",
            "path": "$.affected_files[0]",
            "issue": "Broken file reference.",
            "suggested_fix": "Use the current path.",
        }
    ],
}


def test_feedback_schema_accepts_valid_payload():
    validate(instance=VALID_FEEDBACK, schema=SCHEMA)


def test_feedback_schema_rejects_missing_required_field():
    payload = dict(VALID_FEEDBACK)
    payload.pop("produced_by")

    with pytest.raises(ValidationError):
        validate(instance=payload, schema=SCHEMA)


def test_feedback_schema_rejects_non_analyze_producer():
    payload = dict(VALID_FEEDBACK)
    payload["produced_by"] = "q-review"

    with pytest.raises(ValidationError):
        validate(instance=payload, schema=SCHEMA)


def test_feedback_schema_rejects_extra_finding_properties():
    payload = json.loads(json.dumps(VALID_FEEDBACK))
    payload["findings"][0]["auto_apply"] = True

    with pytest.raises(ValidationError):
        validate(instance=payload, schema=SCHEMA)
