from pathlib import Path
import os

def init():
    # Detect project root (using the same logic as task_manager if needed, 
    # but let's just use current directory for bootstrapping)
    project_root = Path(os.getcwd())
    
    print(f"[*] Initializing Quorum in {project_root}...")
    
    # Required structure
    dirs = [
        ".ai/tasks/inbox",
        ".ai/tasks/active",
        ".ai/tasks/done",
        ".ai/tasks/failed",
        ".ai/tasks/_template",
        "memory/decisions",
        "memory/patterns",
        "memory/lessons",
        "worktrees"
    ]
    
    for d in dirs:
        path = project_root / d
        if not path.exists():
            print(f"  [+] Creating {d}")
            path.mkdir(parents=True, exist_ok=True)
            # Add .gitkeep to all except worktrees (which should be ignored)
            if d != "worktrees":
                (path / ".gitkeep").touch()
        else:
            print(f"  [ ] {d} already exists")

    # Add to .gitignore if it exists
    gitignore_path = project_root / ".gitignore"
    ignore_entries = [
        "\n# Quorum",
        "worktrees/",
        ".ai/tasks/active/*",
        ".ai/tasks/done/*",
        ".ai/tasks/failed/*",
        ".ai/tasks/inbox/*",
        "!.ai/tasks/active/.gitkeep",
        "!.ai/tasks/done/.gitkeep",
        "!.ai/tasks/failed/.gitkeep",
        "!.ai/tasks/inbox/.gitkeep"
    ]
    
    if gitignore_path.exists():
        content = gitignore_path.read_text()
        new_entries = [e for e in ignore_entries if e.strip() and e.strip() not in content]
        if new_entries:
            print(f"[*] Updating .gitignore...")
            with open(gitignore_path, "a") as f:
                for entry in new_entries:
                    f.write(f"{entry}\n")
    else:
        print(f"[*] Creating .gitignore...")
        with open(gitignore_path, "w") as f:
            for entry in ignore_entries:
                f.write(f"{entry}\n")

    print(f"[+] Quorum initialized successfully.")
