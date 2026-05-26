import subprocess
import re
import os
import shutil
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent

def normalize_output(text: str) -> str:
    if not text:
        return text
    
    # Normalize absolute paths
    repo_root_str = str(REPO_ROOT)
    text = text.replace(repo_root_str, "<REPO_ROOT>")
    
    # Normalize timestamps (ISO 8601 like 2026-05-24T20:15:06Z or similar)
    text = re.sub(r'\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})?', '<TIMESTAMP>', text)
    
    # Normalize git SHAs (40 chars hex)
    text = re.sub(r'\b[0-9a-f]{40}\b', '<GIT_SHA>', text)
    
    # Normalize short git SHAs in typical git messages
    text = re.sub(r'\b([0-9a-f]{7})\b', '<GIT_SHORT_SHA>', text) # this might be too broad, let's refine
    
    # Normalize UUIDs
    text = re.sub(r'\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b', '<UUID>', text)
    
    return text

def capture_command(args: list[str], env: dict = None) -> dict:
    run_env = os.environ.copy()
    if env:
        run_env.update(env)
    
    # Ensure deterministic output by enforcing standard width and disabling color
    run_env["TERM"] = "dumb"
    run_env["NO_COLOR"] = "1"
    run_env["PYTHONPATH"] = ".agents"
    
    result = subprocess.run(args, cwd=str(REPO_ROOT), env=run_env, capture_output=True, text=True)
    
    return {
        "command": " ".join(args),
        "exit_code": result.returncode,
        "stdout": normalize_output(result.stdout),
        "stderr": normalize_output(result.stderr)
    }

def save_golden_capture(scenario_name: str, capture: dict, golden_dir: Path):
    scenario_dir = golden_dir / scenario_name
    scenario_dir.mkdir(parents=True, exist_ok=True)
    
    (scenario_dir / "stdout.txt").write_text(capture.get("stdout", ""))
    (scenario_dir / "stderr.txt").write_text(capture.get("stderr", ""))
    (scenario_dir / "exit_code.txt").write_text(str(capture.get("exit_code", 0)))

def generate_corpus(golden_dir: Path):
    if golden_dir.exists():
        shutil.rmtree(golden_dir)
    golden_dir.mkdir(parents=True)
    
    go_cli = ["./quorum_go"]
    
    # 1. Normal task list
    capture = capture_command(go_cli + ["task", "list"])
    save_golden_capture("task_list", capture, golden_dir)
    
    # 2. Invalid spec / validation error (field=$.path; reason=...)
    broken_task_id = "BROKEN-999"
    broken_dir = REPO_ROOT / ".ai" / "tasks" / "inbox" / broken_task_id
    broken_dir.mkdir(parents=True, exist_ok=True)
    broken_spec = broken_dir / "00-spec.yaml"
    broken_spec.write_text("task_id: BROKEN-999\nsummary: broken\n")
    
    capture = capture_command(go_cli + ["task", "blueprint", broken_task_id])
    save_golden_capture("validation_error", capture, golden_dir)
    
    shutil.rmtree(broken_dir, ignore_errors=True)
    active_broken_dir = REPO_ROOT / ".ai" / "tasks" / "active" / broken_task_id
    shutil.rmtree(active_broken_dir, ignore_errors=True)
    
    # 3. Ambiguous task ID resolution
    ambig_1 = REPO_ROOT / ".ai" / "tasks" / "inbox" / "AMBIG-100"
    ambig_2 = REPO_ROOT / ".ai" / "tasks" / "inbox" / "AMBIG-1000"
    ambig_1.mkdir(parents=True, exist_ok=True)
    ambig_2.mkdir(parents=True, exist_ok=True)
    (ambig_1 / "00-spec.yaml").write_text("task_id: AMBIG-100\n")
    (ambig_2 / "00-spec.yaml").write_text("task_id: AMBIG-1000\n")
    
    capture = capture_command(go_cli + ["task", "status", "AMBIG-100"])
    save_golden_capture("ambiguous_resolution", capture, golden_dir)
    
    shutil.rmtree(ambig_1, ignore_errors=True)
    shutil.rmtree(ambig_2, ignore_errors=True)

if __name__ == "__main__":
    golden_dir = REPO_ROOT / "tests" / "golden"
    generate_corpus(golden_dir)
