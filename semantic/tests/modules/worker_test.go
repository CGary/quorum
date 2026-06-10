package modules

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hsme/core/src/core/worker"
	"github.com/hsme/core/src/storage/sqlite"
	_ "github.com/mattn/go-sqlite3"
)

type mockEmbedder struct {
	dim int
}

func (m *mockEmbedder) GenerateVector(ctx context.Context, text string) ([]float32, error) {
	return make([]float32, m.dim), nil
}

func (m *mockEmbedder) Dimension() int {
	return m.dim
}

func (m *mockEmbedder) ModelID() string {
	return "mock-embedder"
}

type mockGraphExtractor struct{}
type crossMemoryGraphExtractor struct{}

type flakyLengthEmbedder struct {
	dim      int
	maxChars int
}

func (m *flakyLengthEmbedder) GenerateVector(ctx context.Context, text string) ([]float32, error) {
	if len(text) > m.maxChars {
		return nil, fmt.Errorf("ollama API returned status 500 for embeddings: the input length exceeds the context length")
	}
	return make([]float32, m.dim), nil
}

func (m *flakyLengthEmbedder) Dimension() int {
	return m.dim
}

func (m *flakyLengthEmbedder) ModelID() string {
	return "flaky-length-embedder"
}

func (m *mockGraphExtractor) ExtractEntities(ctx context.Context, text string) (worker.KnowledgeGraph, error) {
	// Usamos tipos del enum válido (§14.4: TECH|ERROR|FILE|CMD); el worker
	// ahora filtra cualquier otro tipo por spec §6.5.
	return worker.KnowledgeGraph{
		Nodes: []worker.Node{
			{Type: "TECH", Name: "Entity A"},
			{Type: "TECH", Name: "Entity B"},
		},
		Edges: []worker.Edge{
			{Source: "Entity A", Target: "Entity B", Relation: "DEPENDS_ON"},
		},
	}, nil
}

func (m *crossMemoryGraphExtractor) ExtractEntities(ctx context.Context, text string) (worker.KnowledgeGraph, error) {
	switch {
	case strings.Contains(text, "alpha"):
		return worker.KnowledgeGraph{
			Nodes: []worker.Node{
				{Type: "TECH", Name: "Alpha architecture"},
			},
			Edges: []worker.Edge{
				{Source: "Alpha architecture", Target: "BETA SERVICE", Relation: "DEPENDS_ON"},
			},
		}, nil
	case strings.Contains(text, "beta"):
		return worker.KnowledgeGraph{
			Nodes: []worker.Node{
				{Type: "TECH", Name: "Beta service"},
			},
		}, nil
	default:
		return worker.KnowledgeGraph{}, nil
	}
}

func TestLeasingLogic(t *testing.T) {
	dbPath := "test_worker_leasing.db"
	defer os.Remove(dbPath)

	db, err := sqlite.InitDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Insert memories for foreign key constraints
	_, err = db.Exec("INSERT INTO memories (id, raw_content, content_hash, status) VALUES (1, 'content', 'hash1', 'active')")
	if err != nil {
		t.Fatalf("Failed to insert memory: %v", err)
	}

	w := worker.NewWorker(db, &mockEmbedder{dim: 768}, &mockGraphExtractor{}, nil)

	// 1. Test leasing a pending task
	_, err = db.Exec("INSERT INTO async_tasks (memory_id, task_type, status) VALUES (1, 'embed', 'pending')")
	if err != nil {
		t.Fatalf("Failed to insert task: %v", err)
	}

	task, err := w.LeaseNextTask(context.Background())
	if err != nil {
		t.Fatalf("Failed to lease task: %v", err)
	}
	if task == nil {
		t.Fatal("Expected task to be leased, got nil")
	}
	if task.Status != "processing" {
		t.Errorf("Expected status 'processing', got %s", task.Status)
	}
	if task.LeasedUntil == nil || task.LeasedUntil.Before(time.Now()) {
		t.Errorf("Expected LeasedUntil to be in the future, got %v", task.LeasedUntil)
	}

	// 2. Test leasing a task that timed out
	_, err = db.Exec("UPDATE async_tasks SET status='processing', leased_until=?", time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("Failed to update task: %v", err)
	}

	task2, err := w.LeaseNextTask(context.Background())
	if err != nil {
		t.Fatalf("Failed to lease task: %v", err)
	}
	if task2 == nil {
		t.Fatal("Expected timed out task to be leased, got nil")
	}
	if task2.ID != task.ID {
		t.Errorf("Expected to lease the same task ID, got %d", task2.ID)
	}

	// 3. Test retirement after 5 attempts
	_, err = db.Exec("UPDATE async_tasks SET status='pending', attempt_count=5 WHERE id=?", task.ID)
	if err != nil {
		t.Fatalf("Failed to update attempt_count: %v", err)
	}

	task3, err := w.LeaseNextTask(context.Background())
	if err != nil {
		t.Fatalf("Failed to lease task: %v", err)
	}
	if task3 != nil {
		t.Errorf("Expected no task to be leased after 5 attempts, got task ID %d", task3.ID)
	}
}

func TestWorkerExecution(t *testing.T) {
	dbPath := "test_worker_exec.db"
	defer os.Remove(dbPath)

	db, err := sqlite.InitDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Setup memory and chunks
	_, err = db.Exec("INSERT INTO memories (id, raw_content, content_hash, status) VALUES (1, 'content', 'hash1', 'active')")
	if err != nil {
		t.Fatalf("Failed to insert memory: %v", err)
	}
	_, err = db.Exec("INSERT INTO memory_chunks (id, memory_id, chunk_index, chunk_text) VALUES (1, 1, 0, 'chunk content')")
	if err != nil {
		t.Fatalf("Failed to insert chunk: %v", err)
	}

	w := worker.NewWorker(db, &mockEmbedder{dim: 768}, &mockGraphExtractor{}, nil)

	// Test Embed Task Execution
	_, err = db.Exec("INSERT INTO async_tasks (memory_id, task_type, status) VALUES (1, 'embed', 'pending')")
	if err != nil {
		t.Fatalf("Failed to insert task: %v", err)
	}

	task, err := w.LeaseNextTask(context.Background())
	if err != nil {
		t.Fatalf("Failed to lease task: %v", err)
	}

	err = w.ExecuteTask(context.Background(), task)
	if err != nil {
		t.Fatalf("Failed to execute embed task: %v", err)
	}

	// Verify task completed
	var status string
	err = db.QueryRow("SELECT status FROM async_tasks WHERE id=?", task.ID).Scan(&status)
	if err != nil {
		t.Fatalf("Failed to query task status: %v", err)
	}
	if status != "completed" {
		t.Errorf("Expected status 'completed', got %s", status)
	}

	// Verify vector created (skip if vec0 not working correctly in test environment, but try it)
	var count int
	err = db.QueryRow("SELECT count(*) FROM memory_chunks_vec").Scan(&count)
	if err != nil {
		t.Logf("Vector table query failed (expected if vec0 missing): %v", err)
	} else if count != 1 {
		t.Errorf("Expected 1 vector in memory_chunks_vec, got %d", count)
	}

	// Test Graph Extract Task Execution
	_, err = db.Exec("INSERT INTO async_tasks (memory_id, task_type, status) VALUES (1, 'graph_extract', 'pending')")
	if err != nil {
		t.Fatalf("Failed to insert task: %v", err)
	}

	taskGE, err := w.LeaseNextTask(context.Background())
	if err != nil {
		t.Fatalf("Failed to lease task: %v", err)
	}

	err = w.ExecuteTask(context.Background(), taskGE)
	if err != nil {
		t.Fatalf("Failed to execute graph_extract task: %v", err)
	}

	// Verify nodes and evidence created
	// mockGraphExtractor returns "Entity A", "Entity B"
	err = db.QueryRow("SELECT count(*) FROM kg_nodes").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query kg_nodes: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 nodes, got %d", count)
	}

	err = db.QueryRow("SELECT count(*) FROM kg_edge_evidence").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query kg_edge_evidence: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 edge evidence, got %d", count)
	}
}

func TestWorkerExecution_RechunksOversizedEmbedInput(t *testing.T) {
	dbPath := "test_worker_rechunk.db"
	defer os.Remove(dbPath)

	db, err := sqlite.InitDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	content := strings.TrimSpace(strings.Repeat("abcdefgh ", 500))
	_, err = db.Exec("INSERT INTO memories (id, raw_content, content_hash, source_type, status) VALUES (1, ?, 'hash-rechunk', 'note', 'active')", content)
	if err != nil {
		t.Fatalf("Failed to insert memory: %v", err)
	}
	_, err = db.Exec("INSERT INTO memory_chunks (id, memory_id, chunk_index, chunk_text, token_estimate) VALUES (1, 1, 0, ?, 500)", content)
	if err != nil {
		t.Fatalf("Failed to insert oversized chunk: %v", err)
	}

	w := worker.NewWorker(db, &flakyLengthEmbedder{dim: 768, maxChars: 3200}, &mockGraphExtractor{}, nil)

	_, err = db.Exec("INSERT INTO async_tasks (memory_id, task_type, status) VALUES (1, 'embed', 'pending')")
	if err != nil {
		t.Fatalf("Failed to insert embed task: %v", err)
	}

	task, err := w.LeaseNextTask(context.Background())
	if err != nil {
		t.Fatalf("Failed to lease task: %v", err)
	}
	if task == nil {
		t.Fatal("Expected embed task to be leased")
	}

	if err := w.ExecuteTask(context.Background(), task); err != nil {
		t.Fatalf("Expected rechunk-and-embed to succeed, got: %v", err)
	}

	var chunkCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM memory_chunks WHERE memory_id = 1").Scan(&chunkCount); err != nil {
		t.Fatalf("Failed to count chunks: %v", err)
	}
	if chunkCount < 2 {
		t.Fatalf("Expected oversized chunk to be split into multiple chunks, got %d", chunkCount)
	}

	var maxChunkLen int
	if err := db.QueryRow("SELECT MAX(LENGTH(chunk_text)) FROM memory_chunks WHERE memory_id = 1").Scan(&maxChunkLen); err != nil {
		t.Fatalf("Failed to query max chunk length: %v", err)
	}
	if maxChunkLen > 3200 {
		t.Fatalf("Expected rechunked pieces <= 3200 chars, got %d", maxChunkLen)
	}

	var status string
	if err := db.QueryRow("SELECT status FROM async_tasks WHERE id = ?", task.ID).Scan(&status); err != nil {
		t.Fatalf("Failed to query task status: %v", err)
	}
	if status != "completed" {
		t.Fatalf("Expected task status completed, got %s", status)
	}
}

func TestWorkerExecution_ReconcilesCrossMemoryUnresolvedEdges(t *testing.T) {
	dbPath := "test_worker_unresolved_edges.db"
	defer os.Remove(dbPath)

	db, err := sqlite.InitDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`
		INSERT INTO memories (id, raw_content, content_hash, status) VALUES
			(1, 'alpha memory', 'hash-alpha', 'active'),
			(2, 'beta memory', 'hash-beta', 'active')`)
	if err != nil {
		t.Fatalf("Failed to insert memories: %v", err)
	}
	_, err = db.Exec(`
		INSERT INTO async_tasks (memory_id, task_type, status, created_at) VALUES
			(1, 'graph_extract', 'pending', '2026-04-26T00:00:00Z'),
			(2, 'graph_extract', 'pending', '2026-04-26T00:00:01Z')`)
	if err != nil {
		t.Fatalf("Failed to insert graph tasks: %v", err)
	}

	w := worker.NewWorker(db, &mockEmbedder{dim: 768}, &crossMemoryGraphExtractor{}, nil)

	firstTask, err := w.LeaseNextTask(context.Background())
	if err != nil {
		t.Fatalf("Failed to lease first task: %v", err)
	}
	if firstTask == nil || firstTask.MemoryID != 1 {
		t.Fatalf("Expected first leased task for memory 1, got %#v", firstTask)
	}
	if err := w.ExecuteTask(context.Background(), firstTask); err != nil {
		t.Fatalf("Failed to execute first graph task: %v", err)
	}

	var edgeCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM kg_edge_evidence").Scan(&edgeCount); err != nil {
		t.Fatalf("Failed to count edge evidence after first task: %v", err)
	}
	if edgeCount != 0 {
		t.Fatalf("Expected unresolved edge to remain deferred after first task, got %d edge evidence rows", edgeCount)
	}
	var unresolvedCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM kg_unresolved_edges").Scan(&unresolvedCount); err != nil {
		t.Fatalf("Failed to count unresolved edges: %v", err)
	}
	if unresolvedCount != 1 {
		t.Fatalf("Expected 1 unresolved edge after first task, got %d", unresolvedCount)
	}

	secondTask, err := w.LeaseNextTask(context.Background())
	if err != nil {
		t.Fatalf("Failed to lease second task: %v", err)
	}
	if secondTask == nil || secondTask.MemoryID != 2 {
		t.Fatalf("Expected second leased task for memory 2, got %#v", secondTask)
	}
	if err := w.ExecuteTask(context.Background(), secondTask); err != nil {
		t.Fatalf("Failed to execute second graph task: %v", err)
	}

	err = db.QueryRow(`
		SELECT COUNT(*)
		  FROM kg_edge_evidence e
		  JOIN kg_nodes source ON source.id = e.source_node_id
		  JOIN kg_nodes target ON target.id = e.target_node_id
		 WHERE source.canonical_name = 'alpha architecture'
		   AND target.canonical_name = 'beta service'
		   AND e.relation_type = 'DEPENDS_ON'
		   AND e.memory_id = 1`).Scan(&edgeCount)
	if err != nil {
		t.Fatalf("Failed to count reconciled edge evidence: %v", err)
	}
	if edgeCount != 1 {
		t.Fatalf("Expected deferred Alpha -> Beta edge evidence to be reconciled, got %d", edgeCount)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM kg_unresolved_edges").Scan(&unresolvedCount); err != nil {
		t.Fatalf("Failed to count unresolved edges after reconciliation: %v", err)
	}
	if unresolvedCount != 0 {
		t.Fatalf("Expected unresolved edge table to be empty after reconciliation, got %d", unresolvedCount)
	}
}
