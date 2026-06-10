package modules

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hsme/core/src/core/indexer"
	"github.com/hsme/core/src/core/search"
	"github.com/hsme/core/src/storage/sqlite"
	_ "github.com/mattn/go-sqlite3"
)

func TestRecallRecentSession(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "hsme-recall-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")

	db, err := sqlite.InitDB(dbPath)
	if err != nil {
		t.Logf("Skipping test due to missing FTS5/vec0: %v", err)
		return
	}
	defer db.Close()

	ctx := context.Background()

	// Ingest session summaries
	id1, _ := indexer.StoreContext(db, "Summary 1", "session_summary", "ProjA", nil, false)
	indexer.StoreContext(db, "Manual note", "manual", "ProjA", nil, false)
	_, _ = indexer.StoreContext(db, "Summary 2", "session_summary", "ProjB", nil, false)
	_, _ = indexer.StoreContext(db, "Summary 3", "session_summary", "ProjA", nil, false)
	
	// Superseded memory
	indexer.StoreContext(db, "Summary 4 (new)", "session_summary", "ProjA", &id1, true)

	// 1. Basic chronological recall (no project)
	results, err := search.RecallRecentSession(ctx, db, 10, "")
	if err != nil {
		t.Fatalf("recall failed: %v", err)
	}
	// Should exclude id1 (superseded) and the manual note.
	// Order should be id4 (which superseded id1), id3, id2.
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}

	// 2. Project filter
	resultsProjA, err := search.RecallRecentSession(ctx, db, 10, "ProjA")
	if err != nil {
		t.Fatalf("recall ProjA failed: %v", err)
	}
	if len(resultsProjA) != 2 {
		// id4 and id3
		t.Errorf("expected 2 results for ProjA, got %d", len(resultsProjA))
	}

	// 3. Limit capping
	resultsLimit, err := search.RecallRecentSession(ctx, db, 1, "")
	if err != nil {
		t.Fatalf("recall limit failed: %v", err)
	}
	if len(resultsLimit) != 1 {
		t.Errorf("expected 1 result, got %d", len(resultsLimit))
	}
	// The most recent should be id4
	if resultsLimit[0].Highlights[0].Text != "Summary 4 (new)" {
		t.Errorf("expected newest summary, got %s", resultsLimit[0].Highlights[0].Text)
	}

	// 4. Over cap (request 100, should be capped at 50, but we only have 3 items)
	resultsMax, err := search.RecallRecentSession(ctx, db, 100, "")
	if err != nil {
		t.Fatalf("recall limit failed: %v", err)
	}
	if len(resultsMax) != 3 {
		t.Errorf("expected 3 results, got %d", len(resultsMax))
	}
}
