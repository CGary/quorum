# 0008: Monorepo Merge of HSME as Quorum's Opt-in Semantic Layer

**Date:** 2026-06-09
**Status:** Proposed

## Context

Quorum's curated memory (ADR 0003) is deliberately lexical: FTS5/BM25 plus LIKE fallback, no embeddings (out of scope per ADR 0003). Consequently, semantically equivalent memories captured through different routes (`q-memory` after a task vs `q-session` from a dialog) are stored as independent rows with no detection or linkage — verified in `.ai/reports/memory-flow-dedup-analysis.yaml`. The manifesto (quorum.md, "Storage Sovereignty and Rebuildability") already permits external semantic stores as subordinate integration targets that "must not become the code truth or silently replace curated `q-memory` ingestion."

HSME (`CGary/mcp-semantic-memory`) is the user's Hybrid Semantic Memory Engine: a Go project exposing MCP tools (`store_context`, `search_fuzzy`, `search_exact`, `recall_recent_session`, `explore_knowledge_graph`) backed by sqlite-vec vector search, Ollama embeddings (`nomic-embed-text`, 768-dim, async via worker), hybrid RRF (k=60) retrieval, and an LLM-extracted knowledge graph. Both projects are Go, both keep superseded entries instead of deleting them, and both ingest only by explicit write.

The goal is one open-source project serving the developer community, where Quorum orchestrates and HSME provides optional semantic recall — including a semantic-duplicate advisor for memory capture.

Verified constraints that shape this decision:

1. **Build-mode incompatibility.** Quorum uses `modernc.org/sqlite` (pure Go, `CGO_ENABLED=0`, trivially cross-compilable). HSME requires CGO, build tags `sqlite_fts5 sqlite_vec` (mattn/go-sqlite3 + sqlite-vec C extension), and a running Ollama daemon. Importing HSME packages into the Quorum binary would impose CGO and a runtime daemon on every Quorum consumer.
2. **Corpus mismatch.** HSME's `search_fuzzy` queries HSME's own database, not Quorum's `~/.quorum/memory.db`. A dedup advisor is useless until Quorum's curated corpus is indexed in HSME; therefore data sync must precede the advisor.
3. **Divergent hashing.** Quorum's `content_hash` is SHA256 of canonical JSON; HSME's is SHA256 of NFC-normalized raw content. They are not interchangeable and must not be reconciled — each store owns its own idempotency.
4. **ID models differ.** Quorum memory IDs are strings (`DEC-YYYY-MM-DD-...`); HSME IDs are int64. Supersession edges cannot be translated without a mapping.
5. **HSME MCP tools do not enforce project isolation**; the caller must pass the `project` parameter explicitly.

## Decision

### 1. Repository

The existing `CGary/quorum` repository becomes the monorepo. HSME's full history (~310 commits) is merged via a relocate-then-merge (`git merge --allow-unrelated-histories`) into a `semantic/` subdirectory. `CGary/mcp-semantic-memory` is archived (read-only) with a final README pointing to `CGary/quorum`. No new repository is created: Quorum is the umbrella identity; HSME is its subordinate semantic component.

### 2. Two Go modules, zero compile-time coupling

- The Quorum core module stays at the repository root, unchanged (`CGO_ENABLED=0`, `modernc.org/sqlite`).
- HSME becomes the `semantic/` module (module path renamed from `github.com/hsme/core`; `src/` may be restructured to `pkg/` in the same change). It keeps CGO, its build tags, its own justfile, and its three binaries.
- A `go.work` file ties the workspace together for development only.
- **The modules never import each other.** The integration contract is data and protocol, not code: `.agents/schemas/memory.schema.json`, the documented SQLite schema of `~/.quorum/memory.db`, and the MCP tool surface.
- **CI acid test:** the core module must build and pass `go test ./...` with `CGO_ENABLED=0`, no C compiler present, and the `semantic/` directory deleted. A violation of this job is a constitutional regression, not a flaky build.

### 3. Boundaries (the three-frontier rule)

1. **Compile frontier: none.** No shared Go packages between core and semantic.
2. **Data frontier: unidirectional, semantic reads core.** A semantic-side importer (modeled on HSME's existing `cmd/migrate-legacy` full/delta pattern) reads Quorum's memory DB read-only (`?mode=ro`). It maps `project_id` to HSME's `project` column, renders each entry deterministically to `raw_content` (HSME computes its own hash, making hash divergence a non-issue), and maintains a Quorum-ID-to-HSME-ID mapping table to translate `supersedes` edges. The semantic layer never writes to Quorum's memory DB.
3. **Query frontier: MCP, consumed by skills only.** `/q-*` skills may call HSME MCP tools (e.g. `search_fuzzy` as a pre-capture semantic-duplicate advisor) and MUST pass the `project` parameter. The Quorum binary itself never invokes HSME. If HSME, its worker, or Ollama is unavailable, skills degrade gracefully and all Quorum functionality remains intact.

### 4. Authority rule

**HSME informs; Git, lifecycle artifacts, and curated `q-memory` decide.** HSME output is advisory context. It is never code truth, never a validation gate, never an ingestion path into Quorum's curated memory, and never grounds for an agent to mutate task history or erase evidence. Any prompt or client configuration describing HSME as a "primary system" must be understood as subordinate to this rule within Quorum projects.

### 5. Explicit codification of the CGO-free core

What was previously implicit is now policy: the Quorum core binary remains pure Go with no CGO and no runtime daemon dependencies. All semantic capability lives in the `semantic/` module's separate processes, opt-in.

### 6. Execution order

1. Merge HSME history into `semantic/` and rename its module (this ADR's mechanical phase).
2. Build the semantic-side delta importer for Quorum's memory DB (data frontier).
3. Add the optional semantic-duplicate advisor step to `q-memory`/`q-session` via MCP (query frontier) — only after the importer exists, because the advisor is blind without the corpus.
4. Optional, later: native near-duplicate candidate surfacing inside HSME's `StoreContext` (returns candidates to the human; never auto-collapses).

### 7. Release model

One release train, two artifact families: `quorum` (cross-compiled broadly, no CGO) and the semantic binaries (`hsme`, `hsme-worker`, `hsme-ops`; reduced platform matrix). CI runs two lanes: a fast CGO-free core lane on every PR, and a semantic lane with CGO and mocked or containerized Ollama.

## Consequences

- **Positive:** One project, one community, one ADR series. Semantic dedup detection becomes possible against the real curated corpus. Quorum's portability and determinism are untouched and now explicitly protected by CI. HSME gains distribution and a concrete consumer. Full git history of both projects is preserved.
- **Negative:** Repository build instructions become dual-mode (core vs semantic prerequisites). The importer introduces an eventual-consistency window (async embeddings via `hsme-worker`) during which new memories are not yet vector-searchable; the advisor must tolerate this. Maintaining the ID mapping table adds a small amount of state to the semantic side.
- **Deferred:** Automatic supersedence inference, semantic auto-deduplication, and any auto-capture path remain rejected (manifesto; HSME's own V1 non-goals). Reverse data flow (HSME → curated memory) remains prohibited.
