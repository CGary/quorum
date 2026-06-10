# Work Packages: HSME V1 Core

**Mission**: `hsme-v1-core-01KPW31N`
**Branch**: `hsme-v1-core-01KPW31N`

## Subtask Index
| ID | Description | WP | Parallel |
|---|---|---|---|
| T001 | Setup go project and define core models | WP01 | | [D] |
| T002 | Create test stubs for the storage engine (BDD tests first) | WP01 | | [D] |
| T003 | Implement the SQLite schema initialization (WAL, vec0, fts5) | WP01 | | [D] |
| T004 | Create chunker and deduplication block tests | WP02 | | [D] |
| T005 | Implement content hashing and chunking logic | WP02 | | [D] |
| T006 | Implement `store_context` ingestion logic | WP02 | | [D] |
| T007 | Enqueue async tasks on ingestion | WP02 | | [D] |
| T008 | Create worker block tests (leasing, retry counts) | WP03 | [D] |
| T009 | Define Embedder and GraphExtractor interfaces | WP03 | [D] |
| T010 | Implement polling worker logic | WP03 | [D] |
| T011 | Implement `embed` and `graph_extract` execution | WP03 | [D] |
| T012 | Create search block tests (RRF scoring logic) | WP04 | [D] |
| T013 | Implement Reciprocal Rank Fusion (FTS5 + Vec0) | WP04 | [D] |
| T014 | Create graph traversal block tests | WP04 | [D] |
| T015 | Implement `trace_dependencies` recursive CTE traversal logic | WP04 | [D] |
| T016 | Setup stdio MCP server skeleton | WP05 | | [D] |
| T017 | Register `store_context` and `search_fuzzy` handlers | WP05 | | [D] |
| T018 | Register `search_exact` and `trace_dependencies` handlers | WP05 | | [D] |

## WP01: Foundation & DB Setup
**Goal**: Initialize the Go project, define core entities, and implement the SQLite storage engine initialization with its schema.
**Prompt**: `tasks/WP01-foundation-db.md` (~250 lines)
**Dependencies**: None
**Included Subtasks**:
- [x] T001 Setup go project and define core models (WP01)
- [x] T002 Create test stubs for the storage engine (BDD tests first) (WP01)
- [x] T003 Implement the SQLite schema initialization (WAL, vec0, fts5) (WP01)

## WP02: Indexer Core
**Goal**: Implement the synchronous ingestion path (`store_context` logic) including hashing, deduplication, chunking, and FTS5 synchronization.
**Prompt**: `tasks/WP02-indexer-core.md` (~300 lines)
**Dependencies**: WP01
**Included Subtasks**:
- [x] T004 Create chunker and deduplication block tests (WP02)
- [x] T005 Implement content hashing and chunking logic (WP02)
- [x] T006 Implement `store_context` ingestion logic (WP02)
- [x] T007 Enqueue async tasks on ingestion (WP02)

## WP03: Async Worker & Interfaces
**Goal**: Develop the background worker responsible for polling `async_tasks` using leasing, and the interfaces for inference logic.
**Prompt**: `tasks/WP03-async-worker.md` (~300 lines)
**Dependencies**: WP01
**Parallel Opportunities**: Can be worked on parallel to WP02 (depends only on DB schema).
**Included Subtasks**:
- [x] T008 Create worker block tests (leasing, retry counts) (WP03)
- [x] T009 Define Embedder and GraphExtractor interfaces (WP03)
- [x] T010 Implement polling worker logic (WP03)
- [x] T011 Implement `embed` and `graph_extract` execution (WP03)

## WP04: Search & Graph Traversal
**Goal**: Implement the hybrid semantic search using Reciprocal Rank Fusion (RRF) and the recursive graph traversal for dependencies.
**Prompt**: `tasks/WP04-search-traversal.md` (~300 lines)
**Dependencies**: WP01, WP02
**Included Subtasks**:
- [x] T012 Create search block tests (RRF scoring logic) (WP04)
- [x] T013 Implement Reciprocal Rank Fusion (FTS5 + Vec0) (WP04)
- [x] T014 Create graph traversal block tests (WP04)
- [x] T015 Implement `trace_dependencies` recursive CTE traversal logic (WP04)

## WP05: MCP Transport Layer
**Goal**: Wrap the core logic into an MCP stdio server and expose the configured tools.
**Prompt**: `tasks/WP05-mcp-layer.md` (~200 lines)
**Dependencies**: WP02, WP03, WP04
**Included Subtasks**:
- [x] T016 Setup stdio MCP server skeleton (WP05)
- [x] T017 Register `store_context` and `search_fuzzy` handlers (WP05)
- [x] T018 Register `search_exact` and `trace_dependencies` handlers (WP05)
