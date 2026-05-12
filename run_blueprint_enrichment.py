import sys
import yaml
from pathlib import Path

sys.path.insert(0, ".agents")
from cli.core.failure_lookup import find_related_failed_tasks
from cli.core.blueprint_context import enrich_blueprint_with_retrievers

task_id = "FEAT-005-j"
draft_blueprint = {
    "task_id": task_id,
    "summary": "Reescribir README.md como Quick Start completo del flujo padre/hijas.",
    "affected_files": ["README.md"],
    "symbols": [],
    "dependencies": [],
    "test_scenarios": [
        "Verificar que README.md contiene el ciclo de vida completo incluyendo padre/hijas."
    ],
    "strategy": [
        {
            "step": 1,
            "action": "Actualizar README.md con el flujo Quick Start completo.",
            "files": ["README.md"]
        }
    ]
}

related = find_related_failed_tasks(draft_blueprint, Path(".ai/tasks"))
if related:
    draft_blueprint["risks"] = related

enriched = enrich_blueprint_with_retrievers(draft_blueprint, Path("."))

print(yaml.dump(enriched, sort_keys=False))
