from pathlib import Path
import os
import shutil

def init():
    # Detect project root
    project_root = Path(os.getcwd())
    # Resource source is relative to this file in the tool installation
    # .agents/cli/commands/project.py -> .agents/ is 3 levels up
    resource_src = Path(__file__).parent.parent.parent
    
    print(f"[*] Initializing Quorum in {project_root}...")
    
    # 1. Basic directory structure
    dirs = [
        ".ai/tasks/inbox",
        ".ai/tasks/active",
        ".ai/tasks/done",
        ".ai/tasks/failed",
        "memory/decisions",
        "memory/patterns",
        "memory/lessons",
        "worktrees"
    ]
    
    for d in dirs:
        path = project_root / d
        if not path.exists():
            print(f"  [+] Creating {d}/")
            path.mkdir(parents=True, exist_ok=True)
            if d != "worktrees":
                (path / ".gitkeep").touch()

    # 2. Copy Resources (Scaffolding)
    # Map: source_subpath -> target_subpath
    scaffold_map = {
        "templates": ".ai/tasks/_template",
        "skills": ".agents/skills",
        "schemas": ".agents/schemas",
        "policies": ".agents/policies",
        "prompts": ".agents/prompts",
    }
    
    for src_sub, tgt_sub in scaffold_map.items():
        src_path = resource_src / src_sub
        tgt_path = project_root / tgt_sub
        
        if src_path.exists():
            print(f"  [*] Scaffolding {tgt_sub}...")
            if src_path.is_dir():
                shutil.copytree(src_path, tgt_path, dirs_exist_ok=True)
            else:
                shutil.copy2(src_path, tgt_path)
        else:
            # Fallback for local development where templates might still be in .ai/tasks/_template
            if src_sub == "templates":
                fallback_src = resource_src.parent / ".ai" / "tasks" / "_template"
                if fallback_src.exists():
                    print(f"  [*] Scaffolding {tgt_sub} from fallback...")
                    shutil.copytree(fallback_src, tgt_path, dirs_exist_ok=True)
                    continue
            print(f"  [!] Warning: Source {src_sub} not found in {resource_src}")

    # 3. Special handling for config.yaml
    config_src = resource_src / "config.yaml"
    config_tgt = project_root / ".agents" / "config.yaml"
    if config_src.exists():
        print(f"  [*] Updating .agents/config.yaml...")
        config_tgt.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(config_src, config_tgt)

    # 4. .gitignore logic
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
