from ..core import task_manager

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
