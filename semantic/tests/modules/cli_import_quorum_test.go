package modules

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLI_ImportQuorum(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hsme-cli-import-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	hsmeDBPath := filepath.Join(tmpDir, "hsme.db")
	quorumDBPath := filepath.Join(tmpDir, "quorum_memory.db")
	cliPath := filepath.Join(tmpDir, "hsme-cli")

	// 1. Build CLI binary
	cmd := exec.Command("go", "build", "-tags", "sqlite_fts5 sqlite_vec", "-o", cliPath, "./cmd/cli")
	cmd.Dir = "../.."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build hsme-cli: %v\nOutput: %s", err, out)
	}

	// 2. Setup Quorum curated memory DB fixture
	qDB := setupTestQuorumDB(t, quorumDBPath)
	_, err = qDB.Exec(`INSERT INTO memory_entries (project_id, id, type, source_task, title, context, content, created_at, content_hash, raw_json) VALUES
		('testproj', 'DEC-2026-01-01-001', 'decision', 'HSME-001', 'Title 1', 'Context 1', 'Content 1', '2026-01-01T00:00:00Z', 'hash1', '{}'),
		('testproj', 'PAT-2026-01-01-002', 'pattern', 'HSME-001', 'Title 2', 'Context 2', 'Content 2', '2026-01-01T00:00:00Z', 'hash2', '{}')
	`)
	if err != nil {
		t.Fatalf("failed to insert test memory_entries: %v", err)
	}
	qDB.Close()

	tasksRoot := filepath.Join("..", "..", "tests", "modules", "testdata", "capsule_scan")

	runCLI := func(args ...string) (string, string, error) {
		fullArgs := append([]string{"--db", hsmeDBPath}, args...)
		c := exec.Command(cliPath, fullArgs...)
		var stdout, stderr bytes.Buffer
		c.Stdout = &stdout
		c.Stderr = &stderr
		err := c.Run()
		return stdout.String(), stderr.String(), err
	}

	// (a) --source all end-to-end
	t.Run("SourceAll_EndToEnd", func(t *testing.T) {
		stdout, stderr, err := runCLI("--format", "json", "import-quorum", "--project", "testproj", "--quorum-db", quorumDBPath, "--tasks-root", tasksRoot, "--source", "all")
		if err != nil {
			t.Fatalf("import-quorum --source all failed: %v\nStderr: %s\nStdout: %s", err, stderr, stdout)
		}

		var res map[string]map[string]interface{}
		if err := json.Unmarshal([]byte(stdout), &res); err != nil {
			t.Fatalf("failed to parse JSON result: %v\nOutput: %s", err, stdout)
		}

		curated, ok := res["curated"]
		if !ok {
			t.Fatalf("missing 'curated' key in result: %v", res)
		}
		if curated["Errored"].(float64) != 0 {
			t.Errorf("expected 0 errored curated rows, got %v", curated["Errored"])
		}

		capsules, ok := res["capsules"]
		if !ok {
			t.Fatalf("missing 'capsules' key in result: %v", res)
		}
		if capsules["Errored"].(float64) != 0 {
			t.Errorf("expected 0 errored capsule rows, got %v", capsules["Errored"])
		}
	})

	// (b) Re-running produces zero new imported rows (dedup)
	t.Run("DedupOnReRun", func(t *testing.T) {
		stdout, stderr, err := runCLI("--format", "json", "import-quorum", "--project", "testproj", "--quorum-db", quorumDBPath, "--tasks-root", tasksRoot, "--source", "all")
		if err != nil {
			t.Fatalf("re-run import-quorum failed: %v\nStderr: %s", err, stderr)
		}

		var res map[string]map[string]interface{}
		if err := json.Unmarshal([]byte(stdout), &res); err != nil {
			t.Fatalf("failed to parse JSON result: %v", err)
		}

		if res["curated"]["Ingested"].(float64) != 0 {
			t.Errorf("expected 0 new ingested curated rows on re-run, got %v", res["curated"]["Ingested"])
		}
		if res["capsules"]["Ingested"].(float64) != 0 {
			t.Errorf("expected 0 new ingested capsule rows on re-run, got %v", res["capsules"]["Ingested"])
		}
	})

	// (c) --source curated only
	t.Run("SourceCuratedOnly", func(t *testing.T) {
		freshDB := filepath.Join(tmpDir, "hsme_curated_only.db")
		stdout, err := exec.Command(cliPath, "--db", freshDB, "--format", "json", "import-quorum", "--project", "testproj", "--quorum-db", quorumDBPath, "--source", "curated").CombinedOutput()
		if err != nil {
			t.Fatalf("import-quorum --source curated failed: %v\nOutput: %s", err, stdout)
		}
		var res map[string]interface{}
		if err := json.Unmarshal(stdout, &res); err != nil {
			t.Fatalf("failed to parse JSON: %v", err)
		}
		if _, ok := res["curated"]; !ok {
			t.Errorf("expected 'curated' key in result")
		}
		if _, ok := res["capsules"]; ok {
			t.Errorf("did not expect 'capsules' key when --source curated")
		}
	})

	// (d) --source capsules only
	t.Run("SourceCapsulesOnly", func(t *testing.T) {
		freshDB := filepath.Join(tmpDir, "hsme_capsules_only.db")
		stdout, err := exec.Command(cliPath, "--db", freshDB, "--format", "json", "import-quorum", "--project", "testproj", "--tasks-root", tasksRoot, "--source", "capsules").CombinedOutput()
		if err != nil {
			t.Fatalf("import-quorum --source capsules failed: %v\nOutput: %s", err, stdout)
		}
		var res map[string]interface{}
		if err := json.Unmarshal(stdout, &res); err != nil {
			t.Fatalf("failed to parse JSON: %v", err)
		}
		if _, ok := res["capsules"]; !ok {
			t.Errorf("expected 'capsules' key in result")
		}
		if _, ok := res["curated"]; ok {
			t.Errorf("did not expect 'curated' key when --source capsules")
		}
	})

	// (e) Missing --project fails usage
	t.Run("MissingProjectError", func(t *testing.T) {
		_, stderr, err := runCLI("import-quorum")
		if err == nil {
			t.Errorf("expected non-zero exit when --project is missing")
		}
		if !strings.Contains(stderr, "missing required flag: --project") {
			t.Errorf("expected missing required flag error message, got stderr: %s", stderr)
		}
	})

	// (f) Invalid --source fails usage
	t.Run("InvalidSourceError", func(t *testing.T) {
		_, stderr, err := runCLI("import-quorum", "--project", "testproj", "--source", "invalid")
		if err == nil {
			t.Errorf("expected non-zero exit when --source is invalid")
		}
		if !strings.Contains(stderr, "invalid source") {
			t.Errorf("expected invalid source error message, got stderr: %s", stderr)
		}
	})

	// (g) Default --quorum-db path resolution via $HOME
	t.Run("DefaultQuorumDBResolution", func(t *testing.T) {
		fakeHome := filepath.Join(tmpDir, "fakehome")
		quorumDir := filepath.Join(fakeHome, ".quorum")
		if err := os.MkdirAll(quorumDir, 0755); err != nil {
			t.Fatalf("failed to create fake .quorum dir: %v", err)
		}
		qHomeDB := setupTestQuorumDB(t, filepath.Join(quorumDir, "memory.db"))
		_, err = qHomeDB.Exec(`INSERT INTO memory_entries (project_id, id, type, source_task, title, context, content, created_at, content_hash, raw_json) VALUES
			('testproj', 'DEC-2026-01-01-099', 'decision', 'HSME-001', 'Home Title', 'Home Context', 'Home Content', '2026-01-01T00:00:00Z', 'homehash', '{}')
		`)
		if err != nil {
			t.Fatalf("failed to insert home memory_entries: %v", err)
		}
		qHomeDB.Close()

		c := exec.Command(cliPath, "--db", filepath.Join(tmpDir, "hsme_home_test.db"), "--format", "json", "import-quorum", "--project", "testproj", "--source", "curated")
		c.Env = append(os.Environ(), "HOME="+fakeHome)
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("import-quorum with default --quorum-db failed: %v\nOutput: %s", err, out)
		}
		var res map[string]map[string]interface{}
		if err := json.Unmarshal(out, &res); err != nil {
			t.Fatalf("failed to parse JSON result: %v", err)
		}
		if res["curated"]["Ingested"].(float64) == 0 {
			t.Errorf("expected ingested rows from default ~/.quorum/memory.db, got 0")
		}
	})

	// (h) `hsme-cli help import-quorum` documents ADR 0013 §4
	t.Run("HelpImportQuorumDocsADR0013", func(t *testing.T) {
		stdout, _, err := runCLI("help", "import-quorum")
		if err != nil {
			t.Fatalf("help import-quorum failed: %v", err)
		}
		if !strings.Contains(stdout, "ADR 0013 section 4") {
			t.Errorf("help import-quorum output missing ADR 0013 section 4 note, got: %s", stdout)
		}
	})
}
