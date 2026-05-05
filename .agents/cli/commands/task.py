import json
import sys
import yaml

from ..core import task_manager


def _read_payload_from_stdin(artifact_path):
    raw = sys.stdin.read()
    if artifact_path.endswith('.json'):
        return json.loads(raw)
    return yaml.safe_load(raw)


def specify(task_id=None):
    print(f"[*] Initializing specification session...")
    path = task_manager.initialize_specify(task_id)
    print(f"[+] Task directory created: {path}")
    print(f"[!] Please use the '/q-brief' skill to fill '00-spec.yaml'.")


def blueprint(task_id):
    print(f"[*] Generating technical blueprint for {task_id}...")
    task_manager.prepare_blueprint(task_id)
    print(f"[!] Please use the '/q-blueprint' skill to generate '01-blueprint.yaml' and '02-contract.yaml'.")


def start(task_id):
    print(f"[*] Starting task {task_id}...")
    task_manager.start_task(task_id)


def run(task_id):
    print(f"[*] Running task {task_id}...")
    task_manager.run_task(task_id)


def status(task_id):
    task_manager.show_status(task_id)


def clean(task_id):
    print(f"[*] Cleaning up task {task_id}...")
    task_manager.clean_task(task_id)


def list_all():
    task_manager.list_tasks()


def back(task_id):
    print(f"[*] Reverting task {task_id} to its previous state...")
    task_manager.back_task(task_id)


def split(parent_id):
    print(f"[*] Materialising children for {parent_id}...")
    task_manager.split_task(parent_id)


def artifact_save(task_id, artifact_path):
    task_dir, _ = task_manager.find_task_dir(task_id)
    if not task_dir:
        print(f"[!] Task {task_id} not found.")
        raise SystemExit(1)

    destination = task_dir / artifact_path
    try:
        payload = _read_payload_from_stdin(artifact_path)
        task_manager.save_artifact(destination, payload)
    except (json.JSONDecodeError, yaml.YAMLError) as error:
        print(f"[!] artifact={destination}; field=$; reason=payload parse failed: {error}")
        raise SystemExit(1) from error
    except task_manager.ArtifactValidationError as error:
        print(f"[!] {error}")
        raise SystemExit(1) from error

    print(f"[+] Saved artifact: {destination}")
