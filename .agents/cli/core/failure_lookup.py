"""
Pure-function lookup of related failed tasks for blueprint context.

Reads `.ai/tasks/failed/*` and returns failed tasks whose blueprint overlaps
significantly with the new blueprint's affected_files. No I/O beyond reads.
No LLM. No state mutation.
"""
from __future__ import annotations
import json
from pathlib import Path

import yaml


OVERLAP_THRESHOLD = 0.50


def find_related_failed_tasks(
    blueprint: dict,
    ai_tasks_root: Path,
) -> list[dict]:
    """
    Return failed tasks whose blueprint touches at least 50% of the same files
    as the new blueprint.

    Args:
        blueprint: dict matching blueprint.schema.json.
        ai_tasks_root: path to .ai/tasks (will read <root>/failed/*).

    Returns:
        List of dicts:
            {
              "task_id": str,
              "overlap_files": [str, ...],
              "overlap_ratio": float,
              "validation_excerpt": str | None,
              "fix_tasks": [dict],
              "notes": [str],
            }
        Sorted by overlap_ratio descending. Empty list if failed/ is missing.
    """
    new_files = set(blueprint.get("affected_files") or [])
    if not new_files:
        return []

    failed_root = Path(ai_tasks_root) / "failed"
    if not failed_root.exists():
        return []

    results: list[dict] = []

    for task_dir in sorted(failed_root.iterdir()):
        if not task_dir.is_dir():
            continue

        bp_path = task_dir / "01-blueprint.yaml"
        if not bp_path.exists():
            continue

        try:
            failed_bp = yaml.safe_load(bp_path.read_text()) or {}
        except yaml.YAMLError:
            continue

        failed_files = set(failed_bp.get("affected_files") or [])
        if not failed_files:
            continue

        overlap = new_files & failed_files
        ratio = len(overlap) / len(new_files | failed_files)
        if ratio < OVERLAP_THRESHOLD:
            continue

        results.append({
            "task_id": failed_bp.get("task_id", task_dir.name),
            "overlap_files": sorted(overlap),
            "overlap_ratio": round(ratio, 3),
            "validation_excerpt": _read_validation_excerpt(task_dir),
            "fix_tasks": _read_fix_tasks(task_dir),
            "notes": _read_review_notes(task_dir),
        })

    results.sort(key=lambda r: r["overlap_ratio"], reverse=True)
    return results


def _read_validation_excerpt(task_dir: Path) -> str | None:
    path = task_dir / "05-validation.json"
    if not path.exists():
        return None
    try:
        data = json.loads(path.read_text())
    except json.JSONDecodeError:
        return None
    failed = [c for c in data.get("commands", []) if c.get("exit_code") != 0]
    if not failed:
        return None
    first = failed[0]
    return f"{first.get('command')!r} exited {first.get('exit_code')}: {first.get('output_excerpt', '')[:500]}"


def _read_fix_tasks(task_dir: Path) -> list[dict]:
    path = task_dir / "06-review.json"
    if not path.exists():
        return []
    try:
        data = json.loads(path.read_text())
    except json.JSONDecodeError:
        return []
    return list(data.get("fix_tasks") or [])


def _read_review_notes(task_dir: Path) -> list[str]:
    path = task_dir / "06-review.json"
    if not path.exists():
        return []
    try:
        data = json.loads(path.read_text())
    except json.JSONDecodeError:
        return []
    return list(data.get("notes") or [])
