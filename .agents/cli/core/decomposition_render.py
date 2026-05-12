"""Pure ASCII rendering helpers for task decomposition DAGs."""


def render_ascii_dag(decomposition):
    """Render a deterministic ASCII map for a decomposition dependency DAG.

    The renderer is presentation-only: it does not validate, mutate, or persist
    task state. Unknown dependency IDs are ignored for level calculation but are
    still shown in the edge list so callers can see exactly what was provided.
    """
    if not decomposition:
        return ""

    entries = [entry for entry in decomposition if isinstance(entry, dict)]
    child_ids = sorted(
        entry.get("child_id")
        for entry in entries
        if isinstance(entry.get("child_id"), str) and entry.get("child_id")
    )
    if not child_ids:
        return ""

    known = set(child_ids)
    deps_by_child = {
        child_id: sorted(
            dep
            for dep in (entry.get("depends_on") or [])
            if isinstance(dep, str)
        )
        for entry in entries
        for child_id in [entry.get("child_id")]
        if child_id in known
    }
    level_cache = {}
    visiting = set()

    def level_for(child_id):
        if child_id in level_cache:
            return level_cache[child_id]
        if child_id in visiting:
            # split_task rejects cycles before calling this renderer. Keep the
            # pure renderer defensive so malformed direct calls do not recurse.
            return 0
        visiting.add(child_id)
        known_deps = [dep for dep in deps_by_child.get(child_id, []) if dep in known]
        if known_deps:
            level = max(level_for(dep) + 1 for dep in known_deps)
        else:
            level = 0
        visiting.remove(child_id)
        level_cache[child_id] = level
        return level

    levels = {}
    for child_id in child_ids:
        levels.setdefault(level_for(child_id), []).append(child_id)

    ordered_levels = sorted(levels)
    child_columns = [
        [f"[{child_id}]" for child_id in sorted(levels[level])]
        for level in ordered_levels
    ]
    headers = [f"L{level}" for level in ordered_levels]
    widths = [
        max(len(header), *(len(child) for child in children))
        for header, children in zip(headers, child_columns)
    ]
    row_count = max(len(children) for children in child_columns)

    lines = ["Decomposition DAG:"]
    lines.append("  order: " + " -> ".join(headers))
    lines.append(
        ("  " + "  ".join(
            header.ljust(width)
            for header, width in zip(headers, widths)
        )).rstrip()
    )
    for row in range(row_count):
        cells = [
            (children[row] if row < len(children) else "").ljust(width)
            for children, width in zip(child_columns, widths)
        ]
        lines.append("  " + "  ".join(cells).rstrip())

    edges = []
    for child_id in child_ids:
        for dep in deps_by_child.get(child_id, []):
            edges.append((dep, child_id))

    lines.append("  edges:")
    if edges:
        for dep, child_id in sorted(edges):
            lines.append(f"    {dep} -> {child_id}")
    else:
        lines.append("    (none)")

    return "\n".join(lines)
