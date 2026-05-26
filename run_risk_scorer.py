import sys
import yaml

sys.path.insert(0, ".agents")

from cli.core.risk_scorer import assign_risk_level, build_risk_trace_events
from cli.core.task_manager import append_trace_attempt

with open(".agents/policies/risk.yaml") as f:
    policy = yaml.safe_load(f)
with open(".ai/tasks/active/F-03-d/01-blueprint.yaml") as f:
    blueprint = yaml.safe_load(f)
with open(".ai/tasks/active/F-03-d/00-spec.yaml") as f:
    spec = yaml.safe_load(f)

result = assign_risk_level(blueprint, policy)
events = build_risk_trace_events(spec.get("risk"), result)

trace_path = ".ai/tasks/active/F-03-d/07-trace.json"
for e in events:
    append_trace_attempt(trace_path, e)

print(yaml.dump(events))
