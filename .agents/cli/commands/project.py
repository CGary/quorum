from pathlib import Path
import os
import shutil


def _ensure_claude_skills_symlink(project_root: Path, resource_src: Path) -> None:
    """Create or validate .claude/skills as a symlink to resource_src/skills.

    Idempotent when the symlink already resolves to the expected target.
    Refuses to overwrite any pre-existing content (regular file, directory,
    or foreign symlink) and raises RuntimeError with an actionable message.
    """
    claude_dir = project_root / ".claude"
    link_path = claude_dir / "skills"
    expected_target = (resource_src / "skills").resolve()

    claude_dir.mkdir(exist_ok=True)

    if link_path.is_symlink():
        current_target = link_path.resolve()
        if current_target == expected_target:
            print(f"  [=] .claude/skills already linked to {expected_target}")
            return
        raise RuntimeError(
            ".claude/skills es un symlink hacia un destino distinto al esperado.\n"
            f"  Encontrado: {current_target}\n"
            f"  Esperado:   {expected_target}\n"
            "Eliminá o ajustá el symlink manualmente antes de re-ejecutar quorum init."
        )
    if link_path.exists():
        kind = "directorio" if link_path.is_dir() else "archivo"
        raise RuntimeError(
            f".claude/skills existe como {kind} y no como symlink.\n"
            f"  Encontrado: {link_path} ({kind})\n"
            f"  Esperado:   symlink hacia {expected_target}\n"
            "Eliminá o renombrá el contenido existente antes de re-ejecutar quorum init."
        )

    print(f"  [+] Creating .claude/skills -> {expected_target}")
    link_path.symlink_to(expected_target, target_is_directory=True)


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

    # 3b. Expose skills to Claude Code via .claude/skills symlink
    _ensure_claude_skills_symlink(project_root, resource_src)

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
