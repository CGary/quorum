import os
import shutil
from pathlib import Path
from cli.commands import project

def test_init_scaffolding(tmp_path):
    # Change current working directory to tmp_path for the test
    old_cwd = os.getcwd()
    os.chdir(tmp_path)
    
    try:
        # Run init
        project.init()
        
        # 1. Verify directories
        assert (tmp_path / ".ai/tasks/inbox").is_dir()
        assert (tmp_path / "memory/patterns").is_dir()
        assert (tmp_path / "worktrees").is_dir()
        
        # 2. Verify Scaffolding
        # Templates
        assert (tmp_path / ".ai/tasks/_template/00-spec.yaml").is_file()
        assert (tmp_path / ".ai/tasks/_template/01-blueprint.yaml").is_file()
        assert (tmp_path / ".ai/tasks/_template/02-contract.yaml").is_file()
        
        # .agents resources
        assert (tmp_path / ".agents/skills").is_dir()
        assert (tmp_path / ".agents/schemas").is_dir()
        assert (tmp_path / ".agents/policies").is_dir()
        assert (tmp_path / ".agents/config.yaml").is_file()
        
        # Check one specific schema
        assert (tmp_path / ".agents/schemas/spec.schema.json").is_file()
        
        # 3. Verify .gitignore
        gitignore = (tmp_path / ".gitignore").read_text()
        assert "worktrees/" in gitignore
        assert ".ai/tasks/active/*" in gitignore
        
    finally:
        os.chdir(old_cwd)

def test_init_overwrite(tmp_path):
    old_cwd = os.getcwd()
    os.chdir(tmp_path)
    
    try:
        # Create a dummy config
        config_path = tmp_path / ".agents" / "config.yaml"
        config_path.parent.mkdir(parents=True)
        config_path.write_text("old_config: true")
        
        # Run init
        project.init()
        
        # Verify it was overwritten
        content = config_path.read_text()
        assert "old_config" not in content
        assert "levels:" in content # Assuming standard config has levels
        
    finally:
        os.chdir(old_cwd)
