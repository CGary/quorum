import sys
import yaml
import json
from pathlib import Path

sys.path.insert(0, ".agents")

from cli.core.risk_scorer import assign_risk_level, build_risk_trace_events

task_id = "F-03-a"

try:
    with open(".agents/policies/risk.yaml") as f:
        policy = yaml.safe_load(f)
    with open(f".ai/tasks/active/{task_id}/01-blueprint.yaml") as f:
        blueprint = yaml.safe_load(f)
    with open(f".ai/tasks/active/{task_id}/00-spec.yaml") as f:
        spec = yaml.safe_load(f)

    result = assign_risk_level(blueprint, policy)
    events = build_risk_trace_events(spec.get("risk"), result)
    print(json.dumps(events, indent=2))
except Exception as e:
    print(f"Error: {e}")
