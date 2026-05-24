import sys
import json
from pathlib import Path

sys.path.insert(0, ".agents")

from cli.core.blueprint_context import enrich_blueprint_with_retrievers

blueprint_dict = {
    "affected_files": ["tests/capture_golden.py", "tests/test_golden_master.py"],
    "dependencies": []
}

try:
    enriched = enrich_blueprint_with_retrievers(blueprint_dict, Path("."))
    print(json.dumps(enriched, indent=2))
except Exception as e:
    print(f"Error: {e}")
