import pytest
import sys
from pathlib import Path
REPO_ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(REPO_ROOT))

from tests.golden_master import generate_corpus
import shutil

REPO_ROOT = Path(__file__).resolve().parent.parent

def test_golden_corpus_stable(tmp_path):
    golden_dir = tmp_path / "golden"
    
    # Run first generation
    generate_corpus(golden_dir)
    
    # Capture the state of the first generation
    first_state = {}
    for p in golden_dir.rglob("*"):
        if p.is_file():
            first_state[str(p.relative_to(golden_dir))] = p.read_text()
            
    # Run second generation
    generate_corpus(golden_dir)
    
    # Capture the state of the second generation
    second_state = {}
    for p in golden_dir.rglob("*"):
        if p.is_file():
            second_state[str(p.relative_to(golden_dir))] = p.read_text()
            
    # Assert zero differences
    assert set(first_state.keys()) == set(second_state.keys()), "File structure is not deterministic!"
    
    for key in first_state:
        assert first_state[key] == second_state[key], f"Content of {key} is not deterministic!"

