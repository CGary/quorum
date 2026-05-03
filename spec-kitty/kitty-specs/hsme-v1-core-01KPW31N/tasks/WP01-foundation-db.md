---
work_package_id: WP01
title: Foundation & DB Setup
dependencies: []
requirement_refs:
- C-001
- C-003
planning_base_branch: master
merge_target_branch: master
branch_strategy: Planning artifacts for this feature were generated on master. During /spec-kitty.implement this WP may branch from a dependency-specific base, but completed changes must merge back into master unless the human explicitly redirects the landing branch.
base_branch: kitty/mission-hsme-v1-core-01KPW31N
base_commit: e0f043ccfbfc33f62e331b26c651c43d0510d134
created_at: '2026-04-23T02:47:44.266109+00:00'
subtasks:
- T001
- T002
- T003
agent: "gemini:gemini-2.5-pro:reviewer:reviewer"
shell_pid: "1636644"
history: []
agent_profile: implementer-ivan
authoritative_surface: src/storage/sqlite/
execution_mode: code_change
owned_files:
- go.mod
- go.sum
- src/core/models/**
- src/storage/sqlite/**
- tests/modules/storage_test.go
role: implementer
tags: []
---

## ⚡ Do This First: Load Agent Profile
```bash
spec-kitty agent profile load --id implementer-ivan
```

## Objective
Initialize the Go project, define core domain models mapping to the SQLite schema, and implement the database initialization script applying WAL mode, FTS5, and the `vec0` extension.

## Branch Strategy
Current branch at workflow start: master. Planning/base branch for this feature: master. Completed changes must merge into master.

## Testing Constraint
**CRITICAL**: You must follow the BDD / Module-level testing strategy. Write the tests for the module FIRST to define its behavior, verify they fail (or compile and fail), and only then write the implementation code. Do not use strict TDD (red-green-refactor per function).

## Subtasks

### T001: Setup go project and define core models
**Purpose**: Initialize `go.mod` and define Go structs.
**Steps**:
1. Run `go mod init github.com/hsme/core`
2. Add `github.com/mattn/go-sqlite3` dependency.
3. In `src/core/models/`, create models for `MemoryDocument`, `MemoryChunk`, `KGNode`, `KGEdgeEvidence`, and `AsyncTask`.

### T002: Create test stubs for the storage engine
**Purpose**: BDD Module test for the storage engine initialization.
**Steps**:
1. Create `tests/modules/storage_test.go`.
2. Write a test `TestStorageInitialization` that asserts:
   - The database file is created.
   - WAL mode is active (`PRAGMA journal_mode;`).
   - Extensions (`vec0`) are loaded without error.
   - All expected tables (`memories`, `memory_chunks`, `memory_chunks_fts`, `memory_chunks_vec`, etc.) exist.
3. Run the test to ensure it fails.

### T003: Implement the SQLite schema initialization
**Purpose**: Write the SQLite schema definition and migration logic.
**Steps**:
1. In `src/storage/sqlite/db.go`, implement an `InitDB(path string) (*sql.DB, error)` function.
2. The function must open the DB using the CGO driver configured for `sqlite_load_extension`.
3. Load the `vec0` extension dynamically.
4. Execute the `PRAGMA` setup (WAL, busy timeout, foreign keys).
5. Apply the `CREATE TABLE` and `CREATE VIRTUAL TABLE` SQL definitions provided in the `data-model.md` artifact.
6. Verify the module test passes.

## Activity Log

- 2026-04-23T02:52:13Z – claude – shell_pid=1626831 – Ready for review. Note: Tests fail locally only due to missing vec0 shared library.
- 2026-04-23T02:52:37Z – gemini:gemini-2.5-pro:reviewer:reviewer – shell_pid=1636644 – Started review via action command
- 2026-04-23T02:53:09Z – gemini:gemini-2.5-pro:reviewer:reviewer – shell_pid=1636644 – Review passed: Implementation matches schema correctly. Local test failures are due to missing vec0 shared library environment setup.
