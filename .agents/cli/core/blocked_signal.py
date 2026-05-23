"""Parser for standardized contract-block signals.

The parseable text form intentionally mirrors the deferred v1.2
renegotiation request shape: path, reason, and severity.
"""

import re

_BLOCKED_SIGNAL_RE = re.compile(
    r"^BLOCKED:\s*"
    r"missing_file=(?P<path>[^;\n]+);\s*"
    r"reason=(?P<reason>[^;\n]+);\s*"
    r"severity=(?P<severity>critical|minor)\s*$"
)


def parse_blocked_signal(message):
    """Parse a standardized BLOCKED contract signal.

    Returns a dict compatible with the future renegotiation-request shape:
    ``{"path": str, "reason": str, "severity": "critical"|"minor"}``.

    Raises:
        ValueError: when the message is not in the standardized format.
    """
    if not isinstance(message, str):
        raise ValueError("blocked signal must be a string")

    match = _BLOCKED_SIGNAL_RE.match(message.strip())
    if not match:
        raise ValueError(
            "blocked signal must match "
            "'BLOCKED: missing_file=<path>; reason=<text>; severity=<critical|minor>'"
        )

    path = match.group("path").strip()
    reason = match.group("reason").strip()
    severity = match.group("severity")
    if not path:
        raise ValueError("blocked signal missing_file must not be empty")
    if not reason:
        raise ValueError("blocked signal reason must not be empty")

    return {"path": path, "reason": reason, "severity": severity}
