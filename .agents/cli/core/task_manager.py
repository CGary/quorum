import sys
import shutil
import yaml
import json
import subprocess
import os
import re
from pathlib import Path
from jsonschema import validate, ValidationError
import datetime
from cli.core.decomposition_render import render_ascii_dag
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
def get_execution_context():
    """Detect whether CWD is the main repo root or a linked worktree.

    Returns a (mode, identifier) tuple:
      - ("root", None) when --git-dir and --git-common-dir resolve to the same path.
      - ("worktree", "<basename of --show-toplevel>") when they differ.
      - (None, None) when git rev-parse fails (cwd outside any git repo).

    The identifier is derived exclusively from `git rev-parse --show-toplevel`;
    callers must not hardcode IDs or infer them from directory naming conventions.
    """
    try:
        git_dir = subprocess.run(
            ["git", "rev-parse", "--git-dir"],
            check=True, capture_output=True, text=True
        ).stdout.strip()
        common_dir = subprocess.run(
            ["git", "rev-parse", "--git-common-dir"],
            check=True, capture_output=True, text=True
        ).stdout.strip()
    except Exception:
        return None, None
    if Path(git_dir).resolve() == Path(common_dir).resolve():
        return "root", None
    try:
        toplevel = subprocess.run(
            ["git", "rev-parse", "--show-toplevel"],
            check=True, capture_output=True, text=True
        ).stdout.strip()
    except Exception:
        return None, None
    return "worktree", Path(toplevel).name
def render_context_prefix():
    """Return the CLI context prefix line for the current execution context.

    Emits "[root]" or "[worktree:<ID>]" based on get_execution_context().
    Returns an empty string when git rev-parse fails so callers can omit the
    prefix without aborting the underlying command.
    """
    mode, ident = get_execution_context()
    if mode == "root":
        return "[root]"
    if mode == "worktree" and ident:
        return f"[worktree:{ident}]"
    return ""
AGENTS_DIR = PROJECT_ROOT / ".agents" # Note: Local project settings if they exist
SCHEMAS_DIR = Path(__file__).parent.parent.parent / "schemas" # Schemas are usually in the tool source
POLICIES_DIR = Path(__file__).parent.parent.parent / "policies"
TEMPLATES_DIR = Path(__file__).parent.parent.parent / "templates"
CHILD_ID_RE = re.compile(r"^([A-Z]+-[0-9]+)-([a-z])$")
PARENT_ID_RE = re.compile(r"^[A-Z]+-[0-9]+$")
ARTIFACT_SCHEMA_MAP = {
    "00-spec.yaml": "spec.schema.json",
    "01-blueprint.yaml": "blueprint.schema.json",
    "02-contract.yaml": "contract.schema.json",
    "04-implementation-log.yaml": "implementation-log.schema.json",
    "05-validation.json": "validation.schema.json",
    "06-review.json": "review.schema.json",
    "07-trace.json": "trace.schema.json",
}
class ArtifactValidationError(Exception):
    """Raised when an artifact cannot be safely persisted."""
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
def _artifact_schema_name(artifact_path: Path):
    name = artifact_path.name
    if name in ARTIFACT_SCHEMA_MAP:
        return ARTIFACT_SCHEMA_MAP[name]
    if "memory" in artifact_path.parts and artifact_path.suffix == ".json":
        return "memory.schema.json"
    return None
def _load_artifact_payload(path: Path):
    raw = path.read_text()
    if path.suffix == ".json":
        return json.loads(raw)
    return yaml.safe_load(raw)
def _dump_artifact_payload(path: Path, payload):
    if path.suffix == ".json":
        path.write_text(json.dumps(payload, indent=2, ensure_ascii=False) + "\n")
        return
    path.write_text(yaml.safe_dump(payload, sort_keys=False, allow_unicode=True))
def _json_pointer(path_parts):
    if not path_parts:
        return "$"
    rendered = "$"
    for part in path_parts:
        if isinstance(part, int):
            rendered += f"[{part}]"
        else:
            rendered += f".{part}"
    return rendered
def _format_validation_error(artifact_path: Path, error: ValidationError) -> str:
    return (
        f"artifact={artifact_path}; field={_json_pointer(error.path)}; "
        f"reason={error.message}"
    )
def validate_artifact(artifact_path, payload):
    artifact_path = Path(artifact_path)
    schema_name = _artifact_schema_name(artifact_path)
    if not schema_name:
        raise ArtifactValidationError(
            f"artifact={artifact_path}; field=$; reason=unsupported artifact path"
        )
    try:
        validate(instance=payload, schema=_load_json_schema(schema_name))
    except ValidationError as error:
        raise ArtifactValidationError(_format_validation_error(artifact_path, error)) from error
def _ensure_trace_append_only(artifact_path: Path, existing_payload, new_payload):
    existing_attempts = list((existing_payload or {}).get("attempts") or [])
    new_attempts = list((new_payload or {}).get("attempts") or [])
    if len(new_attempts) < len(existing_attempts):
        raise ArtifactValidationError(
            f"artifact={artifact_path}; field=$.attempts; reason=append-only trace cannot remove existing attempts"
        )
    if new_attempts[:len(existing_attempts)] != existing_attempts:
        raise ArtifactValidationError(
            f"artifact={artifact_path}; field=$.attempts; reason=append-only trace cannot reorder or mutate existing attempts"
        )
def save_artifact(artifact_path, payload):
    artifact_path = Path(artifact_path)
    existing_payload = None
    if artifact_path.exists():
        existing_payload = _load_artifact_payload(artifact_path)
    if artifact_path.name == "07-trace.json" and existing_payload is not None:
        _ensure_trace_append_only(artifact_path, existing_payload, payload)
    validate_artifact(artifact_path, payload)
    artifact_path.parent.mkdir(parents=True, exist_ok=True)
    _dump_artifact_payload(artifact_path, payload)
    return artifact_path
def append_trace_attempt(trace_path, attempt):
    trace_path = Path(trace_path)
    if not trace_path.exists():
        raise ArtifactValidationError(
            f"artifact={trace_path}; field=$; reason=trace file must exist before appending attempts"
        )
    trace_payload = _load_artifact_payload(trace_path)
    attempts = list(trace_payload.get("attempts") or [])
    attempts.append(attempt)
    trace_payload["attempts"] = attempts
    save_artifact(trace_path, trace_payload)
    return trace_payload
def _validate_implementation_log(log):
    """Validate a 04-implementation-log.yaml payload against its schema."""
    validate(instance=log, schema=_load_json_schema("implementation-log.schema.json"))
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
    task_dir = AI_TASKS / "inbox" / task_id
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
        save_artifact(spec_path, spec)
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
    contract = _load_artifact_payload(contract_path)
    try:
        validate_artifact(contract_path, contract)
    except ArtifactValidationError as e:
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

    # 5. Initialize implementation log (04-implementation-log.yaml)
    log_path = task_dir / "04-implementation-log.yaml"
    if not log_path.exists():
        log = {
            "task_id": task_id,
            "summary": contract.get("summary", "Implementation log initialized."),
            "entries": []
        }
        try:
            save_artifact(log_path, log)
        except ArtifactValidationError as e:
            print(f"[!] Error initializing implementation log: {e}")
            return

    # 6. Initialize trace.json (07-trace.json)
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
        save_artifact(trace_path, trace)
    print(f"[+] Task {task_id} initialized and worktree ready.")
def _derive_parent_state(spec):
    """Derive a parent task's state from its children's current locations.

    Returns one of:
      - "partial":   at least one child resides in failed/.
      - "completed": every child resides in done/ (and none in failed/).
      - "active":    otherwise (children still in inbox/active or missing).

    The state is never persisted; it is recomputed every call so retries
    that move a child out of failed/ revert the parent automatically.
    """
    decomposition = spec.get("decomposition") or []
    child_locs = []
    for entry in decomposition:
        child_id = entry.get("child_id") if isinstance(entry, dict) else None
        if not child_id:
            continue
        _, child_loc = find_task_dir(child_id)
        child_locs.append(child_loc)
    if any(loc == "failed" for loc in child_locs):
        return "partial"
    if child_locs and all(loc == "done" for loc in child_locs):
        return "completed"
    return "active"
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
    for art in ["00-spec.yaml", "01-blueprint.yaml", "02-contract.yaml", "04-implementation-log.yaml", "05-validation.json", "06-review.json", "07-trace.json"]:
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
                print(f"- parent_state: {_derive_parent_state(spec)}")
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
def _is_worktree_dirty(worktree_path):
    """Returns True if the worktree has uncommitted changes (tracked or untracked)."""
    result = subprocess.run(
        ["git", "-C", str(worktree_path), "status", "--porcelain"],
        capture_output=True, text=True
    )
    if result.returncode != 0:
        return False
    return bool(result.stdout.strip())


def _save_worktree_changes(worktree_path, task_id):
    """Creates a stash entry preserving worktree changes (including untracked) before removal."""
    message = f"quorum:save:{task_id}"
    result = subprocess.run(
        ["git", "-C", str(worktree_path), "stash", "push", "--include-untracked", "-m", message],
        capture_output=True, text=True
    )
    return result.returncode == 0, (result.stdout or "") + (result.stderr or "")


def _ensure_retry_worktree(task_id):
    """Ensure a failed child has a safe worktree for a new /q-implement dispatch.

    Existing worktrees are never reset or removed here. If they contain dirty
    changes, retry is blocked so the human can decide whether to keep, stash, or
    discard that work outside this helper. Missing worktrees are recreated from
    the existing ai/<TASK> branch when present, otherwise from the detected base
    branch.
    """
    worktree_path = PROJECT_ROOT / "worktrees" / task_id
    if worktree_path.exists():
        if _is_worktree_dirty(worktree_path):
            print(f"[!] Worktree {worktree_path} has uncommitted changes.")
            print(f"[!] Refusing retry for {task_id} until the worktree is clean.")
            print(f"      cd {worktree_path} && git status")
            return False
        return True

    branch_name = f"ai/{task_id}"
    branch_check = subprocess.run(
        ["git", "-C", str(PROJECT_ROOT), "rev-parse", "--verify", branch_name],
        capture_output=True, text=True
    )
    if branch_check.returncode == 0:
        cmd = ["git", "-C", str(PROJECT_ROOT), "worktree", "add", str(worktree_path), branch_name]
    else:
        cmd = [
            "git", "-C", str(PROJECT_ROOT), "worktree", "add",
            str(worktree_path), "-b", branch_name, get_base_branch()
        ]

    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        output = (result.stderr or result.stdout or "").strip()
        print(f"[!] Could not prepare retry worktree for {task_id}: {output}")
        return False
    return True


def _clear_retry_artifacts(task_dir):
    """Remove stale terminal artifacts from a failed attempt before retrying.

    07-trace.json is deliberately excluded so attempts remain append-only and
    historical validation/review outcomes stay available in Git history.
    """
    removed = []
    for name in ["05-validation.json", "06-review.json"]:
        path = task_dir / name
        if path.exists():
            path.unlink()
            removed.append(name)
    return removed


def _restore_parent_for_child_retry(parent_id):
    """Ensure a retried child's parent is active and observable during work."""
    parent_dir, parent_loc = find_task_dir(parent_id, ["active", "done"])
    if not parent_dir:
        print(f"[!] Parent task {parent_id} not found for retry.")
        return False
    if parent_loc == "active":
        return True

    target_dir = AI_TASKS / "active" / parent_dir.name
    if target_dir.exists():
        print(f"[!] Cannot restore parent {parent_id}: active/{parent_dir.name} already exists.")
        return False
    print(f"[*] Restoring parent task {parent_id} from done/ to active/ for child retry...")
    target_dir.parent.mkdir(parents=True, exist_ok=True)
    shutil.move(str(parent_dir), str(target_dir))
    return True


def prepare_failed_child_retry(task_id):
    """Prepare a failed child task for an externally requested /q-implement retry.

    This is not a rollback path and does not invoke `quorum task back`. It only
    performs the ADR-authorized retry setup for child tasks: validate that the
    task is a failed child, keep 07-trace.json intact, remove stale terminal
    validation/review artifacts, restore the parent to active when needed, and
    move the child back to active.
    """
    task_dir, loc = find_task_dir(task_id, ["active", "failed"])
    if not task_dir:
        print(f"[!] Task {task_id} not found in active/ or failed/.")
        return False
    if loc == "active":
        print(f"[*] Task {task_id} is already active; retry preparation not needed.")
        return True
    if loc != "failed":
        print(f"[!] Task {task_id} is in {loc}/; retry preparation only handles failed/ children.")
        return False

    spec_path = task_dir / "00-spec.yaml"
    if not spec_path.exists():
        print(f"[!] Cannot retry {task_id}: missing 00-spec.yaml.")
        return False
    try:
        spec = yaml.safe_load(spec_path.read_text()) or {}
        _validate_spec(spec)
    except Exception as error:
        print(f"[!] Cannot retry {task_id}: invalid 00-spec.yaml: {error}")
        return False

    parent_id = spec.get("parent_task")
    if not parent_id:
        print(f"[!] Retry is only authorized for failed child tasks; {task_id} has no parent_task.")
        return False

    active_target = AI_TASKS / "active" / task_dir.name
    if active_target.exists():
        print(f"[!] Cannot retry {task_id}: active/{task_dir.name} already exists.")
        return False

    trace_path = task_dir / "07-trace.json"
    if not trace_path.exists():
        print(f"[!] Cannot retry {task_id}: missing 07-trace.json to preserve attempts history.")
        return False
    try:
        validate_artifact(trace_path, _load_artifact_payload(trace_path))
    except ArtifactValidationError as error:
        print(f"[!] Cannot retry {task_id}: invalid 07-trace.json: {error}")
        return False

    if not _ensure_retry_worktree(task_id):
        return False
    if not _restore_parent_for_child_retry(parent_id):
        return False

    removed = _clear_retry_artifacts(task_dir)
    active_target.parent.mkdir(parents=True, exist_ok=True)
    shutil.move(str(task_dir), str(active_target))
    if removed:
        print(f"[*] Removed stale retry artifacts for {task_id}: {', '.join(removed)}")
    print(f"[+] Failed child task {task_id} restored to active/ for /q-implement retry.")
    return True


def clean_task(task_id, force=False, save=False):
    if force and save:
        print(f"[!] --force and --save are mutually exclusive. Pick one: --force discards changes, --save stashes them.")
        return
    task_dir, loc = find_task_dir(task_id, ["active", "done", "failed"])
    if not task_dir:
        print(f"[!] Task {task_id} not found.")
        return
    spec_path = task_dir / "00-spec.yaml"
    parent_id = None
    if loc == "active" and spec_path.exists():
        try:
            with open(spec_path) as f:
                spec = yaml.safe_load(f) or {}
            decomposition = spec.get("decomposition") or []
            parent_id = spec.get("parent_task")
        except Exception:
            decomposition = []
            parent_id = None
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
        dirty = _is_worktree_dirty(worktree_path)
        if dirty and not force and not save:
            print(f"[!] Worktree {worktree_path} has uncommitted changes.")
            print(f"[!] Refusing to clean task {task_id} silently. Choose one of:")
            print(f"      cd {worktree_path} && git status      # inspect changes")
            print(f"      cd {worktree_path} && git commit -am '...'  # commit, then re-run clean")
            print(f"      quorum task clean {task_id} --save     # stash WIP and clean")
            print(f"      quorum task clean {task_id} --force    # discard WIP and clean")
            return
        if dirty and save:
            print(f"[*] Saving worktree changes as stash 'quorum:save:{task_id}'...")
            ok, output = _save_worktree_changes(worktree_path, task_id)
            if not ok:
                print(f"[!] git stash push failed: {output.strip()}")
                return
        remove_cmd = ["git", "-C", str(PROJECT_ROOT), "worktree", "remove", str(worktree_path)]
        if force and dirty:
            remove_cmd.append("--force")
            print(f"[*] Force-removing worktree {worktree_path} (discarding changes)...")
        else:
            print(f"[*] Removing worktree {worktree_path}...")
        subprocess.run(remove_cmd, check=False)
    if loc == "active":
        target_dir = AI_TASKS / "done" / task_dir.name
        print(f"[*] Archiving task to done/...")
        shutil.move(str(task_dir), str(target_dir))
    print(f"[+] Task {task_id} cleaned up.")
    # Auto-archive the parent when this child completes the decomposition.
    # The transition belongs to the CLI (quorum task clean), not to any skill.
    if parent_id:
        _auto_archive_parent_if_complete(parent_id)
def _auto_archive_parent_if_complete(parent_id):
    """If `parent_id` is still active and all its declared children are in done/,
    archive the parent to done/ as a side-effect of the same CLI transition.

    No-op when the parent is missing, already archived, has no decomposition,
    or still has at least one child outside done/. Skills must never call this
    directly — clean_task is the only authorized entry point."""
    parent_dir, parent_loc = find_task_dir(parent_id, ["active"])
    if not parent_dir or parent_loc != "active":
        return
    parent_spec_path = parent_dir / "00-spec.yaml"
    if not parent_spec_path.exists():
        return
    try:
        with open(parent_spec_path) as f:
            parent_spec = yaml.safe_load(f) or {}
    except Exception:
        return
    decomposition = parent_spec.get("decomposition") or []
    if not decomposition:
        return
    for entry in decomposition:
        child_id = entry.get("child_id") if isinstance(entry, dict) else None
        if not child_id:
            return
        _, child_loc = find_task_dir(child_id)
        if child_loc != "done":
            return
    target_dir = AI_TASKS / "done" / parent_dir.name
    print(f"[*] All children of {parent_id} are in done/; auto-archiving parent...")
    shutil.move(str(parent_dir), str(target_dir))
    print(f"[+] Parent task {parent_id} archived to done/.")
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
        child_dir = AI_TASKS / "inbox" / child_id
        child_dir.mkdir(parents=True, exist_ok=True)
        save_artifact(child_dir / "00-spec.yaml", child_spec)
        created.append(child_id)
    if created:
        print(f"[+] Created {len(created)} child task(s) in inbox/: {', '.join(created)}")
    if skipped:
        print(f"[*] Skipped existing children: {', '.join(skipped)}")
    if not created and not skipped:
        print(f"[!] No children materialised; check the decomposition entries in {spec_path}.")
    else:
        dag = render_ascii_dag(decomposition)
        if dag:
            print(dag)
        print(f"[!] Each child still needs '/q-brief <child_id>' to refine its own spec, then 'quorum task blueprint <child_id>' will be auto-run by the skill on success.")
