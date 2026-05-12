"""Read-only parent/child decomposition coverage helpers for q-analyze."""
from __future__ import annotations

import json
import re
from pathlib import Path
from typing import Any, Iterable, Mapping

import yaml
from jsonschema import Draft202012Validator

LOCATIONS = ("inbox", "active", "done", "failed")
PARENT_ID_RE = re.compile(r"^[A-Z]+-[0-9]+$")


def analyze_parent_child_coverage(parent_spec_path, ai_tasks_root, spec_schema_path=None):
    """Return coverage/findings for a decomposed parent without mutating state."""
    parent_spec_path = Path(parent_spec_path)
    ai_tasks_root = Path(ai_tasks_root)
    schema = _schema(spec_schema_path)
    parent, error = _load_spec(parent_spec_path, schema)
    if error:
        finding = _finding("critical", parent_spec_path.as_posix(), f"Parent spec could not be loaded: {error}")
        return _result(False, "blocked", findings=[finding])

    parent_id = str(parent.get("task_id") or parent_spec_path.parent.name)
    decomposition = parent.get("decomposition") or []
    if not decomposition:
        out = _result(False, "not_decomposed")
        out.update(parent_task_id=parent_id, parent_spec_path=parent_spec_path.as_posix())
        return out
    if not isinstance(decomposition, list):
        finding = _finding("critical", "00-spec.yaml.decomposition", "Expected decomposition to be a list.")
        out = _result(True, "blocked", findings=[finding])
        out.update(parent_task_id=parent_id, parent_spec_path=parent_spec_path.as_posix())
        return out

    entries, findings = _entries(decomposition)
    declared = {entry["child_id"] for entry in entries}
    children: dict[str, dict[str, Any]] = {}
    loaded: dict[str, Mapping[str, Any]] = {}

    for entry in entries:
        child_id = entry["child_id"]
        expected = list(entry.get("depends_on") or [])
        path, resolver_error = _find_spec_path(child_id, ai_tasks_root)
        if resolver_error or path is None:
            issue = resolver_error or f"Child {child_id} is declared but no 00-spec.yaml was found."
            children[child_id] = {"status": "missing", "path": None, "error": issue, "depends_on": expected}
            findings.append(_finding("high", f"00-spec.yaml.decomposition[{child_id}]", issue))
            continue

        spec, error = _load_spec(path, schema)
        children[child_id] = {"status": "invalid" if error else "loaded", "path": path.as_posix(), "error": error, "depends_on": expected}
        if error:
            findings.append(_finding("high", path.as_posix(), f"Child {child_id} has an invalid 00-spec.yaml: {error}"))
            continue

        loaded[child_id] = spec
        if spec.get("parent_task") != parent_id:
            findings.append(_finding("high", f"{path.as_posix()}.parent_task", f"Child {child_id} references parent_task={spec.get('parent_task')!r}; expected {parent_id!r}."))
        actual = list(spec.get("depends_on") or [])
        unknown = sorted(dep for dep in actual if dep not in declared)
        if unknown:
            findings.append(_finding("medium", f"{path.as_posix()}.depends_on", f"Child {child_id} depends on undeclared sibling(s): {', '.join(unknown)}."))
        if set(actual) != set(expected):
            findings.append(_finding("medium", f"{path.as_posix()}.depends_on", f"Child {child_id} depends_on {sorted(actual)} does not match parent decomposition {sorted(expected)}."))

    for entry in entries:
        for dep in entry.get("depends_on") or []:
            if dep not in declared:
                findings.append(_finding("medium", f"00-spec.yaml.decomposition[{entry['child_id']}].depends_on", f"Child {entry['child_id']} depends on undeclared sibling {dep}."))

    coverage = {
        "invariants": _coverage_for_items(parent.get("invariants") or [], loaded, "invariants"),
        "acceptance": _coverage_for_items(parent.get("acceptance") or [], loaded, "acceptance"),
    }
    gaps = _gaps(coverage)
    findings.extend(gaps)
    return {
        "applies": True,
        "status": "issues_found" if findings else "pass",
        "parent_task_id": parent_id,
        "parent_spec_path": parent_spec_path.as_posix(),
        "children": children,
        "coverage": coverage,
        "gaps": gaps,
        "inconsistencies": [finding for finding in findings if finding not in gaps],
        "findings": findings,
    }


def load_parent_child_specs(parent_spec_path, ai_tasks_root, spec_schema_path=None):
    """Return parent/child load state without coverage rows."""
    result = analyze_parent_child_coverage(parent_spec_path, ai_tasks_root, spec_schema_path)
    return {key: result.get(key) for key in ("applies", "status", "parent_task_id", "parent_spec_path", "children", "inconsistencies")}


def _coverage_for_items(parent_items: Iterable[str], child_specs: Mapping[str, Mapping[str, Any]], field: str):
    rows = []
    for item in map(str, parent_items):
        covered_by = [child_id for child_id, spec in sorted(child_specs.items()) if any(_covers(item, str(child_item)) for child_item in spec.get(field) or [])]
        rows.append({"item": item, "covered_by": covered_by})
    return rows


def _gaps(coverage):
    gaps = []
    for field, rows in coverage.items():
        for index, row in enumerate(rows):
            if not row["covered_by"]:
                name = "invariant" if field == "invariants" else "acceptance"
                severity = "medium" if field == "invariants" else "high"
                gaps.append(_finding(severity, f"00-spec.yaml.{field}[{index}]", f"No child spec covers parent {name}: {row['item']}"))
    return gaps


def _entries(decomposition):
    entries, findings, seen = [], [], set()
    for index, entry in enumerate(decomposition):
        artifact = f"00-spec.yaml.decomposition[{index}]"
        if not isinstance(entry, Mapping):
            findings.append(_finding("high", artifact, "Expected decomposition entry to be an object.")); continue
        child_id = entry.get("child_id")
        if not child_id:
            findings.append(_finding("high", artifact, "Missing child_id.")); continue
        child_id = str(child_id)
        if child_id in seen:
            findings.append(_finding("high", artifact, f"Duplicate child_id {child_id}.")); continue
        seen.add(child_id)
        entries.append({"child_id": child_id, "depends_on": [str(dep) for dep in entry.get("depends_on") or []]})
    return entries, findings


def _find_spec_path(task_id: str, ai_tasks_root: Path):
    yaml_matches, exact_matches, prefix_matches = [], [], []
    for location in LOCATIONS:
        root = ai_tasks_root / location
        if not root.exists():
            continue
        for task_dir in sorted(path for path in root.iterdir() if path.is_dir()):
            spec_path = task_dir / "00-spec.yaml"
            spec_id = _read_task_id(spec_path)
            if spec_id == task_id:
                yaml_matches.append(spec_path); continue
            if task_dir.name == task_id:
                exact_matches.append(spec_path); continue
            if task_dir.name.startswith(f"{task_id}-"):
                rest = task_dir.name[len(task_id) + 1:]
                if not (PARENT_ID_RE.match(task_id) and _child_suffix(rest)):
                    prefix_matches.append(spec_path)
    matches = yaml_matches or exact_matches or prefix_matches
    if len(matches) > 1:
        return None, f"Multiple task directories match {task_id}: {', '.join(path.parent.as_posix() for path in matches)}."
    return (matches[0], None) if matches else (None, None)


def _read_task_id(spec_path: Path):
    if not spec_path.exists():
        return None
    try:
        spec = yaml.safe_load(spec_path.read_text())
    except (OSError, yaml.YAMLError):
        return None
    return str(spec.get("task_id")) if isinstance(spec, Mapping) and spec.get("task_id") else None


def _load_spec(path: Path, schema):
    try:
        spec = yaml.safe_load(path.read_text())
    except OSError as error:
        return None, str(error)
    except yaml.YAMLError as error:
        return None, str(error)
    if not isinstance(spec, Mapping):
        return None, "spec root is not an object"
    if schema is not None:
        errors = sorted(Draft202012Validator(schema).iter_errors(spec), key=lambda err: list(err.path))
        if errors:
            first = errors[0]
            field = ".".join(str(part) for part in first.path) or "$"
            return spec, f"field={field}; reason={first.message}"
    return spec, None


def _schema(path):
    if path is None:
        return None
    try:
        return json.loads(Path(path).read_text())
    except (OSError, ValueError):
        return None


def _covers(parent_item: str, child_item: str):
    parent_norm = _norm(parent_item)
    child_norm = _norm(child_item)
    return bool(parent_norm) and (parent_norm == child_norm or parent_norm in child_norm)


def _norm(value: str):
    return re.sub(r"\W+", " ", value.lower()).strip()


def _child_suffix(rest: str):
    return (len(rest) == 1 and rest.islower()) or (len(rest) > 1 and rest[0].islower() and rest[1] == "-")


def _finding(severity: str, artifact: str, issue: str):
    return {"severity": severity, "artifact": artifact, "issue": issue}


def _result(applies: bool, status: str, findings=None):
    findings = list(findings or [])
    return {"applies": applies, "status": status, "parent_task_id": None, "parent_spec_path": None, "children": {}, "coverage": {"invariants": [], "acceptance": []}, "gaps": [], "inconsistencies": findings, "findings": findings}
