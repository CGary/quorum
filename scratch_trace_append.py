import json
from pathlib import Path

trace_path = Path(".ai/tasks/active/F-03-a/07-trace.json")
if trace_path.exists():
    trace = json.loads(trace_path.read_text())
    trace["attempts"].append({
        "phase": "blueprint",
        "result": "passed",
        "duration_s": 5.0,
        "notes": "Risk calculation events: [{\"event\": \"risk_level_calculated\", \"level\": \"medium\", \"reasons\": [\"symbols_count_high: 3\"], \"signals\": {\"files_count\": 2, \"symbols_count\": 3, \"sensitive_matches\": []}}]"
    })
    trace_path.write_text(json.dumps(trace, indent=2))
    print("Appended attempt to trace")
else:
    print("Trace not found")
