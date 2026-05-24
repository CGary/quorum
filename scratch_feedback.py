import json
import sys
from pathlib import Path

sys.path.insert(0, ".agents")
from cli.core.task_manager import partition_feedback_findings

task_id = "F-03-a"
feedback_path = Path(f".ai/tasks/active/{task_id}/feedback.json")
if feedback_path.exists():
    payload = json.loads(feedback_path.read_text())
    partitioned = partition_feedback_findings(payload)
    print(json.dumps(partitioned, indent=2))
else:
    print("No feedback.json found")
