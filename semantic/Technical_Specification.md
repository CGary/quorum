# Technical Specification: Hybrid Semantic Memory Engine

## 1. Objective

Design a hybrid semantic memory engine for a local MCP server, capable of:

* persistently storing technical context;
* retrieving information through lexical matching, semantic similarity, and causal traceability;
* enriching context in the background via structured extraction of entities and relations;
* operating locally with SQLite and decoupled inference models.

---

## 2. Design Principles

1. **The primary logical unit is the memory document**, not the chunk.
2. **Vector search operates over chunks**, but final retrieval must reconstruct the source document.
3. **Storage and inference are decoupled** through interfaces.
4. **Persistence must survive container restarts**.
5. **Semantic enrichment (embeddings and graph extraction) is asynchronous and fault-tolerant**. Only the minimum required for lexical retrieval is synchronous.
6. **A database is bound to a fixed embedding configuration**; embedding dimensionality must not be mixed within the same index.
7. **MCP tools must have explicit input and output contracts**.
8. **Schema evolution is explicit**; the schema version is tracked in `system_config`.

---

## 3. Deployment Architecture

### 3.1 Execution model

The system runs inside a Docker container and communicates exclusively over `stdin/stdout` with the CLI agent, following the local MCP server pattern.

* No ports are exposed.
* Persistence is provided by a volume mounted from the host.
* The SQLite database lives at `/app/data/engram.db`.

### 3.2 Process lifecycle and the async worker

Because MCP stdio servers live only while a client is connected, V1 adopts the following model:

* The **asynchronous worker runs in-process** as a goroutine within the same MCP server process.
* On startup, the worker drains any tasks left in `pending` state or whose `leased_until` has expired.
* When the client disconnects, the process exits and pending tasks remain queued. They are resumed on the next session.
* A dedicated long-running worker (separate container sharing the same volume) is **out of scope for V1** and explicitly deferred; if adopted later it must reuse the same leasing semantics.

This trade-off is accepted because users of a local MCP server expect work to occur while the assistant is active. It must be documented in user-facing notes.

### 3.3 Runtime compatibility

Because the system requires CGO and dynamic loading of the `vec0` SQLite extension, builder and runner must maintain ABI compatibility.

**Decision:** use a homogeneous Linux family between build and runtime.

#### Operational recommendation

* **Builder:** Debian slim or Ubuntu minimal.
* **Runner:** Debian slim, or a `glibc`-compatible distroless image.
* **Do not use Alpine** for the runner if `vec0` was compiled against `glibc`.

This avoids `musl` vs `glibc` incompatibilities when loading the SQLite extension.

#### Go driver constraint

Dynamic extension loading forces the implementation to use a CGO-backed SQLite driver. Specifically:

* Use `github.com/mattn/go-sqlite3` (or an equivalent CGO-backed driver) compiled with the `sqlite_load_extension` build tag.
* Pure-Go drivers such as `modernc.org/sqlite` **cannot load dynamic extensions** and must not be used.

### 3.4 Docker Compose baseline

```yaml
services:
  mcp-core:
    build: .
    volumes:
      - ./data:/app/data
    environment:
      - SQLITE_DB_PATH=/app/data/engram.db
      - SQLITE_VEC_PATH=/usr/local/lib/vec0.so
      - AI_PROVIDER=ollama
      - EMBEDDING_MODEL=nomic-embed-text
      - EMBEDDING_DIM=768
      - LLM_ENDPOINT=http://host.docker.internal:11434
    extra_hosts:
      - "host.docker.internal:host-gateway"
```

Notes:

* The Compose file is provided as a reference for local development and for running the optional sidecar worker in the future. For a stdio MCP server the agent typically invokes the container via `docker run -i`, which is also supported with the same image.
* `host-gateway` requires Docker 20.10+. In rootless Docker, Podman, or setups where Ollama runs on a different machine, set `LLM_ENDPOINT` to an explicit URL rather than relying on `host.docker.internal`.

### 3.5 Embedding compatibility constraint

A SQLite database initialized with a given embedding dimensionality cannot be reused with a different dimensionality.

**Rule:** if `EMBEDDING_DIM` or `EMBEDDING_MODEL` changes, either a full migration/reindexing must be executed, or a new database must be created.

### 3.6 Backup and restore

Because SQLite is configured in WAL mode (see §15), a naive `cp engram.db` is not safe. Backups must use one of:

* `sqlite3 engram.db ".backup /path/to/backup.db"`,
* `VACUUM INTO '/path/to/backup.db'`, or
* stopping the process before copying the `.db`, `.db-wal`, and `.db-shm` files together.

---

## 4. Inference Abstraction

Inference logic must be decoupled from persistence.

```go
type Embedder interface {
    GenerateVector(text string) ([]float32, error)
    ModelID() string
    Dimension() int
}

type GraphExtractor interface {
    ExtractEntities(text string) (KnowledgeGraphJSON, error)
    ModelID() string
}
```

### 4.1 Purpose of the interfaces

* Allow swapping providers (`ollama`, `openai`, others) without altering storage.
* Validate at runtime that the active model matches the persisted configuration.
* Make visible which model produced each embedding or extraction.

### 4.2 Configuration check on startup

On every startup the server must:

1. Read `schema_version`, `embedding_model`, `embedding_dim`, and `embedder_id` from `system_config`.
2. Compare against the active `Embedder`. If `Dimension()` or `ModelID()` disagree, the server refuses to start and emits a clear diagnostic pointing at §11.3 (reindexing).

---

## 5. Data Model

### 5.1 Storage units

The system explicitly distinguishes between:

* **Memory Document:** the logical unit originally stored by the agent.
* **Memory Chunk:** a fragment derived from the document for vector indexing and partial retrieval.

This avoids ambiguity between original content and the fragments used for search.

### 5.2 SQL schema

```sql
-- 1. Global configuration metadata
CREATE TABLE system_config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- Canonical keys populated at initialization:
--   schema_version   -> integer as text, e.g. "1"
--   embedding_model  -> e.g. "nomic-embed-text"
--   embedding_dim    -> integer as text, e.g. "768"
--   embedder_id      -> stable identifier returned by Embedder.ModelID()

-- 2. Memory document (primary logical unit)
CREATE TABLE memories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    raw_content TEXT NOT NULL,
    content_hash TEXT NOT NULL UNIQUE,
    source_type TEXT NOT NULL DEFAULT 'manual',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    superseded_by INTEGER DEFAULT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    FOREIGN KEY(superseded_by) REFERENCES memories(id)
);

CREATE INDEX idx_memories_status        ON memories(status);
CREATE INDEX idx_memories_superseded_by ON memories(superseded_by);

-- 3. Chunks derived from the document
CREATE TABLE memory_chunks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    memory_id INTEGER NOT NULL,
    chunk_index INTEGER NOT NULL,
    chunk_text TEXT NOT NULL,
    token_estimate INTEGER,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(memory_id, chunk_index),
    FOREIGN KEY(memory_id) REFERENCES memories(id) ON DELETE CASCADE
);

CREATE INDEX idx_memory_chunks_memory_id ON memory_chunks(memory_id);

-- 4. Lexical index over chunks (FTS5, external-content)
CREATE VIRTUAL TABLE memory_chunks_fts USING fts5(
    chunk_text,
    content='memory_chunks',
    content_rowid='id',
    tokenize='unicode61 remove_diacritics 2'
);

-- 5. Vector index over chunks (sqlite-vec, rowid == memory_chunks.id)
-- Requires sqlite-vec v0.1.x or later; pin the exact version at build time.
CREATE VIRTUAL TABLE memory_chunks_vec USING vec0(
    embedding float[768]
);

-- 6. Asynchronous work queue (embeddings and graph extraction)
CREATE TABLE async_tasks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    memory_id INTEGER NOT NULL,
    task_type TEXT NOT NULL,             -- 'embed' | 'graph_extract'
    status TEXT NOT NULL DEFAULT 'pending',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    last_error TEXT DEFAULT NULL,
    leased_until DATETIME DEFAULT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME DEFAULT NULL,
    UNIQUE(memory_id, task_type),
    FOREIGN KEY(memory_id) REFERENCES memories(id) ON DELETE CASCADE
);

CREATE INDEX idx_async_tasks_status_lease ON async_tasks(status, leased_until);

-- 7. Graph node catalog
CREATE TABLE kg_nodes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    type TEXT NOT NULL,
    canonical_name TEXT NOT NULL,
    display_name TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(type, canonical_name)
);

CREATE INDEX idx_kg_nodes_canonical ON kg_nodes(canonical_name);

-- 8. Edge evidence: relations extracted from specific memories
CREATE TABLE kg_edge_evidence (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_node_id INTEGER NOT NULL,
    target_node_id INTEGER NOT NULL,
    relation_type TEXT NOT NULL,
    memory_id INTEGER NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(source_node_id, target_node_id, relation_type, memory_id),
    FOREIGN KEY(source_node_id) REFERENCES kg_nodes(id),
    FOREIGN KEY(target_node_id) REFERENCES kg_nodes(id),
    FOREIGN KEY(memory_id) REFERENCES memories(id) ON DELETE CASCADE
);

CREATE INDEX idx_edge_source    ON kg_edge_evidence(source_node_id);
CREATE INDEX idx_edge_target    ON kg_edge_evidence(target_node_id);
CREATE INDEX idx_edge_memory    ON kg_edge_evidence(memory_id);
```

Changes vs the previous revision:

* `kg_nodes` uniqueness is now `(type, canonical_name)` so the same literal can refer to nodes of different types.
* `async_tasks` uniqueness is now `(memory_id, task_type)` to support multiple task kinds per memory (embeddings and graph extraction).
* `memory_chunks_vec` no longer declares an explicit `chunk_id` column. The `rowid` of the virtual table **is** `memory_chunks.id`, written explicitly on insert.
* FTS5 declares its tokenizer explicitly (`unicode61 remove_diacritics 2`).
* Secondary indexes are declared for every access pattern used by the worker, the ranker, and `trace_dependencies`.

---

## 6. Integrity and Modeling Rules

### 6.1 Documents and chunks

* `memories` represents the full original content.
* `memory_chunks` holds the indexable fragments.
* Any chunk retrieval must be traceable back to its originating document.

### 6.2 `memory_chunks_vec` consistency

Because `memory_chunks_vec` is a virtual table, SQLite **does not propagate `ON DELETE CASCADE`** to it. Any deletion path (physical delete, future `forget` tool, re-chunking during reindex) must explicitly issue:

```sql
DELETE FROM memory_chunks_vec WHERE rowid = :chunk_id;
```

within the same SQLite transaction that removes the chunk. The same applies to FTS5 synchronization (see §10).

### 6.3 Obsolescence

`superseded_by` applies at the **document** level, not the chunk level:

* a memory may be superseded by a more recent memory;
* its chunks continue to exist for historical traceability;
* ranking penalties are applied during final retrieval.

### 6.4 Causal relations

The graph does not store a single global edge without context; it stores **evidence per memory**. This allows:

* preserving multiple proofs of the same relation;
* querying the source that produced the edge;
* reconstructing causal traces with documentary backing.

### 6.5 Node normalization

Before inserting into `kg_nodes`, a deterministic normalization pipeline must run:

* trim leading and trailing whitespace;
* collapse repeated internal whitespace;
* lowercase for case-insensitive types (configurable per node type);
* apply type-specific canonicalization where relevant;
* strip decorative characters.

Example: `Redis`, `redis`, and `REDIS` converge on the same `canonical_name`, while `display_name` preserves a readable form.

---

## 7. Chunking

### 7.1 Objective

Split long texts into units useful for embedding and search without losing document reconstruction.

### 7.2 Strategy

V1 uses a recursive splitter with the following hierarchical separators, tried in order:

1. `\n\n`
2. `\n`
3. ` ` (space)

For memories with `source_type = 'code'`, the following separators are inserted before the space, in this order: `}`, `{`, `;`. Code-aware behavior for other source types is out of scope for V1.

### 7.3 Initial recommended parameters

* Target size: **400 to 800 tokens** per chunk.
* Overlap: **10% to 15%**.
* For short texts that fit within a chunk, do not split.

### 7.4 Identity rule

Every chunk must record:

* `memory_id`
* `chunk_index`
* `chunk_text`
* `token_estimate`

This avoids relying on implicit insertion order to correlate results.

---

## 8. Ingestion Flow

### 8.1 Synchronous ingestion

#### Goal

Persist the document quickly and make it available for lexical search as soon as possible.

#### Steps

All steps happen inside **one SQLite transaction** (`BEGIN ... COMMIT`). The embedder and graph extractor are **not called during this transaction**.

1. Receive `content` from the MCP client.
2. Compute `content_hash` (see §8.3).
3. Apply the deduplication policy (see §8.4). If the incoming content is a duplicate, short-circuit and return the existing memory id.
4. Insert the document into `memories`.
5. Split the content into chunks.
6. Insert the chunks into `memory_chunks`.
7. Synchronize `memory_chunks_fts` explicitly for every inserted chunk (see §10).
8. Enqueue one `embed` task and one `graph_extract` task in `async_tasks`.
9. Commit the transaction.
10. Return confirmation to the client.

Embedding generation and vector index population happen asynchronously in the worker (see §9).

#### Latency target

No hard SLA is promised. The operational target is:

* short texts: sub-second response for `store_context`, because the synchronous path touches only SQLite;
* medium and long texts: time dominated by the number of chunks and by FTS5 insertion.

Actual latency must be measured and documented empirically rather than guaranteed.

#### Search consequences

Immediately after `store_context` returns, the content is findable via FTS5 but not yet via vector search. `search_fuzzy` must tolerate this window (see §12.3).

### 8.2 Transaction discipline

* The embedder and graph extractor are **never** called inside a SQLite transaction. Their latency is variable and must not hold write locks.
* Worker-side inserts (embeddings and graph evidence) each open their own short transaction per memory.

### 8.3 Content hashing

* Algorithm: **SHA-256**, lowercase hex.
* Input preparation before hashing:
  1. Unicode NFC normalization.
  2. Trim leading and trailing whitespace.
  3. Internal whitespace is preserved as-is.
* This is applied only for deduplication; `raw_content` stores the input as received.

### 8.4 Deduplication policy

Default behavior for V1: **return the existing memory**.

* If `content_hash` already exists and `force_reingest` is not set, `store_context` returns the existing `memory_id` with `deduplicated: true` and does not create new rows.
* If `force_reingest` is `true`, a new memory is created with the same content hash **only if** the caller passes `supersedes_memory_id` pointing to the existing entry (the previous one is marked as superseded). Otherwise the call is rejected with `DUPLICATE_CONTENT`.

This avoids silent data duplication while allowing intentional re-ingestion.

---

## 9. Asynchronous Enrichment

### 9.1 Objective

Produce chunk embeddings and extract entities/relations without blocking ingestion.

### 9.2 Task selection and leasing

The worker polls `async_tasks` using the following rule:

> A task may transition to `processing` only if it is currently `pending`, or if its `leased_until` has expired.

Concretely:

```sql
UPDATE async_tasks
SET status = 'processing',
    leased_until = :now + :lease_duration,
    attempt_count = attempt_count + 1,
    updated_at = :now
WHERE id = (
    SELECT id FROM async_tasks
    WHERE status = 'pending'
       OR (status = 'processing' AND leased_until < :now)
    ORDER BY created_at
    LIMIT 1
)
RETURNING *;
```

* Lease duration: **5 minutes** by default, configurable via `ASYNC_LEASE_SECONDS`.
* Each attempt increments `attempt_count`.
* On failure, `last_error` is recorded and the task stays in `processing` until the lease expires, at which point it becomes re-selectable.
* After a configurable maximum of attempts (default **5**), the task transitions to `failed` and is no longer polled.

Valid states: `pending`, `processing`, `completed`, `failed`.

### 9.3 Embedding task (`task_type = 'embed'`)

1. Load all chunks for the task's `memory_id`.
2. For each chunk, call `Embedder.GenerateVector`.
3. Validate that `Embedder.Dimension()` still matches `embedding_dim` in `system_config`.
4. For each chunk, execute:
   ```sql
   INSERT INTO memory_chunks_vec(rowid, embedding) VALUES (:chunk_id, :vec);
   ```
   (use `INSERT OR REPLACE` to make the task idempotent.)
5. Mark the task as `completed`.

Steps 4 are grouped into a short transaction per memory.

### 9.4 Graph extraction task (`task_type = 'graph_extract'`)

The worker sends the text to the extractor with a strict JSON-output prompt. Example system prompt:

> You are a technical graph extractor. You will receive code, logs, or technical notes. Return only valid JSON with the structure `{"nodes": [{"id": "string", "type": "TECH|ERROR|FILE|CMD", "name": "string"}], "edges": [{"source": "string", "target": "string", "relation": "DEPENDS_ON|RESOLVES|CAUSES"}]}`. Do not return explanations.

#### Insertion steps

1. Parse JSON. On parse failure, record `last_error` and let the lease expire for retry.
2. Normalize nodes (see §6.5).
3. Resolve or create `kg_nodes` using `UNIQUE(type, canonical_name)`.
4. Insert relations into `kg_edge_evidence` with `memory_id`. `INSERT OR IGNORE` makes the task idempotent.
5. Mark the task as `completed`.

---

## 10. Lexical Index

### 10.1 Scope

FTS5 is applied to `memory_chunks`, not to `memories`, to remain consistent with the smallest retrievable unit.

### 10.2 Synchronization

`memory_chunks_fts` is declared as an **external-content** FTS5 table synchronized automatically via SQLite triggers:

```sql
CREATE TRIGGER IF NOT EXISTS memory_chunks_ai AFTER INSERT ON memory_chunks BEGIN
    INSERT INTO memory_chunks_fts(rowid, chunk_text) VALUES (new.id, new.chunk_text);
END;

CREATE TRIGGER IF NOT EXISTS memory_chunks_ad AFTER DELETE ON memory_chunks BEGIN
    INSERT INTO memory_chunks_fts(memory_chunks_fts, rowid, chunk_text) VALUES ('delete', old.id, old.chunk_text);
END;

CREATE TRIGGER IF NOT EXISTS memory_chunks_au AFTER UPDATE ON memory_chunks BEGIN
    INSERT INTO memory_chunks_fts(memory_chunks_fts, rowid, chunk_text) VALUES ('delete', old.id, old.chunk_text);
    INSERT INTO memory_chunks_fts(rowid, chunk_text) VALUES (new.id, new.chunk_text);
END;
```

For bulk repair or migration:

```sql
INSERT INTO memory_chunks_fts(memory_chunks_fts) VALUES ('rebuild');
```

### 10.3 Tokenizer

`unicode61 remove_diacritics 2` is the V1 choice: tolerant of Spanish/English mixing and lightweight enough for low latency. If usage patterns show heavy subtoken search over code identifiers, a future revision may switch to `trigram`.

---

## 11. Vector Index

### 11.1 Scope

Embeddings are generated per chunk and inserted by the worker (§9.3), not during synchronous ingestion.

### 11.2 Consistency rule

Before inserting embeddings, the application must validate:

* that `Embedder.Dimension()` matches `embedding_dim` in `system_config`;
* that `Embedder.ModelID()` matches `embedder_id`, or that the change is explicitly authorized by a reindexing migration.

A mismatch at runtime aborts the task and surfaces through the observability layer (§16).

### 11.3 Reindexing

If the embedding model or dimensionality changes:

* either a full reindex of `memory_chunks_vec` must be triggered, or
* a new database must be initialized.

V1 ships with a one-shot reindex CLI (`engram reindex`) that:

1. Drops and recreates `memory_chunks_vec` with the new dimension.
2. Updates `system_config` atomically.
3. Enqueues one `embed` task per existing memory.

---

## 12. Retrieval and Ranking

### 12.1 Hybrid strategy

`search_fuzzy` combines two engines:

* lexical retrieval via FTS5;
* semantic retrieval via the vector index.

### 12.2 Fusion

Reciprocal Rank Fusion (RRF) is applied **at the chunk level first**:

$$
\text{RRFscore}(c) = \sum_{r \in R} \frac{1}{k + r(c)}
$$

Where:

* `R` is the set of input rankings (FTS5, vector);
* `r(c)` is the position of chunk `c` in each ranking;
* `k = 60` is the initial recommended value.

### 12.3 Ranking unit

The pipeline is:

1. Run FTS5 and vector search independently, each returning top-N chunks (default N = 50).
2. Apply RRF across both chunk rankings to produce a unified chunk ranking.
3. Group the ranked chunks by `memory_id`. For each memory, the document-level score is the **maximum** RRF score among its chunks; the document is represented by its top-scoring chunks as highlights.
4. Sort memories by document-level score, apply obsolescence penalty (§12.4), and return the top `limit`.

Because vector search may miss chunks whose `embed` task is still pending, FTS5 still contributes during the indexing window and degrades gracefully to lexical-only results for freshly ingested content.

### 12.4 Obsolescence penalty

If a memory has `superseded_by IS NOT NULL`, its document-level score is multiplied by a configurable factor:

* active memory: `score_final = score_base`
* superseded memory: `score_final = score_base * 0.5` (default)

The factor is read from `system_config` under key `superseded_score_factor`.

---

## 13. Supersedence Policy

The previous revision declared the field but not the policy.

### Policy

Supersedence is **never inferred automatically** in V1. It is established via:

* an explicit `supersedes_memory_id` flag on `store_context`, or
* a future dedicated MCP tool (out of scope for V1).

This avoids unsafe heuristics that might replace historical memory by mistake.

---

## 14. MCP Tools

The server exposes tools with explicit contracts.

### 14.1 `store_context`

Input:

```json
{
  "content": "string",
  "source_type": "manual|log|code|note",
  "project": "string|null",
  "supersedes_memory_id": "number|null",
  "force_reingest": "boolean|null"
}
```

Output:

```json
{
  "memory_id": 123,
  "status": "stored, pending processing"
}
```

`pending_tasks` lets the caller know which enrichment tasks are queued. Tasks already completed at return time are omitted.

### 14.2 `search_fuzzy`

Input:

```json
{
  "query": "string",
  "limit": 10,
  "project": "string|null"
}
```

Output:

```json
{
  "results": [
    {
      "memory_id": 123,
      "score": 0.82,
      "is_superseded": false,
      "vector_coverage": "complete|partial|none",
      "highlights": [
        {
          "chunk_id": 55,
          "chunk_index": 1,
          "text": "relevant fragment"
        }
      ]
    }
  ]
}
```

`vector_coverage` signals whether every chunk of the memory has been embedded at query time. Callers that care about recall can choose to retry later when coverage is not `complete`.

### 14.3 `search_exact`

Input:

```json
{
  "keyword": "string",
  "limit": 10,
  "project": "string|null"
}
```

Output:

```json
{
  "results": [
    {
      "memory_id": 123,
      "chunk_id": 55,
      "chunk_index": 1,
      "text": "exact match"
    }
  ]
}
```

### 14.4 `trace_dependencies`

Input:

```json
{
  "entity_name": "string",
  "direction": "downstream|upstream|both",
  "max_depth": 5,
  "max_nodes": 100
}
```

* `entity_name` is passed through the same normalization pipeline used at insertion (§6.5) before lookup.
* `direction = downstream` follows edges where the starting node is `source_node_id` (what it depends on / causes).
* `direction = upstream` follows edges where the starting node is `target_node_id` (what depends on it / what causes it).
* `direction = both` is the undirected traversal.
* Traversal is implemented via `WITH RECURSIVE` with explicit deduplication by `(node_id, depth)` and a hard cap of `max_nodes` returned nodes regardless of depth, to prevent pathological fanout.

Output:

```json
{
  "entity": "redis",
  "nodes": [
    {"id": 1, "type": "TECH", "name": "redis"},
    {"id": 2, "type": "ERROR", "name": "connection timeout"}
  ],
  "edges": [
    {
      "source_id": 2,
      "target_id": 1,
      "relation_type": "CAUSES",
      "memory_id": 123
    }
  ],
  "truncated": false
}
```

`truncated: true` indicates that `max_nodes` or `max_depth` cut the traversal short.

### 14.5 Out of scope for V1

The following tools are intentionally deferred:

* `delete_memory`: until it ships, memories are effectively immutable from the client. `ON DELETE CASCADE` in the schema is provisioned for future use and for tests, but no user-facing path exercises it.
* `update_memory`: mutation of stored content is not supported; create a new memory with `supersedes_memory_id` pointing to the old one.

### 14.6 MCP errors

Every tool must respond with structured errors:

```json
{
  "error": {
    "code": "INVALID_INPUT",
    "message": "content must not be empty"
  }
}
```

Minimum error codes: `INVALID_INPUT`, `NOT_FOUND`, `DUPLICATE_CONTENT`, `EMBEDDING_DIM_MISMATCH`, `INTERNAL`.

---

## 15. SQLite Concurrency Considerations

SQLite supports this use case, but it must be operated carefully.

Required configuration:

* **WAL mode** enabled at initialization (`PRAGMA journal_mode = WAL`).
* **Busy timeout** set (`PRAGMA busy_timeout = 5000`).
* **Foreign keys** enforced (`PRAGMA foreign_keys = ON`).

Operational rules:

* Keep transactions short; never wrap network I/O (embedder, extractor) in a transaction.
* The worker must not hold write locks longer than necessary; one memory per transaction is the unit.
* Vector index writes must be serialized if the `sqlite-vec` version in use requires it (verify against the pinned version at build time).

---

## 16. Observability

The system uses environment variables prefixed with `HSME_OBS_*`:

| Variable | Default | Description |
|----------|---------|-------------|
| `HSME_OBS_LEVEL` | `off` | Observability level: `off`, `basic`, `debug`, `trace` |
| `HSME_OBS_SAMPLE_RATE` | `0.10` | Sampling rate (0.0-1.0) for basic mode |
| `HSME_OBS_SLOW_THRESHOLDS` | (see below) | Comma-separated `key=duration` thresholds |
| `HSME_OBS_RAW_RETENTION_DAYS` | `7` | Retention for raw traces/spans/events |
| `HSME_OBS_MINUTE_RETENTION_DAYS` | `7` | Retention for minute rollups |
| `HSME_OBS_HOUR_RETENTION_DAYS` | `30` | Retention for hour rollups |
| `HSME_OBS_DAY_RETENTION_DAYS` | `365` | Retention for day rollups |
| `HSME_OBS_FLUSH_INTERVAL_SECONDS` | `60` | Flush interval for rollups |

Default slow thresholds:
- `mcp.request`: 100ms
- `mcp.tools/call`: 100ms
- `worker.lease`: 200ms
- `worker.execute`: 2s
- `ops.raw_to_minute`: 2s
- `ops.retention`: 2s

The system records:
- `store_context` latency
- embedding latency per chunk
- chunks per memory
- counts of tasks by state (`pending`, `processing`, `completed`, `failed`)
- graph extraction error rate
- FTS5 and vector search latency
- count of `vector_coverage` values returned by `search_fuzzy`
- rollup jobs: `raw_to_minute`, `minute_to_hour`, `hour_to_day`, `retention_cleanup`
- checkpoint (`last_completed_bucket_start_utc`) persisted for catch-up on restart

---

## 17. Additional MCP Tools

### 14.5 `recall_recent_session`

Retrieves recent session summaries in chronological order (pure SQL, no embeddings).

Input:

```json
{
  "project": "string|null",
  "limit": 5
}
```

Output:

```json
{
  "results": [
    {
      "memory_id": 123,
      "score": 0.82,
      "is_superseded": false,
      "vector_coverage": "complete|partial|none",
      "highlights": [...]
    }
  ]
}
```

### 14.6 `explore_knowledge_graph`

Traces entity dependencies across the knowledge graph.

Input:

```json
{
  "entity_name": "string",
  "direction": "downstream|upstream|both",
  "max_depth": 5,
  "max_nodes": 100
}
```

Output:

```json
{
  "entity": "redis",
  "nodes": [...],
  "edges": [...],
  "truncated": false
}
```

---

## 18. Residual Risks

Even after this revision, operational risks remain:

1. Local models introduce variable latency; the async design mitigates but does not eliminate user-visible delays in `vector_coverage`.
2. Graph extraction may produce invalid JSON and require retries; the lease/attempt system bounds the cost but not the failure rate.
3. SQLite may become a bottleneck if write concurrency grows substantially; a sidecar-worker topology is the documented evolution path.
4. Hybrid ranking quality depends on empirical tuning of chunk size, embedding model, and the RRF constant `k`.
5. Entity normalization may require domain-specific rules beyond the generic pipeline.
6. `sqlite-vec` is a young project; the pinned version must be revisited on upgrade.

---

## 19. Scope of V1

V1 focuses on:

* memory persistence;
* explicit chunking;
* FTS5 and vector search with RRF;
* asynchronous worker with leasing and retries for both embedding and graph extraction;
* graph with per-memory evidence;
* stable MCP contracts including `store_context`, `search_fuzzy`, `search_exact`, `recall_recent_session`, and `explore_knowledge_graph`;
* explicit schema versioning;
* observability with automatic rollups and retention policies.

Not recommended for V1:

* automatic supersedence inference;
* multiple active embedding models sharing the same database;
* advanced semantic deduplication;
* probabilistic entity fusion;
* a dedicated long-running worker container;
* `delete_memory` and `update_memory` tools.

---

## 20. Viability Criteria

The project is considered viable for implementation provided that:

* the document/chunk split is preserved;
* embedding dimensionality is frozen per database;
* the Linux runtime matches the ABI of the compiled `sqlite-vec` extension, and the Go driver supports dynamic extension loading;
* the async queue handles leasing, retries, and per-type tasks;
* MCP tools expose explicit contracts including the `vector_coverage` signal;
* the supersedence policy remains manual and explicit;
* the FTS5 external-content table is kept in sync via database triggers on every mutation to `memory_chunks`;
* the `memory_chunks_vec` virtual table is cleaned up explicitly on any chunk deletion path.