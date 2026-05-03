"""
Pure-function risk scorer for Quorum blueprints.

Maps a blueprint + risk policy to a discrete level (low/medium/high)
using signal-based detection. No magic numbers, no LLM, no I/O.
"""
from pathlib import PurePath


def assign_risk_level(blueprint: dict, risk_policy: dict) -> dict:
    """
    Assign a risk level based on signals from the blueprint.

    Args:
        blueprint: dict matching blueprint.schema.json (must contain
                   `affected_files` and `symbols`).
        risk_policy: dict matching the structure of risk.yaml (must contain
                     `sensitive_paths` as a list of glob patterns).

    Returns:
        {
            "level": "low" | "medium" | "high",
            "reasons": [str, ...],
            "signals": {
                "files_count": int,
                "symbols_count": int,
                "sensitive_matches": [str, ...]
            }
        }

    Rules (signal-based, deterministic):
      - Any file matching `sensitive_paths` glob → high.
      - Else, >5 affected files OR >2 symbols → medium.
      - Else → low.

    Never overrides human-set risk; this is advisory only.
    """
    affected = list(blueprint.get("affected_files") or [])
    symbols = list(blueprint.get("symbols") or [])
    globs = list(risk_policy.get("sensitive_paths") or [])

    sensitive_hits = [
        f for f in affected
        if any(_safe_glob_match(f, g) for g in globs)
    ]

    signals = {
        "files_count": len(affected),
        "symbols_count": len(symbols),
        "sensitive_matches": sensitive_hits,
    }

    if sensitive_hits:
        return {
            "level": "high",
            "reasons": [f"sensitive_path_match: {sensitive_hits}"],
            "signals": signals,
        }

    reasons: list[str] = []
    if len(affected) > 5:
        reasons.append(f"file_count_high: {len(affected)}")
    if len(symbols) > 2:
        reasons.append(f"symbols_count_high: {len(symbols)}")

    if reasons:
        return {"level": "medium", "reasons": reasons, "signals": signals}
    return {"level": "low", "reasons": ["no_signals_matched"], "signals": signals}


def build_risk_trace_events(declared_risk: str | None, calculated: dict) -> list[dict]:
    """
    Build advisory trace events for the calculated risk result.

    Args:
        declared_risk: human-declared risk from 00-spec.yaml, if any.
        calculated: result returned by assign_risk_level().

    Returns:
        A list containing a mandatory risk_level_calculated event and,
        when the human-declared value differs, a risk_level_divergence event.
    """
    level = calculated["level"]
    reasons = list(calculated.get("reasons") or [])
    signals = dict(calculated.get("signals") or {})

    events = [
        {
            "event": "risk_level_calculated",
            "level": level,
            "reasons": reasons,
            "signals": signals,
        }
    ]

    if declared_risk and declared_risk != level:
        events.append(
            {
                "event": "risk_level_divergence",
                "declared": declared_risk,
                "calculated": level,
                "reasons": reasons,
            }
        )

    return events


def _safe_glob_match(path: str, pattern: str) -> bool:
    """
    Match a path against a policy glob without raising on malformed patterns.

    Empty or invalid patterns are treated as non-matches so policy parsing
    cannot crash advisory scoring.
    """
    if not pattern:
        return False

    try:
        return PurePath(path).match(pattern)
    except ValueError:
        return False
