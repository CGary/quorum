import pytest
import sys
import os

# Ensure .agents is in the python path for tests
PROJECT_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
AGENTS_DIR = os.path.join(PROJECT_ROOT, ".agents")
if AGENTS_DIR not in sys.path:
    sys.path.insert(0, AGENTS_DIR)

from cli.core.risk_scorer import assign_risk_level, build_risk_trace_events


SAMPLE_POLICY = {
    "sensitive_paths": [
        "**/auth/**",
        "**/payment/**",
        "**/migrations/**",
        "*.lock",
    ]
}


def _bp(files=None, symbols=None):
    return {
        "task_id": "TEST-001",
        "summary": "test",
        "affected_files": files or [],
        "symbols": symbols or [],
        "dependencies": [],
        "test_scenarios": ["covers something"],
    }


def test_low_when_no_signals():
    out = assign_risk_level(_bp(files=["src/util.py"]), SAMPLE_POLICY)
    assert out["level"] == "low"
    assert out["reasons"] == ["no_signals_matched"]


def test_high_on_sensitive_match():
    out = assign_risk_level(_bp(files=["src/auth/login.py"]), SAMPLE_POLICY)
    assert out["level"] == "high"
    assert "sensitive_path_match" in out["reasons"][0]


def test_high_on_lockfile_match():
    out = assign_risk_level(_bp(files=["package-lock.lock"]), SAMPLE_POLICY)
    assert out["level"] == "high"


def test_medium_on_file_count():
    files = [f"src/feature/m{i}.py" for i in range(6)]
    out = assign_risk_level(_bp(files=files), SAMPLE_POLICY)
    assert out["level"] == "medium"
    assert any("file_count_high" in r for r in out["reasons"])


def test_medium_on_symbol_count():
    out = assign_risk_level(
        _bp(files=["src/api.py"], symbols=["A", "B", "C"]),
        SAMPLE_POLICY,
    )
    assert out["level"] == "medium"
    assert any("symbols_count_high" in r for r in out["reasons"])


def test_sensitive_overrides_volume():
    files = ["src/auth/x.py"] + [f"docs/d{i}.md" for i in range(20)]
    out = assign_risk_level(_bp(files=files), SAMPLE_POLICY)
    assert out["level"] == "high"


def test_empty_blueprint_is_low():
    out = assign_risk_level(_bp(), SAMPLE_POLICY)
    assert out["level"] == "low"


def test_purity_no_mutation():
    bp = _bp(files=["src/auth/x.py"])
    snapshot = dict(bp)
    assign_risk_level(bp, SAMPLE_POLICY)
    assert bp == snapshot  # function did not mutate inputs


def test_invalid_empty_glob_is_ignored():
    policy = {"sensitive_paths": ["", "**/auth/**"]}
    out = assign_risk_level(_bp(files=["src/auth/login.py"]), policy)
    assert out["level"] == "high"


def test_divergence_event_when_declared_differs():
    calculated = assign_risk_level(_bp(files=["src/auth/login.py"]), SAMPLE_POLICY)
    events = build_risk_trace_events("low", calculated)

    assert events[0]["event"] == "risk_level_calculated"
    assert events[1] == {
        "event": "risk_level_divergence",
        "declared": "low",
        "calculated": "high",
        "reasons": calculated["reasons"],
    }


def test_no_divergence_event_when_declared_matches():
    calculated = assign_risk_level(_bp(files=["src/util.py"]), SAMPLE_POLICY)
    events = build_risk_trace_events("low", calculated)

    assert len(events) == 1
    assert events[0]["event"] == "risk_level_calculated"
