import json
import os
import sys
from pathlib import Path

import yaml
from jsonschema import validate

PROJECT_ROOT = Path(__file__).resolve().parents[1]
AGENTS_DIR = PROJECT_ROOT / ".agents"
if str(AGENTS_DIR) not in sys.path:
    sys.path.insert(0, str(AGENTS_DIR))

from cli.core import blueprint_context


def _valid_blueprint():
    return {
        "task_id": "TEST-001",
        "summary": "test blueprint context",
        "affected_files": ["src/seed.py"],
        "symbols": ["seed_symbol"],
        "dependencies": [],
        "test_scenarios": ["retriever output affects yaml"],
    }


def test_retriever_output_changes_serialized_blueprint_yaml(tmp_path, monkeypatch):
    (tmp_path / "src").mkdir()
    (tmp_path / "src/seed.py").write_text("def seed_symbol():\n    return 1\n")

    monkeypatch.setattr(
        blueprint_context.ast_neighbors,
        "neighbors",
        lambda seed_files, root: [str(tmp_path / "src/neighbor.py")],
    )
    monkeypatch.setattr(
        blueprint_context.import_graph,
        "expand",
        lambda seed_files, root, max_hops: [str(tmp_path / "src/imported.py")],
    )

    before = yaml.safe_dump(_valid_blueprint(), sort_keys=False)
    enriched = blueprint_context.enrich_blueprint_with_retrievers(_valid_blueprint(), tmp_path)
    after = yaml.safe_dump(enriched, sort_keys=False)

    assert "src/neighbor.py" not in before
    assert "src/imported.py" not in before
    assert "src/neighbor.py" in after
    assert "src/imported.py" in after
    assert enriched["affected_files"] == ["src/seed.py", "src/neighbor.py"]
    assert enriched["dependencies"] == ["src/imported.py"]


def test_enrichment_deduplicates_relative_paths_and_preserves_schema(tmp_path, monkeypatch):
    (tmp_path / "src").mkdir()
    (tmp_path / "src/seed.py").write_text("def seed_symbol():\n    return 1\n")

    monkeypatch.setattr(
        blueprint_context.ast_neighbors,
        "neighbors",
        lambda seed_files, root: [
            str(tmp_path / "src/neighbor.py"),
            "src/neighbor.py",
            str(tmp_path / "src/another.py"),
            str(tmp_path.parent / "outside.py"),
        ],
    )
    monkeypatch.setattr(
        blueprint_context.import_graph,
        "expand",
        lambda seed_files, root, max_hops: [
            "src/imported.py",
            str(tmp_path / "src/imported.py"),
            str(tmp_path / "src/seed.py"),
        ],
    )

    blueprint = _valid_blueprint()
    blueprint["affected_files"] = ["src/seed.py", "src/seed.py"]
    enriched = blueprint_context.enrich_blueprint_with_retrievers(blueprint, tmp_path)

    assert enriched["affected_files"] == [
        "src/seed.py",
        "src/another.py",
        "src/neighbor.py",
    ]
    assert enriched["dependencies"] == ["src/imported.py", "src/seed.py"]

    schema_path = PROJECT_ROOT / ".agents/schemas/blueprint.schema.json"
    validate(instance=enriched, schema=json.loads(schema_path.read_text()))


def test_q_blueprint_skill_documents_the_wired_helper():
    skill_text = (PROJECT_ROOT / ".agents/skills/q-blueprint/SKILL.md").read_text()

    assert "blueprint_context" in skill_text
    assert "enrich_blueprint_with_retrievers" in skill_text
    assert "retrievers.ast_neighbors" in skill_text
    assert "retrievers.import_graph" in skill_text
