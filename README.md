# Quorum ⚖️

**Autonomous Agent Orchestration with Strict Contracts and Traceability.**

Quorum is a framework designed to manage AI agents not as "general assistants" but as focused mission-execution units. It enforces strict contracts, deterministic context retrieval, and human-in-the-loop gates to ensure safety, predictability, and cost-efficiency.

## 🧠 Philosophy

- **Agents don't get "the project"**: They receive a bounded mission, a strict contract, and deterministically retrieved context.
- **The Human Leads**: Humans decide direction, risk, and final merges.
- **The Code Validates**: The system handles validations, routing, worktrees, costs, and retries.
- **Traceability by Default**: Every attempt, token, and cent is recorded in a `07-trace.json`.

## 🛠 Project Structure

- `.agents/`: Core logic, prompts, schemas, and policies.
- `.ai/tasks/`: Task lifecycle (inbox, active, done, failed).
- `memory/`: Selective semantic memory (decisions, patterns, lessons).
- `worktrees/`: Isolated sandboxes for agent execution.

## 🚀 Getting Started

### Prerequisites

- [uv](https://github.com/astral-sh/uv) (Python package manager)
- Git

### Installation

```bash
git clone https://github.com/your-username/quorum.git
cd quorum
chmod +x agents
```

### Basic Workflow

1. **Create a task**: Move a contract to `.ai/tasks/inbox/<task-id>-<slug>/01-contract.yaml`.
2. **Start the task**:
   ```bash
   ./agents task start FEAT-001
   ```
   This validates the contract, creates a Git worktree, and initializes traceability.
3. **Check status**:
   ```bash
   ./agents task status FEAT-001
   ```
4. **Finish and Clean**:
   ```bash
   ./agents task clean FEAT-001
   ```

## ⚖️ License

MIT
