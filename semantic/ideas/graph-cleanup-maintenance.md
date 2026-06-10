# Idea: Knowledge Graph Maintenance Job

## The Problem
As the agent stores context, it may produce "dirty" nodes (e.g., "the service", "fix", "bug") or duplicate entities with slightly different names ("AuthService" vs "Auth-Service"). This dilutes the graph's power.

## Proposed Solution: The "Janitor" Job
A periodic asynchronous process (or a specific MCP tool `optimize_graph`) that performs the following:

1. **Entity Merging**:
   - Use a high-capability LLM to identify semantically identical nodes.
   - Update `kg_edge_evidence` to point to a single "canonical" node ID and prune the duplicates.

2. **Node Pruning**:
   - Identify nodes with low centrality (few edges) and generic names.
   - Flag them for review or automatic deletion if they provide no architectural value.

3. **Relation Validation**:
   - Check if the relations (`DEPENDS_ON`, `CAUSES`) still make sense across different memories.
   - Detect "Conflicting Truths" (e.g., Memory A says X depends on Y, Memory B says the opposite).

4. **Normalization Reinforcement**:
   - Feedback loop: provide the agent with a list of "preferred entity names" for future store operations.
