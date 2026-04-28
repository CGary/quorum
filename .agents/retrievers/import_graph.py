"""
Expand context by following import/require statements from seed files.
Returns a list of file paths reachable within max_hops.
"""
import re
import sys
from pathlib import Path


IMPORT_PATTERNS = [
    # JS/TS: import ... from '...'
    re.compile(r"""(?:import|export)\s+.*?from\s+['"]([^'"]+)['"]"""),
    # JS/TS: require('...')
    re.compile(r"""require\s*\(\s*['"]([^'"]+)['"]\s*\)"""),
    # Python: from . import / import .
    re.compile(r"""^(?:from\s+([\w.]+)\s+import|import\s+([\w.]+))""", re.MULTILINE),
    # Go: import "..."
    re.compile(r'''"([\w./\-]+)"'''),
]

SKIP_DIRS = {"node_modules", "vendor", "dist", "build", ".git", "__pycache__"}

EXTENSIONS = {".ts", ".tsx", ".js", ".jsx", ".py", ".go"}


def resolve_import(importer: Path, raw: str, root: Path) -> Path | None:
    if raw.startswith("."):
        candidate = (importer.parent / raw).resolve()
        for ext in EXTENSIONS:
            if (candidate.with_suffix(ext)).exists():
                return candidate.with_suffix(ext)
            index = candidate / f"index{ext}"
            if index.exists():
                return index
    return None


def expand(seed_files: list[str], root: str, max_hops: int) -> list[str]:
    root_path = Path(root).resolve()
    visited: set[Path] = set()
    frontier: set[Path] = set()

    for f in seed_files:
        p = Path(f).resolve()
        if p.exists():
            frontier.add(p)

    for _ in range(max_hops):
        next_frontier: set[Path] = set()
        for path in frontier:
            if path in visited:
                continue
            if any(part in SKIP_DIRS for part in path.parts):
                continue
            visited.add(path)
            if path.suffix not in EXTENSIONS:
                continue
            try:
                content = path.read_text(errors="ignore")
            except OSError:
                continue
            for pattern in IMPORT_PATTERNS:
                for match in pattern.finditer(content):
                    raw = next((g for g in match.groups() if g), None)
                    if not raw:
                        continue
                    resolved = resolve_import(path, raw, root_path)
                    if resolved and resolved not in visited:
                        next_frontier.add(resolved)
        frontier = next_frontier

    return [str(p) for p in sorted(visited)]


if __name__ == "__main__":
    import json
    seeds = sys.argv[1:-2]
    root = sys.argv[-2]
    hops = int(sys.argv[-1])
    result = expand(seeds, root, hops)
    print(json.dumps(result, indent=2))
