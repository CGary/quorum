# 0003: Memoria Centralizada Multi-Proyecto con SQLite

**Date:** 2026-05-30
**Status:** Accepted

## Context

Quorum's original memory system relied on local `memory/*.json` files stored within each project's repository. While this ensured that knowledge lived alongside the codebase, it led to fragmented operational knowledge across multiple projects and made programmatic retrieval (for `q-blueprint` and `q-analyze`) inefficient without an overarching index.

The need has arisen to evolve Quorum into a multi-project orchestrator where patterns, decisions, and lessons can be queried centrally.

## Decision

We will transition the operational source of truth for durable memory to a centralized SQLite database managed by Quorum.

1. **Git remains the absolute source of truth for code.** SQLite becomes the operational source of truth for durable memory only.
2. **Curation remains human-driven.** The `q-memory` skill will persist data via `quorum memory save`. There is no automatic background ingestion.
3. **Migration via `quorum init`.** Legacy `memory/*.json` files will be automatically migrated to the central SQLite database during `quorum init`. They will only be deleted from the local project after a successful schema validation and transaction commit.
4. **Project Configuration.** Projects will declare their identity in a `.quorumrc` file (containing `project_id` and `project_name`). No absolute local filesystem paths or database paths will be stored in project configuration files. The SQLite database location will be resolved dynamically (e.g., via `QUORUM_MEMORY_DB` or a default `~/.quorum/memory.db`).
5. **RAG / Embeddings.** RAG, semantic vector search, and embeddings are explicitly out of scope for the initial implementation.
6. **Future Export.** A `quorum memory export` command will remain a future capability for backup, audit, and portability, ensuring SQLite does not become an opaque black box.

## Consequences

- **Positive:** Centralized, queryable knowledge base across all Quorum projects. Enables future programmatic retrieval features (e.g., `quorum memory search`).
- **Negative:** Minor loss of raw file portability. Mitigated by the future addition of `quorum memory export`.
