import sys
import shutil
import yaml
import json
import subprocess
import os
from pathlib import Path
from jsonschema import validate
import datetime

# Quorum v1.1: PROJECT_ROOT is now dynamic based on where the user is running the tool.
# We look for the git root or fallback to CWD.
def get_project_root():
    try:
        res = subprocess.run(
            ["git", "rev-parse", "--show-toplevel"],
            check=True, capture_output=True, text=True
        )
        return Path(res.stdout.strip())
    except Exception:
        return Path(os.getcwd())

PROJECT_ROOT = get_project_root()
AI_TASKS = PROJECT_ROOT / ".ai" / "tasks"
AGENTS_DIR = PROJECT_ROOT / ".agents" # Note: Local project settings if they exist
SCHEMAS_DIR = Path(__file__).parent.parent.parent / "schemas" # Schemas are usually in the tool source

def get_base_branch():
    """Detects the base branch of the repository."""
    # Try origin HEAD first
    try:
        res = subprocess.run(
            ["git", "symbolic-ref", "refs/remotes/origin/HEAD"],
            check=True, capture_output=True, text=True
        )
        return res.stdout.strip().split("/")[-1]
    except subprocess.CalledProcessError:
        pass
    
    # Try common defaults
    for branch in ["main", "master", "develop", "trunk"]:
        try:
            subprocess.run(
                ["git", "rev-parse", "--verify", branch],
                check=True, capture_output=True
            )
            return branch
        except subprocess.CalledProcessError:
            continue
            
    # Fallback to current branch
    res = subprocess.run(
        ["git", "rev-parse", "--abbrev-ref", "HEAD"],
        check=True, capture_output=True, text=True
    )
    return res.stdout.strip()

def find_task_dir(task_id, locations=["inbox", "active", "done", "failed"]):
    matches = []
    for loc in locations:
        loc_path = AI_TASKS / loc
        if not loc_path.exists():
            continue
        for d in loc_path.iterdir():
            if d.is_dir() and (d.name == task_id or d.name.startswith(f"{task_id}-")):
                matches.append((d, loc))
    
    if len(matches) > 1:
        print(f"[!] AMBIGUITY ERROR: Multiple tasks match '{task_id}':")
        for m, l in matches:
            print(f"  - {l}/{m.name}")
        sys.exit(1)
        
    return matches[0] if matches else (None, None)

def initialize_specify(task_id=None):
    if not task_id:
        # Generate a simple timestamp ID for now
        task_id = f"TASK-{datetime.datetime.now().strftime('%m%d%H%M')}"
    
    task_dir = AI_TASKS / "inbox" / f"{task_id}-new-spec"
    task_dir.mkdir(parents=True, exist_ok=True)
    
    # Create YAML spec placeholder. `summary` is the second key by Quorum v1.1 convention.
    spec_path = task_dir / "00-spec.yaml"
    if not spec_path.exists():
        spec = {
            "task_id": task_id,
            "summary": "Draft spec; fill goal, invariants, and acceptance before blueprint.",
            "goal": "TODO: define the feature goal.",
            "invariants": ["TODO: define invariant."],
            "acceptance": ["TODO: define acceptance criterion."],
            "risk": "medium",
        }
        with open(spec_path, "w") as f:
            yaml.safe_dump(spec, f, sort_keys=False)
            
    return task_dir

def prepare_blueprint(task_id):
    # Blueprint happens in 'active' because it requires exploring code, 
    # but it doesn't need a worktree yet.
    task_dir, loc = find_task_dir(task_id, ["inbox"])
    if not task_dir:
        active_dir, _ = find_task_dir(task_id, ["active"])
        if active_dir:
            print(f"[*] Task {task_id} is already in active.")
            return active_dir
        raise ValueError(f"Task {task_id} not found in inbox.")

    # Move to active
    active_path = AI_TASKS / "active" / task_dir.name
    active_path.parent.mkdir(parents=True, exist_ok=True)
    shutil.move(str(task_dir), str(active_path))
    return active_path

def start_task(task_id):
    # 1. Find task in any location
    task_dir, loc = find_task_dir(task_id, ["active", "inbox"])
    if not task_dir:
        print(f"[!] Task {task_id} not found.")
        return

    # 2. Validate contract (02-contract.yaml) BEFORE any movement
    contract_path = task_dir / "02-contract.yaml"
    if not contract_path.exists():
        print(f"[!] Contract (02-contract.yaml) not found for {task_id}.")
        print(f"[!] Please run 'agents task blueprint {task_id}' first.")
        return

    with open(contract_path, "r") as f:
        contract = yaml.safe_load(f)

    schema_path = SCHEMAS_DIR / "contract.schema.json"
    with open(schema_path, "r") as f:
        schema = json.load(f)

    try:
        validate(instance=contract, schema=schema)
    except Exception as e:
        print(f"[!] Contract validation failed for {task_id}: {e}")
        return

    # 3. Transition state (move to active if it was in inbox)
    if loc == "inbox":
        active_path = AI_TASKS / "active" / task_dir.name
        active_path.parent.mkdir(parents=True, exist_ok=True)
        print(f"[*] Moving task from inbox to active...")
        shutil.move(str(task_dir), str(active_path))
        task_dir = active_path

    # 4. Create worktree
    worktree_path = PROJECT_ROOT / "worktrees" / task_id
    branch_name = f"ai/{task_id}"
    base_branch = get_base_branch()
    
    if worktree_path.exists():
        print(f"[*] Worktree for {task_id} already exists.")
    else:
        print(f"[*] Creating worktree in {worktree_path} (base: {base_branch})...")
        try:
            subprocess.run([
                "git", "worktree", "add", str(worktree_path), "-b", branch_name, base_branch
            ], check=True, capture_output=True)
        except subprocess.CalledProcessError as e:
            print(f"[!] Error creating worktree: {e.stderr.decode()}")
            return

    # 4. Initialize trace.json (07-trace.json)
    trace_path = task_dir / "07-trace.json"
    if not trace_path.exists():
        trace = {
            "task_id": task_id,
            "summary": contract.get("summary", "Trace initialized for task."),
            "started_at": datetime.datetime.now(datetime.UTC).isoformat().replace("+00:00", "Z"),
            "execution_mode": contract.get("execution", {}).get("mode", "patch_only"),
            "attempts": [],
            "total_cost_usd": 0.0,
            "violations": [],
            "context_overflows": []
        }
        with open(trace_path, "w") as f:
            json.dump(trace, f, indent=2)

    print(f"[+] Task {task_id} initialized and worktree ready.")

def run_task(task_id):
    # This remains an MVP stub, now consuming 01-blueprint.yaml + 02-contract.yaml.
    task_dir, loc = find_task_dir(task_id, ["active"])
    if not task_dir:
        print(f"[!] Task {task_id} not found in active.")
        return
    print(f"[*] Running task {task_id} based on 01-blueprint.yaml and 02-contract.yaml...")
    # MVP Execution logic would go here

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
        print(f"Summary: {trace.get('summary')}")
        print(f"Cost: ${trace.get('total_cost_usd', 0):.3f}")
    
    # Check for AI-First artifacts
    for art in ["00-spec.yaml", "01-blueprint.yaml", "02-contract.yaml", "05-validation.json", "06-review.json", "07-trace.json"]:
        status = "Present" if (task_dir / art).exists() else "Missing"
        print(f"- {art}: {status}")

def list_tasks():
    for loc in ["inbox", "active", "done", "failed"]:
        loc_path = AI_TASKS / loc
        if not loc_path.exists():
            continue
        for task_dir in sorted(d for d in loc_path.iterdir() if d.is_dir()):
            task_id = task_dir.name.split("-", 2)
            task_id = "-".join(task_id[:2]) if len(task_id) >= 2 else task_dir.name
            summary = ""
            for artifact in ["00-spec.yaml", "01-blueprint.yaml", "02-contract.yaml", "07-trace.json"]:
                path = task_dir / artifact
                if not path.exists():
                    continue
                try:
                    if path.suffix == ".json":
                        data = json.loads(path.read_text())
                    else:
                        data = yaml.safe_load(path.read_text())
                    summary = data.get("summary", "") if isinstance(data, dict) else ""
                except Exception:
                    summary = "<unreadable summary>"
                if summary:
                    break
            print(f"{loc:6} {task_id:12} {summary}")

def clean_task(task_id):
    task_dir, loc = find_task_dir(task_id, ["active", "done", "failed"])
    if not task_dir:
        print(f"[!] Task {task_id} not found.")
        return

    worktree_path = PROJECT_ROOT / "worktrees" / task_id
    if worktree_path.exists():
        print(f"[*] Removing worktree {worktree_path}...")
        subprocess.run(["git", "worktree", "remove", str(worktree_path)], check=False)

    if loc == "active":
        target_dir = AI_TASKS / "done" / task_dir.name
        print(f"[*] Archiving task to done/...")
        shutil.move(str(task_dir), str(target_dir))

    print(f"[+] Task {task_id} cleaned up.")
