import json
import subprocess
import time
import datetime
from pathlib import Path

task_id = "F-03-b"
worktree = Path("worktrees") / task_id

commands = [
    "go build -o tmp-quorum .",
    "./tmp-quorum --help",
    "./tmp-quorum task --help",
    "rm tmp-quorum"
]

results = []
overall = "passed"

for cmd in commands:
    start_time = time.time()
    try:
        proc = subprocess.run(cmd, shell=True, cwd=str(worktree), capture_output=True, text=True)
        duration = time.time() - start_time
        exit_code = proc.returncode
        output = (proc.stdout + proc.stderr)[:2000]
        if exit_code != 0:
            overall = "failed"
    except Exception as e:
        duration = time.time() - start_time
        exit_code = -1
        output = str(e)[:2000]
        overall = "failed"
        
    results.append({
        "command": cmd,
        "exit_code": exit_code,
        "duration_s": round(duration, 3),
        "output_excerpt": output
    })

validation = {
    "task_id": task_id,
    "summary": "Fast verification passed for contract commands." if overall == "passed" else "Fast verification failed.",
    "executed_at": datetime.datetime.now(datetime.timezone.utc).isoformat()[:-6] + "Z",
    "commands": results,
    "overall_result": overall
}

out_path = Path(".ai/tasks/active") / task_id / "05-validation.json"
out_path.write_text(json.dumps(validation, indent=2))
print(f"Validation written to {out_path} with overall_result: {overall}")
