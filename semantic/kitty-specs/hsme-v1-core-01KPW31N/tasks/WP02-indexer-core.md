---
work_package_id: WP02
title: Indexer Core
dependencies:
- WP01
requirement_refs:
- FR-001
- FR-002
- FR-003
- FR-004
planning_base_branch: master
merge_target_branch: master
branch_strategy: Planning artifacts for this feature were generated on master. During /spec-kitty.implement this WP may branch from a dependency-specific base, but completed changes must merge back into master unless the human explicitly redirects the landing branch.
subtasks:
- T004
- T005
- T006
- T007
agent: "gemini:gemini-2.5-pro:reviewer:reviewer"
history: []
agent_profile: implementer-ivan
authoritative_surface: src/core/indexer/
execution_mode: code_change
owned_files:
- src/core/indexer/**
- tests/modules/indexer_test.go
role: implementer
tags: []
shell_pid: "1643931"
---

## ⚡ Do This First: Load Agent Profile
```bash
spec-kitty agent profile load --id implementer-ivan
```

## Objective
Implement the synchronous ingestion path (`store_context`). This includes hashing content for deduplication, splitting it into indexable chunks, and populating the FTS5 index and asynchronous task queue.

## Branch Strategy
Current branch at workflow start: master. Planning/base branch for this feature: master. Completed changes must merge into master.

## Testing Constraint
**CRITICAL**: You must follow the BDD / Module-level testing strategy. Write the tests for the module FIRST to define its behavior, verify they fail, and only then write the implementation code.

## Subtasks

### T004: Create chunker and deduplication block tests
**Purpose**: Write tests validating the business logic of ingestion.
**Steps**:
1. Create `tests/modules/indexer_test.go`.
2. Test `TestContentHashing`: Assert SHA-256 output (lowercase hex) and NFC normalization.
3. Test `TestChunking`: Assert recursive splitting on `\n\n`, `\n`, space targeting 400-800 tokens.
4. Test `TestStoreContext`: Assert deduplication logic, FTS5 explicit sync, and task enqueueing.

### T005: Implement content hashing and chunking logic
**Purpose**: Implement the pure functions for content manipulation.
**Steps**:
1. In `src/core/indexer/hash.go`, implement `ComputeHash(content string) string` matching the SHA-256 and NFC rules.
2. In `src/core/indexer/chunker.go`, implement `Split(content string, sourceType string) []string`.

### T006: Implement `store_context` ingestion logic
**Purpose**: Implement the database transaction inserting a memory document.
**Steps**:
1. In `src/core/indexer/ingest.go`, write `StoreContext`.
2. Check `content_hash` against `memories` table for deduplication. Return existing ID if duplicate and `force_reingest` is false.
3. Insert into `memories`.
4. Insert chunks into `memory_chunks`.
5. Explicitly sync `memory_chunks_fts` with `INSERT INTO memory_chunks_fts(rowid, chunk_text) VALUES (...)`.

### T007: Enqueue async tasks on ingestion
**Purpose**: Wire the background tasks into the ingestion transaction.
**Steps**:
1. Inside the `StoreContext` transaction, insert two rows into `async_tasks`: one for `task_type = 'embed'` and one for `task_type = 'graph_extract'`.
2. Commit the transaction and verify all module tests pass.

## Activity Log

- 2026-04-23T02:53:33Z – gemini:gemini-2.5-pro:implementer-ivan:implementer – shell_pid=1638401 – Started implementation via action command
- 2026-04-23T02:56:23Z – gemini:gemini-2.5-pro:implementer-ivan:implementer – shell_pid=1638401 – Ready for review. Local tests fail due to vec0.
- 2026-04-23T02:56:39Z – gemini:gemini-2.5-pro:reviewer:reviewer – shell_pid=1643931 – Started review via action command
- 2026-04-23T02:57:21Z – gemini:gemini-2.5-pro:reviewer:reviewer – shell_pid=1643931 – Review passed: Hashing, chunking, and store_context transaction correctly implemented.
