# Research: HSME V1 Remediation

## Decision 1: Explicit FTS5 Synchronization
- **Decision**: Remove `memory_chunks_ai`, `memory_chunks_au`, and `memory_chunks_ad` triggers from SQLite initialization schema.
- **Rationale**: The technical specification (Section 10.2) explicitly mandates manual insertion into `memory_chunks_fts` via the application layer during the same transaction to maintain concurrency control and prevent opaque deadlocks. The initial implementation mistakenly used SQLite triggers.

## Decision 2: Implementation of Missing Modules
- **Decision**: Fully implement `src/core/worker`, `src/core/search`, and `cmd/server/main.go`.
- **Rationale**: The previous mission merged without these components being written, resulting in a system that could only ingest data but not process embeddings, search them, or expose them via MCP.
