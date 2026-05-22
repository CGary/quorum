import os
import shutil
from pathlib import Path

import pytest

from cli.commands import project


def _resource_src() -> Path:
    return Path(project.__file__).parent.parent.parent

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
        assert (tmp_path / ".agents/schemas/implementation-log.schema.json").is_file()
        
        # 3. Verify .gitignore
        gitignore = (tmp_path / ".gitignore").read_text()
        assert "worktrees/" in gitignore
        assert ".ai/tasks/active/*" in gitignore
        
    finally:
        os.chdir(old_cwd)


def test_init_creates_claude_skills_symlink(tmp_path):
    old_cwd = os.getcwd()
    os.chdir(tmp_path)
    try:
        project.init()

        link = tmp_path / ".claude" / "skills"
        assert link.is_symlink()

        expected_target = (_resource_src() / "skills").resolve()
        assert link.resolve() == expected_target

        # Discoverabilidad: las skills q-* del install efectivo aparecen vía el symlink.
        source_q = sorted(p.name for p in expected_target.glob("q-*"))
        through_link = sorted(p.name for p in link.glob("q-*"))
        assert source_q, "Expected resource skills to contain q-* entries"
        assert through_link == source_q
    finally:
        os.chdir(old_cwd)


def test_init_is_idempotent_on_existing_symlink(tmp_path):
    old_cwd = os.getcwd()
    os.chdir(tmp_path)
    try:
        project.init()
        link = tmp_path / ".claude" / "skills"
        assert link.is_symlink()
        before_readlink = os.readlink(link)
        before_resolved = link.resolve()

        # Re-init should be a no-op for the symlink: same type, same target.
        project.init()

        assert link.is_symlink()
        assert os.readlink(link) == before_readlink
        assert link.resolve() == before_resolved
    finally:
        os.chdir(old_cwd)


def test_init_rejects_regular_directory_at_claude_skills(tmp_path):
    old_cwd = os.getcwd()
    os.chdir(tmp_path)
    try:
        claude_skills = tmp_path / ".claude" / "skills"
        claude_skills.mkdir(parents=True)
        marker = claude_skills / "user-content.txt"
        marker.write_text("preserve me")

        with pytest.raises(RuntimeError) as excinfo:
            project.init()

        msg = str(excinfo.value)
        assert ".claude/skills" in msg
        assert "directorio" in msg
        # El contenido pre-existente queda intacto.
        assert claude_skills.is_dir()
        assert not claude_skills.is_symlink()
        assert marker.read_text() == "preserve me"
    finally:
        os.chdir(old_cwd)


def test_init_rejects_foreign_symlink_target(tmp_path):
    old_cwd = os.getcwd()
    os.chdir(tmp_path)
    try:
        foreign_target = tmp_path / "elsewhere"
        foreign_target.mkdir()
        (foreign_target / "marker").write_text("foreign")

        claude_dir = tmp_path / ".claude"
        claude_dir.mkdir()
        link = claude_dir / "skills"
        link.symlink_to(foreign_target, target_is_directory=True)

        with pytest.raises(RuntimeError) as excinfo:
            project.init()

        msg = str(excinfo.value)
        assert ".claude/skills" in msg
        assert "destino" in msg
        # El symlink ajeno y su destino no fueron sobrescritos.
        assert link.is_symlink()
        assert link.resolve() == foreign_target.resolve()
        assert (foreign_target / "marker").read_text() == "foreign"
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
