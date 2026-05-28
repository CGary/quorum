import sys
import yaml
import json
import os

sys.path.insert(0, ".agents")
from cli.core.risk_scorer import assign_risk_level, build_risk_trace_events
from cli.core.task_manager import append_trace_attempt

task_id = "F-03-f"

with open(".agents/policies/risk.yaml") as f:
    policy = yaml.safe_load(f)
with open(f".ai/tasks/active/{task_id}/01-blueprint.yaml") as f:
    blueprint = yaml.safe_load(f)
with open(f".ai/tasks/active/{task_id}/00-spec.yaml") as f:
    spec = yaml.safe_load(f)

result = assign_risk_level(blueprint, policy)
events = build_risk_trace_events(spec.get("risk"), result)

trace_path = f".ai/tasks/active/{task_id}/07-trace.json"
if not os.path.exists(trace_path):
    with open(trace_path, "w") as f:
        json.dump({"task_id": task_id, "attempts": []}, f, indent=2)

for e in events:
    append_trace_attempt(trace_path, e)

print("Appended events to trace:", events)
