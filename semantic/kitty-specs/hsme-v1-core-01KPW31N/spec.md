# Specification: HSME V1 Core

**Mission ID**: 01KPW31NAG8EFZ8ZJ73NRC9J0P
**Type**: software-dev

## Purpose
**TLDR**: Implement V1 of the Hybrid Semantic Memory Engine
**Context**: Develop the local MCP server with SQLite, FTS5, and vector search, following the Technical_Specification.md design.

## Success Criteria
- The system successfully initializes an SQLite database with WAL mode and necessary extensions (`vec0`).
- Users can insert a document via `store_context` and receive an immediate `memory_id` within < 1 second.
- Lexical and semantic search (`search_fuzzy`) correctly blends results using Reciprocal Rank Fusion (RRF).
- Background workers automatically process embeddings and extract graph relationships.
- The `trace_dependencies` tool correctly returns upstream or downstream connections up to the specified depth.

## Key Entities
- **Memory Document**: The original text provided by the client.
- **Memory Chunk**: Segments of the memory document (400-800 tokens) used for vector indexing.
- **Knowledge Graph Node**: A technical entity (TECH, ERROR, FILE, CMD).
- **Knowledge Graph Edge**: The evidence of a relationship between two nodes derived from a specific memory.
- **Async Task**: A queued background job (embed or graph_extract).

## Assumptions
- The server will run in a Docker container (Debian slim or Ubuntu minimal) that supports CGO and dynamic loading of the `vec0` extension.
- The embedding model configuration is static per database.
- TDD is not used; tests must cover module-level business blocks (use cases) prior to their implementation.

## User Scenarios & Testing

**Scenario 1: Storing a new technical context**
- **Actor**: AI Agent (MCP Client)
- **Trigger**: The agent sends technical notes via `store_context`.
- **Action**: The system hashes the content, deduplicates if necessary, splits the content into chunks, updates the FTS5 index, enqueues async tasks, and returns the ID.
- **Outcome**: The agent receives a success response and the memory ID immediately.
- **Exception**: If the input is invalid or the database is locked, an `INVALID_INPUT` or `INTERNAL` error is returned.

**Scenario 2: Fuzzy Search**
- **Actor**: AI Agent (MCP Client)
- **Trigger**: The agent invokes `search_fuzzy` with a query string.
- **Action**: The system queries both FTS5 and the vector index, applies RRF on chunks, groups by memory document, applies obsolescence penalty, and returns the top matches.
- **Outcome**: A ranked list of results is returned, indicating if `vector_coverage` is complete.

## Requirements

### Functional Requirements

| ID | Description | Status |
|----|-------------|--------|
| FR-001 | The system must expose the `store_context` MCP tool to save memory documents. | Draft |
| FR-002 | The system must implement content hashing (SHA-256) for deduplication on insertion. | Draft |
| FR-003 | The system must split documents into indexable chunks using predefined separators (`\n\n`, `\n`, space). | Draft |
| FR-004 | The system must enqueue `embed` and `graph_extract` tasks asynchronously upon ingestion. | Draft |
| FR-005 | The system must expose the `search_fuzzy` MCP tool combining FTS5 and vector search using Reciprocal Rank Fusion. | Draft |
| FR-006 | The system must expose the `search_exact` MCP tool for exact keyword matching on chunks. | Draft |
| FR-007 | The system must expose the `trace_dependencies` MCP tool for navigating the knowledge graph. | Draft |
| FR-008 | The worker process must poll `async_tasks` using a leasing mechanism to execute pending jobs. | Draft |

### Non-Functional Requirements

| ID | Description | Status |
|----|-------------|--------|
| NFR-001 | The `store_context` tool must return a response in < 1 second for text that does not require chunking. | Draft |
| NFR-002 | The asynchronous worker must implement a lease timeout (default 5 minutes) and a maximum retry count (default 5). | Draft |
| NFR-003 | Testing Strategy: Module-level/behavior-driven tests must be written and validated before the underlying implementation (No strict TDD). | Draft |

### Constraints

| ID | Description | Status |
|----|-------------|--------|
| C-001 | The system must use a CGO-backed SQLite driver (`github.com/mattn/go-sqlite3`) to load the `vec0` extension dynamically. | Draft |
| C-002 | Storage and inference logic must be explicitly decoupled via interfaces. | Draft |
| C-003 | Any FTS5 or `vec0` virtual table updates/deletions must happen in the same transaction as the `memory_chunks` table mutation. | Draft |
