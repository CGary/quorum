import os
import sys

import pytest

PROJECT_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
AGENTS_DIR = os.path.join(PROJECT_ROOT, ".agents")
if AGENTS_DIR not in sys.path:
    sys.path.insert(0, AGENTS_DIR)

from cli.core.blocked_signal import parse_blocked_signal


def test_parse_blocked_signal_extracts_future_request_fields():
    parsed = parse_blocked_signal(
        "BLOCKED: missing_file=.agents/cli/core/new_file.py; "
        "reason=contract touch list excludes required file; severity=critical"
    )

    assert parsed == {
        "path": ".agents/cli/core/new_file.py",
        "reason": "contract touch list excludes required file",
        "severity": "critical",
    }


@pytest.mark.parametrize(
    "message",
    [
        "missing_file=.agents/new.py; reason=no prefix; severity=critical",
        "BLOCKED: reason=missing path; severity=critical",
        "BLOCKED: missing_file=.agents/new.py; severity=critical",
        "BLOCKED: missing_file=.agents/new.py; reason=bad severity; severity=high",
        "BLOCKED: missing_file= ; reason=empty path; severity=critical",
        "BLOCKED: missing_file=.agents/new.py; reason= ; severity=critical",
    ],
)
def test_parse_blocked_signal_rejects_malformed_messages(message):
    with pytest.raises(ValueError, match="blocked signal"):
        parse_blocked_signal(message)
