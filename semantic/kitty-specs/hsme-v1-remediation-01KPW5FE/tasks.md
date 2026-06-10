# Work Packages: HSME V1 Remediation

**Mission**: `hsme-v1-remediation-01KPW5FE`
**Branch**: `hsme-v1-remediation-01KPW5FE`

## Subtask Index
| ID | Description | WP | Parallel |
|---|---|---|---|
| T001 | Remove implicit FTS5 triggers from SQLite schema | WP01 | | [D] | [D] |
| T002 | Verify `ingest.go` explicitly syncs FTS5 | WP01 | | [D] |
| T003 | Create `src/core/worker` module and implement leasing loop | WP02 | [D] |
| T004 | Create `src/core/search` module and implement RRF | WP02 | [D] |
| T005 | Ensure `tests/modules/worker_test.go` and `search_test.go` compile and pass | WP02 | [D] |
| T006 | Create `cmd/server/main.go` MCP stdio entry point | WP03 | | [D] |
| T007 | Register all 4 MCP tools mapping to core modules | WP03 | | [D] |

## WP01: Fix DB Initialization
**Goal**: Remove the SQLite triggers that violate the architecture decision for explicit FTS5 synchronization.
**Prompt**: `tasks/WP01-fix-db.md` (~150 lines)
**Dependencies**: None
**Included Subtasks**:
- [x] T001 Remove implicit FTS5 triggers from SQLite schema (WP01)
- [x] T002 Verify `ingest.go` explicitly syncs FTS5 (WP01)

## WP02: Implement Missing Modules
**Goal**: Actually implement the `worker` and `search` packages that were stubbed in the previous mission, ensuring the tests compile and run.
**Prompt**: `tasks/WP02-implement-modules.md` (~250 lines)
**Dependencies**: None
**Parallel Opportunities**: Can run alongside WP01.
**Included Subtasks**:
- [x] T003 Create `src/core/worker` module and implement leasing loop (WP02)
- [x] T004 Create `src/core/search` module and implement RRF (WP02)
- [x] T005 Ensure `tests/modules/worker_test.go` and `search_test.go` compile and pass (WP02)

## WP03: MCP Transport Layer
**Goal**: Implement the missing `mcp` package and the main entry point to expose the tools.
**Prompt**: `tasks/WP03-mcp-layer.md` (~150 lines)
**Dependencies**: WP02
**Included Subtasks**:
- [x] T006 Create `cmd/server/main.go` MCP stdio entry point (WP03)
- [x] T007 Register all 4 MCP tools mapping to core modules (WP03)
