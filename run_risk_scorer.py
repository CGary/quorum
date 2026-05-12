import sys
import yaml
import json
from pathlib import Path

sys.path.insert(0, ".agents")
from cli.core.risk_scorer import assign_risk_level, build_risk_trace_events

task_id = "FEAT-005-j"
task_dir = Path(f".ai/tasks/active/{task_id}-new-spec")

with open(".agents/policies/risk.yaml") as f:
    policy = yaml.safe_load(f)

with open(task_dir / "01-blueprint.yaml") as f:
    blueprint = yaml.safe_load(f)

with open(task_dir / "00-spec.yaml") as f:
    spec = yaml.safe_load(f)

result = assign_risk_level(blueprint, policy)
events = build_risk_trace_events(spec.get("risk"), result)

trace_file = task_dir / "07-trace.json"
if trace_file.exists():
    with open(trace_file) as f:
        trace = json.load(f)
else:
    trace = {
        "task_id": task_id,
        "attempts": []
    }

if not trace["attempts"]:
    trace["attempts"].append({"events": []})

trace["attempts"][0]["events"].extend(events)

with open(trace_file, "w") as f:
    json.dump(trace, f, indent=2)

print("Trace updated with events:")
print(json.dumps(events, indent=2))
