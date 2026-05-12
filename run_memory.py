import json
import sys
from pathlib import Path

# Buscamos el próximo ID para DEC
today = "2026-05-12"
dec_dir = Path("memory/lessons")
existing = list(dec_dir.glob(f"LES-{today}-*.json"))
next_id = len(existing) + 1

lesson_id = f"LES-{today}-{next_id}"
lesson = {
  "id": lesson_id,
  "source_task": "FEAT-005-j",
  "type": "lesson",
  "title": "Documentation structure for dual workflows",
  "context": "The README was rewritten to explain both single-task and decomposed workflows.",
  "content": "When a framework supports both single-pass execution and parent-child decomposition, the Quick Start documentation should walk through the complex (decomposed) path as the primary tutorial, because it subsumes the single-pass mechanics while introducing necessary cross-task concepts like dependency waiting and umbrella cleanup.",
  "related": ["FEAT-005"],
  "created_at": today
}

with open(dec_dir / f"{lesson_id}.json", "w") as f:
    json.dump(lesson, f, indent=2)

print(f"Created {lesson_id}")
