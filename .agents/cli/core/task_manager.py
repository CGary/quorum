import sys
import shutil
import yaml
import json
import subprocess
import os
import re
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
POLICIES_DIR = Path(__file__).parent.parent.parent / "policies"
TEMPLATES_DIR = Path(__file__).parent.parent.parent / "templates"

CHILD_ID_RE = re.compile(r"^([A-Z]+-[0-9]+)-([a-z])$")
PARENT_ID_RE = re.compile(r"^[A-Z]+-[0-9]+$")

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

def _read_spec_task_id(task_dir):
    """Returns the canonical task_id stored in 00-spec.yaml, or None."""
    spec_path = task_dir / "00-spec.yaml"
    if not spec_path.exists():
        return None
    try:
        with open(spec_path) as f:
            spec = yaml.safe_load(f)
        if isinstance(spec, dict):
            return spec.get("task_id")
    except Exception:
        return None
    return None

def _load_json_schema(name):
    with open(SCHEMAS_DIR / name, "r") as f:
        return json.load(f)

def _validate_spec(spec):
    """Validate a 00-spec.yaml payload against the bundled spec schema."""
    validate(instance=spec, schema=_load_json_schema("spec.schema.json"))

def find_task_dir(task_id, locations=["inbox", "active", "done", "failed"]):
    """Find a task directory.

    Resolution order:
      1. Exact `task_id` match in `00-spec.yaml` (canonical, unambiguous).
      2. Exact directory name match.
      3. Directory name starts with `<task_id>-` (legacy slug suffix), but only
         when the next character is NOT a child suffix marker (single
         lowercase letter followed by `-` or end). This prevents
         `FEAT-001` from matching `FEAT-001-a-foo`.
    """
    yaml_matches = []
    name_exact = []
    name_prefix = []
    for loc in locations:
        loc_path = AI_TASKS / loc
        if not loc_path.exists():
            continue
        for d in loc_path.iterdir():
            if not d.is_dir():
                continue
            yaml_id = _read_spec_task_id(d)
            if yaml_id == task_id:
                yaml_matches.append((d, loc))
                continue
            if d.name == task_id:
                name_exact.append((d, loc))
                continue
            prefix = f"{task_id}-"
            if d.name.startswith(prefix):
                rest = d.name[len(prefix):]
                # Skip child-suffix-shaped names when caller asked for the parent.
                # Child suffix is exactly one lowercase letter followed by `-` or EOL.
                if PARENT_ID_RE.match(task_id):
                    if len(rest) == 1 and rest.isalpha() and rest.islower():
                        continue
                    if len(rest) >= 2 and rest[0].islower() and rest[0].isalpha() and rest[1] == "-":
                        continue
                name_prefix.append((d, loc))

    matches = yaml_matches or name_exact or name_prefix

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
        print(f"[!] Please run 'quorum task blueprint {task_id}' first.")
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

    # Worktree status
    worktree_path = PROJECT_ROOT / "worktrees" / task_id
    print(f"- worktree: {'Present' if worktree_path.exists() else 'Missing'}")

    # Decomposition / parent linkage
    spec_path = task_dir / "00-spec.yaml"
    if spec_path.exists():
        try:
            with open(spec_path) as f:
                spec = yaml.safe_load(f) or {}
            if spec.get("parent_task"):
                print(f"- parent_task: {spec['parent_task']}")
                if spec.get("depends_on"):
                    print(f"- depends_on: {', '.join(spec['depends_on'])}")
            if spec.get("decomposition"):
                children = [c.get("child_id", "?") for c in spec["decomposition"]]
                print(f"- decomposition (children): {', '.join(children)}")
                for child_id in children:
                    _, child_loc = find_task_dir(child_id)
                    print(f"  - {child_id}: {child_loc or 'missing'}")
        except Exception:
            pass

def list_tasks():
    for loc in ["inbox", "active", "done", "failed"]:
        loc_path = AI_TASKS / loc
        if not loc_path.exists():
            continue
        for task_dir in sorted(d for d in loc_path.iterdir() if d.is_dir()):
            yaml_id = _read_spec_task_id(task_dir)
            if yaml_id:
                task_id = yaml_id
            else:
                # Fallback to dir-name parsing for legacy / pre-spec dirs.
                parts = task_dir.name.split("-")
                if len(parts) >= 3 and len(parts[2]) == 1 and parts[2].islower():
                    task_id = "-".join(parts[:3])
                else:
                    task_id = "-".join(parts[:2]) if len(parts) >= 2 else task_dir.name
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
            print(f"{loc:6} {task_id:14} {summary}")

def clean_task(task_id):
    task_dir, loc = find_task_dir(task_id, ["active", "done", "failed"])
    if not task_dir:
        print(f"[!] Task {task_id} not found.")
        return

    spec_path = task_dir / "00-spec.yaml"
    if loc == "active" and spec_path.exists():
        try:
            with open(spec_path) as f:
                spec = yaml.safe_load(f) or {}
            decomposition = spec.get("decomposition") or []
        except Exception:
            decomposition = []
        if decomposition:
            not_done = []
            for entry in decomposition:
                child_id = entry.get("child_id") if isinstance(entry, dict) else None
                if not child_id:
                    continue
                _, child_loc = find_task_dir(child_id)
                if child_loc != "done":
                    not_done.append(f"{child_id} ({child_loc or 'missing'})")
            if not_done:
                print(f"[!] Parent task {task_id} still has unfinished children: {', '.join(not_done)}")
                print("[!] Clean each child after its human merge before cleaning the parent.")
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


def back_task(task_id):
    """Reverse the most recent state transition for a task.

    Resolution order:
      1. If a worktree exists, remove it (reverses `start`).
      2. Else if the task is in done/ or failed/, move it back to active/.
      3. Else if the task is in active/, move it back to inbox/ (reverses `blueprint`).
      4. Else if the task is in inbox/, refuse — there is no earlier state.
    """
    task_dir, loc = find_task_dir(task_id)
    if not task_dir:
        print(f"[!] Task {task_id} not found.")
        return

    worktree_path = PROJECT_ROOT / "worktrees" / task_id
    if worktree_path.exists():
        print(f"[*] Reversing 'start': removing worktree {worktree_path}...")
        result = subprocess.run(
            ["git", "worktree", "remove", "--force", str(worktree_path)],
            capture_output=True, text=True
        )
        if result.returncode != 0:
            print(f"[!] git worktree remove failed: {result.stderr.strip()}")
            return
        # Best-effort branch cleanup if it has no commits beyond base
        branch_name = f"ai/{task_id}"
        base_branch = get_base_branch()
        try:
            ahead = subprocess.run(
                ["git", "rev-list", "--count", f"{base_branch}..{branch_name}"],
                capture_output=True, text=True, check=True
            )
            if ahead.stdout.strip() == "0":
                subprocess.run(
                    ["git", "branch", "-D", branch_name],
                    capture_output=True, check=False
                )
                print(f"[+] Removed empty branch {branch_name}.")
            else:
                print(f"[!] Branch {branch_name} has commits; not deleted. Use 'git branch -D {branch_name}' if you really want to drop them.")
        except subprocess.CalledProcessError:
            pass
        print(f"[+] Worktree removed. Task {task_id} stays in {loc}/. Re-run '/q-blueprint {task_id}' if the contract needs changes, or 'quorum task start {task_id}' when ready.")
        return

    if loc in ("done", "failed"):
        target_dir = AI_TASKS / "active" / task_dir.name
        target_dir.parent.mkdir(parents=True, exist_ok=True)
        print(f"[*] Reversing 'clean': moving task from {loc}/ back to active/...")
        shutil.move(str(task_dir), str(target_dir))
        print(f"[+] Task {task_id} restored to active/.")
        return

    if loc == "active":
        target_dir = AI_TASKS / "inbox" / task_dir.name
        target_dir.parent.mkdir(parents=True, exist_ok=True)
        print(f"[*] Reversing 'blueprint': moving task from active/ back to inbox/...")
        shutil.move(str(task_dir), str(target_dir))
        print(f"[+] Task {task_id} returned to inbox/. Re-run '/q-brief {task_id}' to refine the spec.")
        return

    if loc == "inbox":
        print(f"[!] Task {task_id} is already in inbox/. There is no earlier state. Edit '00-spec.yaml' directly or delete the directory if you want to start over.")
        return


def split_task(parent_id):
    """Materialise child tasks from a parent's `decomposition` field.

    Reads `00-spec.yaml.decomposition` from the parent and creates one
    child task directory per entry under `inbox/`. Each child gets its own
    `00-spec.yaml` derived from the parent (parent-task linkage, inherited
    risk floor, declared `summary`/`depends_on`).

    Idempotent: re-running on a parent whose children already exist is a
    no-op for each existing child; new entries in `decomposition` are
    materialised on demand.
    """
    if not PARENT_ID_RE.match(parent_id):
        print(f"[!] '{parent_id}' is not a valid parent task ID (expected '<PREFIX>-<NUMBER>', e.g. FEAT-001).")
        return

    parent_dir, loc = find_task_dir(parent_id)
    if not parent_dir:
        print(f"[!] Parent task {parent_id} not found.")
        return
    if loc != "active":
        print(f"[!] Parent task {parent_id} must be in active/ before splitting (currently in {loc}/).")
        return

    spec_path = parent_dir / "00-spec.yaml"
    if not spec_path.exists():
        print(f"[!] Parent {parent_id} has no 00-spec.yaml.")
        return

    with open(spec_path) as f:
        parent_spec = yaml.safe_load(f) or {}

    if parent_spec.get("parent_task"):
        print(f"[!] Task {parent_id} is already a child task; recursive decomposition is not supported.")
        return

    try:
        _validate_spec(parent_spec)
    except Exception as e:
        print(f"[!] Parent spec validation failed for {parent_id}: {e}")
        return

    decomposition = parent_spec.get("decomposition")
    if not decomposition:
        print(f"[!] Parent {parent_id} has no `decomposition` field. Run '/q-decompose {parent_id}' first to author it.")
        return
    if not isinstance(decomposition, list):
        print(f"[!] Parent {parent_id} has malformed `decomposition`; expected a list.")
        return

    child_ids = []
    for entry in decomposition:
        if not isinstance(entry, dict):
            print(f"[!] Malformed decomposition entry (expected object): {entry}")
            return
        child_id = entry.get("child_id")
        match = CHILD_ID_RE.match(child_id or "")
        if not match or match.group(1) != parent_id:
            print(f"[!] Malformed child_id '{child_id}' (expected '{parent_id}-<a..z>').")
            return
        if child_id in child_ids:
            print(f"[!] Duplicate child_id '{child_id}' in decomposition.")
            return
        child_ids.append(child_id)

    child_id_set = set(child_ids)
    for entry in decomposition:
        child_id = entry["child_id"]
        for dep in entry.get("depends_on", []) or []:
            if dep not in child_id_set:
                print(f"[!] Child {child_id} depends on unknown sibling '{dep}'.")
                return

    # Reject sibling dependency cycles before creating any directories.
    graph = {entry["child_id"]: list(entry.get("depends_on", []) or []) for entry in decomposition}
    visiting = set()
    visited = set()

    def visit(node):
        if node in visited:
            return False
        if node in visiting:
            return True
        visiting.add(node)
        for dep in graph.get(node, []):
            if visit(dep):
                return True
        visiting.remove(node)
        visited.add(node)
        return False

    if any(visit(node) for node in graph):
        print(f"[!] Decomposition for {parent_id} contains a dependency cycle.")
        return

    parent_risk = parent_spec.get("risk", "medium")
    parent_invariants = parent_spec.get("invariants", [])
    parent_acceptance = parent_spec.get("acceptance", [])
    parent_non_goals = parent_spec.get("non_goals", [])
    parent_constraints = parent_spec.get("constraints", [])

    created = []
    skipped = []
    for entry in decomposition:
        child_id = entry.get("child_id")
        if not child_id:
            print(f"[!] Skipping decomposition entry without child_id: {entry}")
            continue
        if not CHILD_ID_RE.match(child_id):
            print(f"[!] Skipping malformed child_id '{child_id}' (expected '<PARENT>-<a..z>').")
            continue

        existing_dir, _ = find_task_dir(child_id)
        if existing_dir:
            skipped.append(child_id)
            continue

        child_summary = entry.get("summary", f"Child of {parent_id}.")
        child_depends_on = entry.get("depends_on", [])

        child_spec = {
            "task_id": child_id,
            "summary": child_summary,
            "goal": f"Subset of {parent_id}: {child_summary}",
            "invariants": list(parent_invariants),
            "acceptance": list(parent_acceptance),
            "risk": parent_risk,
            "parent_task": parent_id,
        }
        if parent_non_goals:
            child_spec["non_goals"] = list(parent_non_goals)
        if parent_constraints:
            child_spec["constraints"] = list(parent_constraints)
        if child_depends_on:
            child_spec["depends_on"] = list(child_depends_on)

        try:
            _validate_spec(child_spec)
        except Exception as e:
            print(f"[!] Generated child spec for {child_id} is invalid: {e}")
            return

        child_dir = AI_TASKS / "inbox" / f"{child_id}-new-spec"
        child_dir.mkdir(parents=True, exist_ok=True)

        with open(child_dir / "00-spec.yaml", "w") as f:
            yaml.safe_dump(child_spec, f, sort_keys=False)

        created.append(child_id)

    if created:
        print(f"[+] Created {len(created)} child task(s) in inbox/: {', '.join(created)}")
    if skipped:
        print(f"[*] Skipped existing children: {', '.join(skipped)}")
    if not created and not skipped:
        print(f"[!] No children materialised; check the decomposition entries in {spec_path}.")
    else:
        print(f"[!] Each child still needs '/q-brief <child_id>' to refine its own spec, then 'quorum task blueprint <child_id>' will be auto-run by the skill on success.")
