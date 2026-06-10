package sqlite

import (
	"database/sql"
	"fmt"
	"time"

	vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	"github.com/mattn/go-sqlite3"
)

func init() {
	// Automatically load sqlite-vec for all new connections
	vec.Auto()

	sql.Register("sqlite3_custom", &sqlite3.SQLiteDriver{
		ConnectHook: func(conn *sqlite3.SQLiteConn) error {
			// Enable extension loading
			conn.SetLimit(sqlite3.SQLITE_LIMIT_VARIABLE_NUMBER, 32766)
			return nil
		},
	})
}

const schema = `
-- 1. Global configuration metadata
CREATE TABLE IF NOT EXISTS system_config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- 2. Memory document
CREATE TABLE IF NOT EXISTS memories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    raw_content TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    source_type TEXT NOT NULL DEFAULT 'manual',
    project TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    superseded_by INTEGER DEFAULT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    FOREIGN KEY(superseded_by) REFERENCES memories(id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_memories_active_hash ON memories(content_hash) WHERE status = 'active';

CREATE INDEX IF NOT EXISTS idx_memories_status        ON memories(status);
CREATE INDEX IF NOT EXISTS idx_memories_superseded_by ON memories(superseded_by);
CREATE INDEX IF NOT EXISTS idx_memories_project       ON memories(project);
CREATE INDEX IF NOT EXISTS idx_memories_source_type_created ON memories(source_type, created_at DESC);

-- 3. Chunks derived from the document
CREATE TABLE IF NOT EXISTS memory_chunks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    memory_id INTEGER NOT NULL,
    chunk_index INTEGER NOT NULL,
    chunk_text TEXT NOT NULL,
    token_estimate INTEGER,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(memory_id, chunk_index),
    FOREIGN KEY(memory_id) REFERENCES memories(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_memory_chunks_memory_id ON memory_chunks(memory_id);

-- 4. Lexical index over chunks (FTS5)
CREATE VIRTUAL TABLE IF NOT EXISTS memory_chunks_fts USING fts5(
    chunk_text,
    content='memory_chunks',
    content_rowid='id',
    tokenize='unicode61 remove_diacritics 2'
);

-- 4.a Triggers de sincronización memory_chunks <-> memory_chunks_fts.
-- Sin esto, cualquier UPDATE o DELETE sobre memory_chunks deja el índice léxico
-- desincronizado y el FTS devuelve resultados fantasma o pierde filas.
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

-- 5. Vector index over chunks (sqlite-vec)
CREATE VIRTUAL TABLE IF NOT EXISTS memory_chunks_vec USING vec0(
    embedding float[768]
);

-- 6. Asynchronous work queue
CREATE TABLE IF NOT EXISTS async_tasks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    memory_id INTEGER NOT NULL,
    task_type TEXT NOT NULL,
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

CREATE INDEX IF NOT EXISTS idx_async_tasks_status_lease ON async_tasks(status, leased_until);

-- 7. Graph node catalog
CREATE TABLE IF NOT EXISTS kg_nodes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    type TEXT NOT NULL,
    canonical_name TEXT NOT NULL,
    display_name TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(type, canonical_name)
);

CREATE INDEX IF NOT EXISTS idx_kg_nodes_canonical ON kg_nodes(canonical_name);

-- 8. Edge evidence
CREATE TABLE IF NOT EXISTS kg_edge_evidence (
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

CREATE INDEX IF NOT EXISTS idx_edge_source    ON kg_edge_evidence(source_node_id);
CREATE INDEX IF NOT EXISTS idx_edge_target    ON kg_edge_evidence(target_node_id);
CREATE INDEX IF NOT EXISTS idx_edge_memory    ON kg_edge_evidence(memory_id);

-- 9. Graph edges whose endpoint nodes were not available when the extractor
-- produced the relation. The worker reconciles this table after every
-- graph_extract task so cross-memory task ordering does not permanently lose
-- valid KG evidence.
CREATE TABLE IF NOT EXISTS kg_unresolved_edges (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_name TEXT NOT NULL,
    source_canonical TEXT NOT NULL,
    target_name TEXT NOT NULL,
    target_canonical TEXT NOT NULL,
    relation_type TEXT NOT NULL,
    memory_id INTEGER NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(source_canonical, target_canonical, relation_type, memory_id),
    FOREIGN KEY(memory_id) REFERENCES memories(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_unresolved_edges_source ON kg_unresolved_edges(source_canonical);
CREATE INDEX IF NOT EXISTS idx_unresolved_edges_target ON kg_unresolved_edges(target_canonical);
CREATE INDEX IF NOT EXISTS idx_unresolved_edges_memory ON kg_unresolved_edges(memory_id);

-- 10. Observability traces
CREATE TABLE IF NOT EXISTS obs_traces (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    trace_id TEXT NOT NULL UNIQUE,
    trace_kind TEXT NOT NULL CHECK (trace_kind IN ('mcp_request', 'worker_task', 'maintenance')),
    parent_trace_id TEXT,
    request_id TEXT,
    tool_name TEXT,
    task_id INTEGER,
    task_type TEXT,
    memory_id INTEGER,
    component TEXT NOT NULL,
    operation_name TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('ok', 'error', 'cancelled', 'timeout')),
    obs_level TEXT NOT NULL CHECK (obs_level IN ('off', 'basic', 'debug', 'trace')),
    sampled INTEGER NOT NULL DEFAULT 0 CHECK (sampled IN (0, 1)),
    started_at_utc TEXT NOT NULL,
    ended_at_utc TEXT NOT NULL,
    duration_us INTEGER NOT NULL DEFAULT 0 CHECK (duration_us >= 0),
    error_code TEXT,
    error_message TEXT,
    meta_json TEXT,
    created_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_obs_traces_started_at ON obs_traces(started_at_utc);
CREATE INDEX IF NOT EXISTS idx_obs_traces_kind_started ON obs_traces(trace_kind, started_at_utc);
CREATE INDEX IF NOT EXISTS idx_obs_traces_tool_started ON obs_traces(tool_name, started_at_utc);
CREATE INDEX IF NOT EXISTS idx_obs_traces_task_started ON obs_traces(task_type, started_at_utc);
CREATE INDEX IF NOT EXISTS idx_obs_traces_status_started ON obs_traces(status, started_at_utc);

-- 11. Observability spans
CREATE TABLE IF NOT EXISTS obs_spans (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    trace_id TEXT NOT NULL,
    span_id TEXT NOT NULL,
    parent_span_id TEXT,
    component TEXT NOT NULL,
    operation_name TEXT NOT NULL,
    stage_name TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('ok', 'error', 'cancelled', 'timeout')),
    started_at_utc TEXT NOT NULL,
    ended_at_utc TEXT NOT NULL,
    duration_us INTEGER NOT NULL DEFAULT 0 CHECK (duration_us >= 0),
    queue_delay_us INTEGER,
    rows_read INTEGER,
    rows_written INTEGER,
    bytes_in INTEGER,
    bytes_out INTEGER,
    error_code TEXT,
    error_message TEXT,
    meta_json TEXT,
    created_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE(trace_id, span_id),
    FOREIGN KEY(trace_id) REFERENCES obs_traces(trace_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_obs_spans_trace_started ON obs_spans(trace_id, started_at_utc);
CREATE INDEX IF NOT EXISTS idx_obs_spans_component_stage ON obs_spans(component, stage_name, started_at_utc);
CREATE INDEX IF NOT EXISTS idx_obs_spans_duration ON obs_spans(duration_us, started_at_utc);

-- 12. Observability discrete events
CREATE TABLE IF NOT EXISTS obs_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    trace_id TEXT,
    span_id TEXT,
    event_kind TEXT NOT NULL CHECK (event_kind IN ('error', 'slow_operation', 'state_transition', 'diagnostic')),
    component TEXT NOT NULL,
    operation_name TEXT NOT NULL,
    tool_name TEXT,
    task_id INTEGER,
    task_type TEXT,
    memory_id INTEGER,
    severity TEXT NOT NULL CHECK (severity IN ('debug', 'info', 'warn', 'error')),
    threshold_us INTEGER,
    observed_us INTEGER,
    message TEXT NOT NULL,
    details_json TEXT,
    created_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    FOREIGN KEY(trace_id) REFERENCES obs_traces(trace_id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_obs_events_created ON obs_events(created_at_utc);
CREATE INDEX IF NOT EXISTS idx_obs_events_kind_created ON obs_events(event_kind, created_at_utc);
CREATE INDEX IF NOT EXISTS idx_obs_events_component_created ON obs_events(component, created_at_utc);
CREATE INDEX IF NOT EXISTS idx_obs_events_tool_created ON obs_events(tool_name, created_at_utc);
CREATE INDEX IF NOT EXISTS idx_obs_events_task_created ON obs_events(task_type, created_at_utc);
CREATE INDEX IF NOT EXISTS idx_obs_events_trace ON obs_events(trace_id, created_at_utc);

-- 13. Metric rollups
CREATE TABLE IF NOT EXISTS obs_metric_rollups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    bucket_level TEXT NOT NULL CHECK (bucket_level IN ('minute', 'hour', 'day')),
    bucket_start_utc TEXT NOT NULL,
    component TEXT NOT NULL,
    operation_name TEXT NOT NULL,
    tool_name TEXT,
    task_type TEXT,
    trace_kind TEXT NOT NULL CHECK (trace_kind IN ('mcp_request', 'worker_task', 'maintenance')),
    total_count INTEGER NOT NULL DEFAULT 0,
    success_count INTEGER NOT NULL DEFAULT 0,
    error_count INTEGER NOT NULL DEFAULT 0,
    slow_count INTEGER NOT NULL DEFAULT 0,
    sampled_count INTEGER NOT NULL DEFAULT 0,
    duration_total_us INTEGER NOT NULL DEFAULT 0,
    duration_max_us INTEGER NOT NULL DEFAULT 0,
    p50_us INTEGER,
    p95_us INTEGER,
    p99_us INTEGER,
    queue_delay_total_us INTEGER NOT NULL DEFAULT 0,
    bytes_in_total INTEGER NOT NULL DEFAULT 0,
    bytes_out_total INTEGER NOT NULL DEFAULT 0,
    rows_read_total INTEGER NOT NULL DEFAULT 0,
    rows_written_total INTEGER NOT NULL DEFAULT 0,
    last_source_event_at_utc TEXT,
    created_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_obs_metric_rollups_dims
ON obs_metric_rollups(bucket_level, bucket_start_utc, component, operation_name, ifnull(tool_name, ''), ifnull(task_type, ''), trace_kind);

-- 14. Retention policies
CREATE TABLE IF NOT EXISTS obs_retention_policies (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    policy_name TEXT NOT NULL UNIQUE,
    scope_kind TEXT NOT NULL CHECK (scope_kind IN ('events', 'traces', 'spans', 'rollups')),
    bucket_level TEXT CHECK (bucket_level IN ('minute', 'hour', 'day')),
    keep_days INTEGER NOT NULL CHECK (keep_days >= 0),
    sample_rate REAL CHECK (sample_rate IS NULL OR (sample_rate >= 0.0 AND sample_rate <= 1.0)),
    slow_threshold_us INTEGER,
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    updated_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- 15. Rollup/checkpoint jobs
CREATE TABLE IF NOT EXISTS obs_rollup_jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_name TEXT NOT NULL UNIQUE,
    source_scope TEXT NOT NULL CHECK (source_scope IN ('raw_to_minute', 'minute_to_hour', 'hour_to_day', 'retention_cleanup')),
    last_completed_bucket_start_utc TEXT,
    last_run_started_at_utc TEXT,
    last_run_finished_at_utc TEXT,
    last_status TEXT NOT NULL DEFAULT 'idle' CHECK (last_status IN ('idle', 'running', 'ok', 'error')),
    last_error TEXT,
    updated_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- 16. Operator views
CREATE VIEW IF NOT EXISTS obs_recent_slow_operations AS
SELECT
    e.created_at_utc,
    e.component,
    e.operation_name,
    e.tool_name,
    e.task_type,
    e.observed_us,
    e.threshold_us,
    e.message,
    e.trace_id
FROM obs_events e
WHERE e.event_kind = 'slow_operation';

CREATE VIEW IF NOT EXISTS obs_error_events AS
SELECT
    e.created_at_utc,
    e.component,
    e.operation_name,
    e.tool_name,
    e.task_type,
    e.severity,
    e.message,
    e.details_json,
    e.trace_id
FROM obs_events e
WHERE e.event_kind = 'error';

-- 17. Default policies and rollup jobs
INSERT OR IGNORE INTO obs_retention_policies(policy_name, scope_kind, bucket_level, keep_days, sample_rate, slow_threshold_us, enabled) VALUES
    ('raw-traces-default', 'traces', NULL, 7, 0.10, NULL, 1),
    ('raw-spans-default', 'spans', NULL, 7, 0.10, NULL, 1),
    ('raw-events-default', 'events', NULL, 14, 1.0, 100000, 1),
    ('minute-rollups-default', 'rollups', 'minute', 7, NULL, NULL, 1),
    ('hour-rollups-default', 'rollups', 'hour', 30, NULL, NULL, 1),
    ('day-rollups-default', 'rollups', 'day', 365, NULL, NULL, 1);

INSERT OR IGNORE INTO obs_rollup_jobs(job_name, source_scope, last_status) VALUES
    ('raw_to_minute', 'raw_to_minute', 'idle'),
    ('minute_to_hour', 'minute_to_hour', 'idle'),
    ('hour_to_day', 'hour_to_day', 'idle'),
    ('retention_cleanup', 'retention_cleanup', 'idle');
`

func InitDB(path string) (*sql.DB, error) {
	// _txlock=immediate hace que BeginTx emita BEGIN IMMEDIATE en vez de BEGIN DEFERRED.
	// Con eso el write-lock se toma upfront y las escrituras concurrentes (ej. dos
	// store_context con el mismo contenido) serializan limpias en vez de colisionar
	// en el unique index al commit.
	dsn := fmt.Sprintf("file:%s?_txlock=immediate", path)
	db, err := sql.Open("sqlite3_custom", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Acotar el pool de conexiones. Con MaxOpenConns ilimitado y goroutines
	// concurrentes (ver mcp/handler.go: `go s.handleRequest`), SQLite bajo WAL
	// puede devolver `database is locked` cuando muchas conexiones compiten por
	// el writer. 4 permite lectores concurrentes sin disparar contention.
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Set PRAGMAs
	pragmas := []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA busy_timeout = 5000;",
		"PRAGMA foreign_keys = ON;",
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to set pragma: %w", err)
		}
	}

	// Migration: add 'project' column to 'memories' if it doesn't exist
	// sqlite-vec/sqlite3 doesn't support 'ALTER TABLE memories ADD COLUMN IF NOT EXISTS'
	// until very recent versions, so we check manually.
	var columnExists bool
	err = db.QueryRow("SELECT count(*) FROM pragma_table_info('memories') WHERE name='project'").Scan(&columnExists)
	if err != nil {
		// If pragma_table_info fails, fallback to a more compatible check
		rows, err := db.Query("PRAGMA table_info(memories)")
		if err == nil {
			for rows.Next() {
				var cid int
				var name, ctype string
				var notnull, pk int
				var dfltValue interface{}
				if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err == nil {
					if name == "project" {
						columnExists = true
						break
					}
				}
			}
			rows.Close()
		}
	}

	if !columnExists {
		if _, err := db.Exec("ALTER TABLE memories ADD COLUMN project TEXT"); err != nil {
			// ignore error if table doesn't exist yet
		}
	}

	// Apply schema
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to apply schema: %w", err)
	}

	return db, nil
}
