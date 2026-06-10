# Research: HSME V1 Core

## Decision 1: Hybrid Retrieval Strategy
- **Decision**: Reciprocal Rank Fusion (RRF) at the chunk level.
- **Rationale**: The specification explicitly mandates RRF, grouping the ranked chunks by document. This ensures robust retrieval even when embeddings are still processing and FTS5 is the only available index.
- **Alternatives considered**: Linear combination of normalized scores (rejected due to score distribution mismatch between FTS5 and cosine similarity).

## Decision 2: Asynchronous Enrichment
- **Decision**: In-process background worker with lease polling.
- **Rationale**: MCP servers live only while the client is connected. An external long-running worker is out of scope for V1. The worker must process `embed` and `graph_extract` tasks from `async_tasks` without blocking the fast SQLite ingestion path.
- **Alternatives considered**: External sidecar container (deferred to V2/future).
