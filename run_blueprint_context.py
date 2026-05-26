import sys
from pathlib import Path
sys.path.insert(0, ".agents")
from cli.core.blueprint_context import enrich_blueprint_with_retrievers
import yaml

blueprint_dict = {
    "task_id": "F-03-d",
    "summary": "Port non-worktree task lifecycle commands.",
    "affected_files": [
        "internal/core/task_manager.go",
        "internal/core/task_manager_test.go",
        "cmd/task.go",
        "cmd/project.go"
    ],
    "symbols": [
        "InitializeSpecify",
        "PrepareBlueprint",
        "SplitTask",
        "ListTasks",
        "ShowStatus",
        "SaveArtifact",
        "ConsumeFeedback"
    ],
    "dependencies": [
        "internal/core/schema.go"
    ],
    "test_scenarios": [],
    "strategy": []
}

enriched = enrich_blueprint_with_retrievers(blueprint_dict, Path("."))
print(yaml.dump(enriched))
