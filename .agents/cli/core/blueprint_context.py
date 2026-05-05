"""
Context enrichment helpers for q-blueprint.

This module wires the repository retrievers into the human-operated blueprint
phase without introducing an automatic dispatcher. Callers pass a draft
blueprint payload and receive a copy whose file lists include retriever output.
"""
from __future__ import annotations

from pathlib import Path
from typing import Iterable, Mapping, MutableMapping, Any

import retrievers.ast_neighbors as ast_neighbors
import retrievers.import_graph as import_graph


BLUEPRINT_FILE_KEYS = ("affected_files", "dependencies")


def enrich_blueprint_with_retrievers(
    blueprint: Mapping[str, Any],
    project_root: str | Path = ".",
    *,
    max_hops: int = 2,
) -> dict[str, Any]:
    """
    Return a blueprint copy enriched with ast/import retriever context.

    `ast_neighbors` adds files that reference symbols from the current
    affected files; these are likely additional implementation touch points, so
    they are appended to `affected_files`. `import_graph` adds files reachable
    through local imports; these are context dependencies, so they are appended
    to `dependencies`.
    """
    root = Path(project_root).resolve()
    enriched: dict[str, Any] = dict(blueprint)

    affected = _normalized_unique(enriched.get("affected_files") or [], root)
    dependencies = _normalized_unique(enriched.get("dependencies") or [], root)
    seed_files = _absolute_seed_files(affected, root)

    neighbor_files = ast_neighbors.neighbors([str(p) for p in seed_files], str(root))
    import_files = import_graph.expand([str(p) for p in seed_files], str(root), max_hops)

    enriched["affected_files"] = _merge_paths(affected, neighbor_files, root)
    enriched["dependencies"] = _merge_paths(dependencies, import_files, root)
    return enriched


def _absolute_seed_files(paths: Iterable[str], root: Path) -> list[Path]:
    seeds: list[Path] = []
    for rel in paths:
        candidate = (root / rel).resolve()
        if candidate.exists():
            seeds.append(candidate)
    return seeds


def _merge_paths(existing: Iterable[str], discovered: Iterable[str], root: Path) -> list[str]:
    merged = _normalized_unique(existing, root)
    seen = set(merged)
    for path in sorted(_normalized_unique(discovered, root)):
        if path not in seen:
            merged.append(path)
            seen.add(path)
    return merged


def _normalized_unique(paths: Iterable[str], root: Path) -> list[str]:
    normalized: list[str] = []
    seen: set[str] = set()
    for raw in paths:
        rel = _project_relative(raw, root)
        if rel is None or rel in seen:
            continue
        normalized.append(rel)
        seen.add(rel)
    return normalized


def _project_relative(raw: str, root: Path) -> str | None:
    if not raw:
        return None

    path = Path(raw)
    absolute = path.resolve() if path.is_absolute() else (root / path).resolve()
    try:
        relative = absolute.relative_to(root)
    except ValueError:
        return None

    return relative.as_posix()
