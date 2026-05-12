import os
import sys

PROJECT_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
AGENTS_DIR = os.path.join(PROJECT_ROOT, ".agents")
if AGENTS_DIR not in sys.path:
    sys.path.insert(0, AGENTS_DIR)

from cli.core.decomposition_render import render_ascii_dag


def test_render_ascii_dag_empty_input_is_empty():
    assert render_ascii_dag([]) == ""


def test_render_ascii_dag_trivial_parallel_children():
    decomposition = [
        {"child_id": "FEAT-001-c", "summary": "Third slice."},
        {"child_id": "FEAT-001-a", "summary": "First slice."},
        {"child_id": "FEAT-001-b", "summary": "Second slice."},
    ]

    assert render_ascii_dag(decomposition) == """Decomposition DAG:
  order: L0
  L0
  [FEAT-001-a]
  [FEAT-001-b]
  [FEAT-001-c]
  edges:
    (none)"""


def test_render_ascii_dag_linear_children():
    decomposition = [
        {"child_id": "FEAT-001-a", "summary": "First slice."},
        {"child_id": "FEAT-001-b", "summary": "Second slice.", "depends_on": ["FEAT-001-a"]},
        {"child_id": "FEAT-001-c", "summary": "Third slice.", "depends_on": ["FEAT-001-b"]},
    ]

    assert render_ascii_dag(decomposition) == """Decomposition DAG:
  order: L0 -> L1 -> L2
  L0            L1            L2
  [FEAT-001-a]  [FEAT-001-b]  [FEAT-001-c]
  edges:
    FEAT-001-a -> FEAT-001-b
    FEAT-001-b -> FEAT-001-c"""


def test_render_ascii_dag_diamond_children():
    decomposition = [
        {"child_id": "FEAT-001-d", "summary": "Final slice.", "depends_on": ["FEAT-001-b", "FEAT-001-c"]},
        {"child_id": "FEAT-001-c", "summary": "Right slice.", "depends_on": ["FEAT-001-a"]},
        {"child_id": "FEAT-001-b", "summary": "Left slice.", "depends_on": ["FEAT-001-a"]},
        {"child_id": "FEAT-001-a", "summary": "Initial slice."},
    ]

    assert render_ascii_dag(decomposition) == """Decomposition DAG:
  order: L0 -> L1 -> L2
  L0            L1            L2
  [FEAT-001-a]  [FEAT-001-b]  [FEAT-001-d]
                [FEAT-001-c]
  edges:
    FEAT-001-a -> FEAT-001-b
    FEAT-001-a -> FEAT-001-c
    FEAT-001-b -> FEAT-001-d
    FEAT-001-c -> FEAT-001-d"""
