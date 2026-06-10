# Implementation Plan: HSME V1 Core

**Branch**: `hsme-v1-core-01KPW31N` | **Date**: 2026-04-23 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/kitty-specs/hsme-v1-core-01KPW31N/spec.md`

## Summary
Implement V1 of the Hybrid Semantic Memory Engine (HSME), a local MCP server that stores, chunks, and hybrid-searches technical context using SQLite, FTS5, and the `vec0` vector extension. It runs an in-process async worker to enrich ingested documents with embeddings and knowledge graph relations.

## Technical Context

**Language/Version**: Go 1.22+ (requires CGO)
**Primary Dependencies**: `github.com/mattn/go-sqlite3`, `vec0` (dynamic extension)
**Storage**: SQLite (WAL mode enforced)
**Testing**: BDD / Module-level tests (No strict TDD). Tests validate business blocks (ingestion, RRF search, worker leasing) before underlying code implementation.
**Target Platform**: Linux (Debian slim / Ubuntu minimal for glibc compatibility with `vec0`)
**Project Type**: single/cli (MCP server via stdio)
**Performance Goals**: `< 1 second` for `store_context` (fast ingestion path)
**Constraints**: Dynamic extension loading requires CGO; pure Go SQLite drivers cannot be used.
**Scale/Scope**: Local agent persistent memory storage.

## Charter Check
*Skipped: No charter file found.*

## Project Structure

### Documentation (this feature)
```
kitty-specs/hsme-v1-core-01KPW31N/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
└── tasks.md             # Phase 2 output (generated via spec-kitty.tasks)
```

### Source Code (repository root)
```
src/
├── core/
│   ├── indexer/       # Chunking, Hash, Deduplication
│   ├── worker/        # Async leasing, embedding, graph extraction
│   ├── search/        # RRF logic, FTS5 + vec0 querying
│   └── models/        # Go structs corresponding to SQLite schema
├── storage/
│   └── sqlite/        # DB initialization, migrations, queries
├── mcp/
│   ├── handlers/      # store_context, search_fuzzy, trace_dependencies
│   └── server/        # Stdio MCP server lifecycle
└── tests/
    └── modules/       # BDD / block-level tests
```

**Structure Decision**: Go standard layout adapted for a single MCP server application with clear boundaries between the MCP transport layer, business logic (core), and persistence (storage).

## Complexity Tracking
*N/A - Charter check skipped.*
