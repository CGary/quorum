# Specification: HSME V1 Remediation

**Mission ID**: 01KPW5FEZJV2Z46CT8Q2AEB50V
**Type**: software-dev

## Purpose
**TLDR**: Fix critical drift and missing implementation from V1 Core
**Context**: Address the missing Worker, Search, and MCP layers, remove SQLite FTS5 triggers as per spec, and ensure module tests compile. This remediates the findings from the post-merge review of `hsme-v1-core`.

## Success Criteria
- The `src/storage/sqlite/db.go` file initializes the schema without implicit FTS5 triggers.
- The `worker` module is fully implemented and passes its tests.
- The `search` module (RRF and `trace_dependencies`) is fully implemented.
- The `mcp` transport layer correctly exposes the tools and acts as the main entry point.
- All tests in `tests/modules/` compile successfully.

## Key Entities
- **Memory Document**: The original text provided by the client.
- **Memory Chunk**: Segments of the memory document.
- **Async Task**: A queued background job (embed or graph_extract).

## Assumptions
- The test suite compilation failure is due to missing packages (`src/core/worker` etc.).
- BDD testing constraints still apply: Write tests for any new modules before implementation.

## User Scenarios & Testing

**Scenario 1: Database Initialization without Triggers**
- **Actor**: System
- **Trigger**: System starts.
- **Action**: Initializes DB schema.
- **Outcome**: `memory_chunks_fts` exists, but `memory_chunks_ai`, `memory_chunks_au`, `memory_chunks_ad` triggers do NOT exist.

**Scenario 2: Worker Processing**
- **Actor**: Background Worker
- **Trigger**: Tasks exist in `async_tasks` in `pending` state.
- **Action**: Worker leases task, executes it via interfaces, and marks it `completed`.
- **Outcome**: Vector table or Knowledge Graph tables are updated asynchronously.

## Requirements

### Functional Requirements

| ID | Description | Status |
|----|-------------|--------|
| FR-001 | Remove SQLite FTS5 implicit triggers from DB initialization. | Draft |
| FR-002 | Implement the polling worker logic with leasing mechanism. | Draft |
| FR-003 | Implement the `search_fuzzy` MCP tool combining FTS5 and vector search using Reciprocal Rank Fusion. | Draft |
| FR-004 | Implement the `search_exact` MCP tool. | Draft |
| FR-005 | Implement the `trace_dependencies` MCP tool using recursive CTE. | Draft |
| FR-006 | Implement the MCP stdio transport layer exposing all tools. | Draft |
| FR-007 | Fix compilation errors in `tests/modules/worker_test.go` by creating the required package structure. | Draft |

### Non-Functional Requirements

| ID | Description | Status |
|----|-------------|--------|
| NFR-001 | Testing Strategy: Module-level/behavior-driven tests must be written and validated before the underlying implementation. | Draft |

### Constraints

| ID | Description | Status |
|----|-------------|--------|
| C-001 | FTS5 synchronization must remain explicit in application code (`src/core/indexer/ingest.go`), never via triggers. | Draft |
