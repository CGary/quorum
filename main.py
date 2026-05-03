import os
import sys

# Ensure .agents is in the python path
PROJECT_ROOT = os.path.dirname(os.path.abspath(__file__))
AGENTS_DIR = os.path.join(PROJECT_ROOT, ".agents")
if AGENTS_DIR not in sys.path:
    sys.path.insert(0, AGENTS_DIR)

from cli.main import main

if __name__ == "__main__":
    main()
