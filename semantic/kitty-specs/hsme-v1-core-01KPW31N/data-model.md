# Data Model: HSME V1 Core

## Entities

### Memory Document (`memories`)
- `id` (INTEGER, PK)
- `raw_content` (TEXT)
- `content_hash` (TEXT, UNIQUE)
- `source_type` (TEXT)
- `created_at`, `updated_at` (DATETIME)
- `superseded_by` (INTEGER, FK to memories)
- `status` (TEXT)

### Memory Chunk (`memory_chunks`)
- `id` (INTEGER, PK)
- `memory_id` (INTEGER, FK)
- `chunk_index` (INTEGER)
- `chunk_text` (TEXT)
- `token_estimate` (INTEGER)

### Lexical Index (`memory_chunks_fts`)
- External-content FTS5 table indexing `chunk_text` using `unicode61 remove_diacritics 2`.

### Vector Index (`memory_chunks_vec`)
- Virtual table using `vec0` extension holding `embedding float[768]`.

### Async Task (`async_tasks`)
- `id` (INTEGER, PK)
- `memory_id` (INTEGER, FK)
- `task_type` (TEXT: 'embed', 'graph_extract')
- `status` (TEXT: 'pending', 'processing', 'completed', 'failed')
- `attempt_count` (INTEGER)
- `last_error` (TEXT)
- `leased_until` (DATETIME)

### Knowledge Graph Nodes (`kg_nodes`)
- `id` (INTEGER, PK)
- `type` (TEXT)
- `canonical_name` (TEXT)
- `display_name` (TEXT)

### Knowledge Graph Edges (`kg_edge_evidence`)
- `source_node_id`, `target_node_id` (INTEGER, FKs)
- `relation_type` (TEXT)
- `memory_id` (INTEGER, FK)
