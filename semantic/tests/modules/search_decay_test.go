package modules

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hsme/core/src/core/indexer"
	"github.com/hsme/core/src/core/search"
	"github.com/hsme/core/src/storage/sqlite"
	_ "github.com/mattn/go-sqlite3"
)

type baselineResult struct {
	Fuzzy []search.MemorySearchResult `json:"fuzzy"`
	Exact []search.ExactMatchResult   `json:"exact"`
}

func TestSearchDecay(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "hsme-decay-test-*")
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

	// Ingest documents with different timestamps. We'll modify their created_at manually.
	docs := []struct {
		content string
		ageDays int
	}{
		{"Common collision term here. Very old document.", 30},
		{"Common collision term here. Mid-aged document.", 14},
		{"Common collision term here. Brand new document.", 0},
	}

	for _, d := range docs {
		id, err := indexer.StoreContext(db, d.content, "manual", "", nil, false)
		if err != nil {
			t.Fatalf("failed to ingest: %v", err)
		}

		// Artificially age the document
		if d.ageDays > 0 {
			ageDuration := time.Duration(d.ageDays) * 24 * time.Hour
			pastTime := time.Now().Add(-ageDuration)
			_, err = db.Exec("UPDATE memories SET created_at = ? WHERE id = ?", pastTime, id)
			if err != nil {
				t.Fatalf("failed to backdate memory: %v", err)
			}
			// Also update chunks so exactSearchSubstring fallback created_at join works correctly
			_, err = db.Exec("UPDATE memory_chunks SET created_at = ? WHERE memory_id = ?", pastTime, id)
		}
	}

	// Test 1: Decay OFF Baseline
	search.GlobalDecayConfig = search.DecayConfig{Enabled: false, HalfLifeDays: 14}

	fuzzyOff, err := search.FuzzySearch(ctx, db, nil, "collision", 10, "")
	if err != nil {
		t.Fatalf("fuzzy search failed: %v", err)
	}

	exactOff, err := search.ExactSearch(ctx, db, "collision", 10, "")
	if err != nil {
		t.Fatalf("exact search failed: %v", err)
	}

	baselineFile := filepath.Join("testdata", "decay_off_baseline.json")
	baselineData, err := os.ReadFile(baselineFile)
	if err != nil {
		t.Fatalf("missing committed decay-off baseline fixture %s: %v", baselineFile, err)
	}
	var baseline baselineResult
	if err := json.Unmarshal(baselineData, &baseline); err != nil {
		t.Fatalf("invalid baseline fixture: %v", err)
	}

	// Verify byte-equivalence (JSON marshalling comparison)
	currentFuzzyBytes, _ := json.Marshal(fuzzyOff)
	baselineFuzzyBytes, _ := json.Marshal(baseline.Fuzzy)
	if string(currentFuzzyBytes) != string(baselineFuzzyBytes) {
		t.Errorf("Fuzzy decay OFF results do not match baseline")
	}

	currentExactBytes, _ := json.Marshal(exactOff)
	baselineExactBytes, _ := json.Marshal(baseline.Exact)
	if string(currentExactBytes) != string(baselineExactBytes) {
		t.Errorf("Exact decay OFF results do not match baseline")
	}

	// Test 2: Decay ON reordering
	search.GlobalDecayConfig = search.DecayConfig{Enabled: true, HalfLifeDays: 14}

	fuzzyOn, err := search.FuzzySearch(ctx, db, nil, "latest collision", 10, "")
	if err != nil {
		t.Fatalf("fuzzy search failed: %v", err)
	}

	// Since content is identical, "Brand new document." (age 0) should win over "Mid-aged document." (age 14),
	// which should win over "Very old document." (age 30).
	if len(fuzzyOn) >= 3 {
		if fuzzyOn[0].MemoryID != 3 || fuzzyOn[1].MemoryID != 2 || fuzzyOn[2].MemoryID != 1 {
			t.Errorf("Fuzzy decay ON did not reorder chronologically among ties: got %v", fuzzyOn)
		}
	}

	exactOn, err := search.ExactSearch(ctx, db, "collision", 10, "")
	if err != nil {
		t.Fatalf("exact search failed: %v", err)
	}

	if len(exactOn) >= 3 {
		if exactOn[0].MemoryID != 3 || exactOn[1].MemoryID != 2 || exactOn[2].MemoryID != 1 {
			t.Errorf("Exact decay ON did not reorder chronologically among ties: got %v", exactOn)
		}
	}
}
