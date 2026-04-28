import os
import shutil
import yaml
import json
import subprocess
from pathlib import Path
from jsonschema import validate

PROJECT_ROOT = Path(__file__).parent.parent.parent.parent
AI_TASKS = PROJECT_ROOT / ".ai" / "tasks"
AGENTS_DIR = PROJECT_ROOT / ".agents"
SCHEMAS_DIR = AGENTS_DIR / "schemas"

def find_task_dir(task_id, locations=["inbox", "active", "done", "failed"]):
    for loc in locations:
        loc_path = AI_TASKS / loc
        if not loc_path.exists():
            continue
        for d in loc_path.iterdir():
            if d.is_dir() and d.name.startswith(task_id):
                return d, loc
    return None, None

def start_task(task_id):
    # 1. Find task in inbox
    task_dir, loc = find_task_dir(task_id, ["inbox"])
    if not task_dir:
        # Check if already active
        active_dir, _ = find_task_dir(task_id, ["active"])
        if active_dir:
            print(f"[!] Task {task_id} is already active.")
            return
        print(f"[!] Task {task_id} not found in inbox.")
        return

    # 2. Validate contract
    contract_path = task_dir / "01-contract.yaml"
    if not contract_path.exists():
        print(f"[!] Contract not found for {task_id}")
        return

    with open(contract_path, "r") as f:
        contract = yaml.safe_load(f)

    schema_path = SCHEMAS_DIR / "contract.schema.json"
    with open(schema_path, "r") as f:
        schema = json.load(f)

    try:
        validate(instance=contract, schema=schema)
    except Exception as e:
        print(f"[!] Validation failed: {e}")
        # Move to failed?
        return

    # 3. Move to active
    active_path = AI_TASKS / "active" / task_dir.name
    shutil.move(str(task_dir), str(active_path))
    task_dir = active_path

    # 4. Create worktree
    worktree_path = PROJECT_ROOT / "worktrees" / task_id
    branch_name = f"ai/{task_id}"
    
    print(f"[*] Creating worktree in {worktree_path}...")
    try:
        subprocess.run([
            "git", "worktree", "add", str(worktree_path), "-b", branch_name, "origin/main"
        ], check=True, capture_output=True)
    except subprocess.CalledProcessError as e:
        print(f"[!] Error creating worktree: {e.stderr.decode()}")
        # Rollback move if needed, but for MVP let's just stop
        return

    # 5. Initialize trace.json
    trace = {
        "task_id": task_id,
        "profile": contract.get("profile", "standard"),
        "risk": contract.get("risk", "medium"),
        "complexity": contract.get("complexity", "atomic"),
        "execution_mode": contract.get("execution", {}).get("mode", "patch_only"),
        "started_at": "TODO_ISO_TIMESTAMP",
        "attempts": [],
        "total_cost_usd": 0.0,
        "violations": [],
        "context_overflows": []
    }
    
    import datetime
    trace["started_at"] = datetime.datetime.utcnow().isoformat() + "Z"

    with open(task_dir / "07-trace.json", "w") as f:
        json.dump(trace, f, indent=2)

    print(f"[+] Task {task_id} started successfully.")

def run_task(task_id):
    task_dir, loc = find_task_dir(task_id, ["active"])
    if not task_dir:
        print(f"[!] Task {task_id} not found in active.")
        return
    print(f"[!] 'run' not fully implemented yet in MVP Week 1. This would execute the phases.")

def show_status(task_id):
    task_dir, loc = find_task_dir(task_id)
    if not task_dir:
        print(f"[!] Task {task_id} not found.")
        return
    
    print(f"Task: {task_id}")
    print(f"Location: {loc}")
    
    trace_path = task_dir / "07-trace.json"
    if trace_path.exists():
        with open(trace_path, "r") as f:
            trace = json.load(f)
        print(f"Profile: {trace.get('profile')}")
        print(f"Cost: ${trace.get('total_cost_usd'):.3f}")
        print(f"Attempts: {len(trace.get('attempts', []))}")
    else:
        print("Trace not initialized.")

def clean_task(task_id):
    task_dir, loc = find_task_dir(task_id, ["active", "done", "failed"])
    if not task_dir:
        print(f"[!] Task {task_id} not found.")
        return

    # Remove worktree
    worktree_path = PROJECT_ROOT / "worktrees" / task_id
    if worktree_path.exists():
        print(f"[*] Removing worktree {worktree_path}...")
        subprocess.run(["git", "worktree", "remove", str(worktree_path)], check=False)
        # Also delete branch?
        # subprocess.run(["git", "branch", "-D", f"ai/{task_id}"], check=False)

    # Move to done or failed based on outcome if not already there
    if loc == "active":
        # Check outcome in trace
        trace_path = task_dir / "07-trace.json"
        outcome = "done"
        if trace_path.exists():
            with open(trace_path, "r") as f:
                trace = json.load(f)
            if trace.get("outcome") == "failed":
                outcome = "failed"
        
        target_dir = AI_TASKS / outcome / task_dir.name
        print(f"[*] Moving task to {outcome}/...")
        shutil.move(str(task_dir), str(target_dir))

    print(f"[+] Task {task_id} cleaned up.")
