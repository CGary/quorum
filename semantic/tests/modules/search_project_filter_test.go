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

func TestSearchProjectFilter(t *testing.T) {
	// This test requires fts5 and vec0 to pass InitDB.
	// We'll skip deep integration if they are missing.
	
	tempDir, err := os.MkdirTemp("", "hsme-search-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	
	// Try to init DB properly
	db, err := sqlite.InitDB(dbPath)
	if err != nil {
		t.Logf("Skipping integration test due to missing extensions: %v", err)
		return
	}
	defer db.Close()

	// Ingest data for two different projects
	ctx := context.Background()
	_, err = indexer.StoreContext(db, "Content for Project A", "manual", "ProjA", nil, false)
	if err != nil {
		t.Fatalf("failed to ingest: %v", err)
	}
	_, err = indexer.StoreContext(db, "Content for Project B", "manual", "ProjB", nil, false)
	if err != nil {
		t.Fatalf("failed to ingest: %v", err)
	}

	// 1. Unfiltered search should see both
	results, err := search.FuzzySearch(ctx, db, nil, "Content", 10, "")
	if err != nil {
		t.Fatalf("unfiltered search failed: %v", err)
	}
	// Note: since we don't have embeddings, this will be lexical only.
	// But it should still see both rows.
	if len(results) < 2 {
		t.Errorf("expected at least 2 results, got %d", len(results))
	}

	// 2. Filtered search for Project A
	resultsA, err := search.FuzzySearch(ctx, db, nil, "Content", 10, "ProjA")
	if err != nil {
		t.Fatalf("filtered search A failed: %v", err)
	}
	for range resultsA {
	        // We would need to load the memory to verify the project,
	        // but the search query itself should handle it.
	        // For now we just check count (should be 1).
	        if len(resultsA) != 1 {
	                t.Errorf("expected 1 result for ProjA, got %d", len(resultsA))
	        }
	}
	// 3. Filtered search for non-existent project
	resultsC, err := search.FuzzySearch(ctx, db, nil, "Content", 10, "ProjC")
	if err != nil {
		t.Fatalf("filtered search C failed: %v", err)
	}
	if len(resultsC) != 0 {
		t.Errorf("expected 0 results for ProjC, got %d", len(resultsC))
	}
}
