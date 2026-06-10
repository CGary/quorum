# Implementation Plan: HSME V1 Remediation

**Branch**: `hsme-v1-remediation-01KPW5FE` | **Date**: 2026-04-23 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/kitty-specs/hsme-v1-remediation-01KPW5FE/spec.md`

## Summary
Fix critical drift and missing implementation from V1 Core: Address the missing Worker, Search, and MCP layers, remove SQLite FTS5 triggers as per spec, and ensure module tests compile.

## Technical Context

**Language/Version**: Go 1.22+ (requires CGO)
**Primary Dependencies**: `github.com/mattn/go-sqlite3`, `vec0` (dynamic extension)
**Storage**: SQLite (WAL mode enforced)
**Testing**: BDD / Module-level tests (No strict TDD). Ensure compilation errors in existing tests are resolved by implementing the missing packages.
**Target Platform**: Linux (Debian slim / Ubuntu minimal for glibc compatibility with `vec0`)
**Project Type**: single/cli (MCP server via stdio)
**Constraints**: Explicit FTS5 sync must be used instead of SQLite triggers.

## Charter Check
*Skipped: No charter file found.*

## Project Structure

### Documentation (this feature)
```
kitty-specs/hsme-v1-remediation-01KPW5FE/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
└── tasks.md             # Phase 2 output
```

### Source Code (repository root)
```
src/
├── core/
│   ├── indexer/       # (Already implemented, check for implicit triggers dependency)
│   ├── worker/        # MISSING: Async leasing, embedding, graph extraction
│   ├── search/        # MISSING: RRF logic, FTS5 + vec0 querying
│   └── models/        # (Already implemented)
├── storage/
│   └── sqlite/        # FIX: DB initialization, remove triggers
├── mcp/
│   ├── handlers/      # MISSING: store_context, search_fuzzy, trace_dependencies
│   └── server/        # MISSING: Stdio MCP server lifecycle
└── tests/
    └── modules/       # FIX: Compile errors due to missing worker package
```

**Structure Decision**: Continue with the existing Go standard layout, focusing exclusively on creating the missing directories and implementing their logic, while fixing the initialization in `storage`.
