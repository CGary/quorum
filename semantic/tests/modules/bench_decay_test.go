package modules

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/hsme/core/src/core/indexer"
	"github.com/hsme/core/src/storage/sqlite"
)

func TestBenchDecayCLI(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "bench-decay-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cmdPath := filepath.Join(tempDir, "bench-decay")
	buildCmd := exec.Command("go", "build", "-tags", "sqlite_fts5 sqlite_vec", "-o", cmdPath, "../../cmd/bench-decay")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build bench-decay: %v\nOutput: %s", err, string(out))
	}

	t.Run("InvalidHalfLife", func(t *testing.T) {
		cmd := exec.Command(cmdPath, "-half-life", "-5")
		if err := cmd.Run(); err == nil {
			t.Errorf("expected bench-decay to fail with invalid half-life")
		}
	})

	t.Run("FrozenEvalRunWritesReports", func(t *testing.T) {
		dbPath := filepath.Join(tempDir, "bench.db")
		db, err := sqlite.InitDB(dbPath)
		if err != nil {
			t.Skipf("skipping bench smoke test due to sqlite init failure: %v", err)
		}
		if _, err := indexer.StoreContext(db, "alpha collision fresh document", "manual", "bench", nil, false); err != nil {
			t.Fatalf("failed to store context: %v", err)
		}
		if _, err := indexer.StoreContext(db, "beta collision older document", "manual", "bench", nil, false); err != nil {
			t.Fatalf("failed to store second context: %v", err)
		}
		db.Close()

		evalPath := filepath.Join(tempDir, "eval.json")
		baselinePath := filepath.Join(tempDir, "baseline.json")
		writeFile(t, evalPath, `{
  "schema_version": 1,
  "frozen_at": "test",
  "total_queries": 2,
  "queries": [
    {"id":"q1","category":"pure_recency","query":"alpha","expected_winner_criterion":{"resolved_memory_id":1}},
    {"id":"q2","category":"pure_relevance","query":"collision","expected_winner_criterion":{"resolved_memory_id":2}}
  ]
}`)
		writeFile(t, baselinePath, `{
  "schema_version": 1,
  "measured_at": "test",
  "total_queries": 2,
  "results": [
    {"id":"q1","actual_top_10_ids":[1],"expected_winner_id":1},
    {"id":"q2","actual_top_10_ids":[1,2],"expected_winner_id":2}
  ]
}`)

		outDir := filepath.Join(tempDir, "reports")
		cmd := exec.Command(cmdPath,
			"-db", dbPath,
			"-eval", evalPath,
			"-baseline", baselinePath,
			"-out", outDir,
			"-run-id", "testrun",
			"-no-vector",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("expected benchmark run to succeed: %v\nOutput: %s", err, string(out))
		}

		reportPath := filepath.Join(outDir, "testrun", "report.json")
		deltaPath := filepath.Join(outDir, "testrun", "delta.json")
		mdPath := filepath.Join(outDir, "testrun", "report.md")
		for _, path := range []string{reportPath, deltaPath, mdPath} {
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("expected report file %s: %v", path, err)
			}
		}

		var report struct {
			EvalTotal    int               `json:"eval_total"`
			ExactSamples []json.RawMessage `json:"exact_samples"`
			Deltas       []json.RawMessage `json:"deltas"`
		}
		b, err := os.ReadFile(reportPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(b, &report); err != nil {
			t.Fatalf("invalid report json: %v", err)
		}
		if report.EvalTotal != 2 || len(report.ExactSamples) != 2 || len(report.Deltas) != 2 {
			t.Fatalf("unexpected report shape: %+v", report)
		}
	})
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}
