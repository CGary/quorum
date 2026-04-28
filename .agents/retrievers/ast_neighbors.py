"""
Find files that reference symbols defined in seed files.
Uses ripgrep for speed. Returns a list of file paths.
"""
import json
import re
import subprocess
import sys
from pathlib import Path


SKIP_DIRS = ["node_modules", "vendor", "dist", "build", ".git", "__pycache__"]

SYMBOL_PATTERNS = {
    ".ts":  re.compile(r"""export\s+(?:default\s+)?(?:function|class|const|type|interface|enum)\s+(\w+)"""),
    ".tsx": re.compile(r"""export\s+(?:default\s+)?(?:function|class|const|type|interface|enum)\s+(\w+)"""),
    ".js":  re.compile(r"""export\s+(?:default\s+)?(?:function|class|const)\s+(\w+)"""),
    ".py":  re.compile(r"""^(?:def|class)\s+(\w+)""", re.MULTILINE),
    ".go":  re.compile(r"""^func\s+(?:\(\w+\s+\*?\w+\)\s+)?(\w+)""", re.MULTILINE),
}


def extract_symbols(path: Path) -> list[str]:
    pattern = SYMBOL_PATTERNS.get(path.suffix)
    if not pattern:
        return []
    try:
        content = path.read_text(errors="ignore")
    except OSError:
        return []
    return pattern.findall(content)


def find_references(symbols: list[str], root: str, exclude_files: list[str]) -> list[str]:
    if not symbols:
        return []

    rg_cmd = [
        "rg", "--files-with-matches", "--type-add", "code:*.{ts,tsx,js,jsx,py,go}",
        "--type", "code",
    ]
    for skip in SKIP_DIRS:
        rg_cmd += ["--glob", f"!{skip}"]
    rg_cmd += ["|".join(re.escape(s) for s in symbols), root]

    try:
        result = subprocess.run(rg_cmd, capture_output=True, text=True, timeout=30)
        files = result.stdout.strip().splitlines()
    except (subprocess.TimeoutExpired, FileNotFoundError):
        return []

    exclude_set = {Path(f).resolve() for f in exclude_files}
    return [f for f in files if Path(f).resolve() not in exclude_set]


def neighbors(seed_files: list[str], root: str) -> list[str]:
    all_refs: set[str] = set()
    for seed in seed_files:
        path = Path(seed)
        if not path.exists():
            continue
        symbols = extract_symbols(path)
        refs = find_references(symbols, root, seed_files)
        all_refs.update(refs)
    return sorted(all_refs)


if __name__ == "__main__":
    seeds = sys.argv[1:-1]
    root = sys.argv[-1]
    result = neighbors(seeds, root)
    print(json.dumps(result, indent=2))
