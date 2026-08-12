package modules

import (
	"bytes"
	"database/sql"
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
		stdout, stderr, err := runCLI("import-quorum", "--project", "testproj", "--quorum-db", quorumDBPath, "--tasks-root", tasksRoot, "--source", "all", "--json")
		if err != nil {
			t.Fatalf("import-quorum --source all failed: %v\nStderr: %s\nStdout: %s", err, stderr, stdout)
		}

		var res map[string]any
		if err := json.Unmarshal([]byte(stdout), &res); err != nil {
			t.Fatalf("failed to parse JSON result: %v\nOutput: %s", err, stdout)
		}

		if res["ok"] != true {
			t.Errorf("expected ok: true, got %v", res["ok"])
		}
		if res["command"] != "import-quorum" {
			t.Errorf("expected command 'import-quorum', got %v", res["command"])
		}

		data, ok := res["data"].(map[string]any)
		if !ok {
			t.Fatalf("missing data object in result: %v", res)
		}

		curated, ok := data["curated"].(map[string]any)
		if !ok {
			t.Fatalf("missing 'curated' key in data: %v", data)
		}
		if curated["Errored"].(float64) != 0 {
			t.Errorf("expected 0 errored curated rows, got %v", curated["Errored"])
		}

		capsules, ok := data["capsules"].(map[string]any)
		if !ok {
			t.Fatalf("missing 'capsules' key in data: %v", data)
		}
		if capsules["Errored"].(float64) != 0 {
			t.Errorf("expected 0 errored capsule rows, got %v", capsules["Errored"])
		}
	})

	// (b) Re-running produces zero new imported rows (dedup)
	t.Run("DedupOnReRun", func(t *testing.T) {
		stdout, stderr, err := runCLI("import-quorum", "--project", "testproj", "--quorum-db", quorumDBPath, "--tasks-root", tasksRoot, "--source", "all", "--json")
		if err != nil {
			t.Fatalf("re-run import-quorum failed: %v\nStderr: %s", err, stderr)
		}

		var res map[string]any
		if err := json.Unmarshal([]byte(stdout), &res); err != nil {
			t.Fatalf("failed to parse JSON result: %v", err)
		}

		data := res["data"].(map[string]any)
		curated := data["curated"].(map[string]any)
		capsules := data["capsules"].(map[string]any)

		if curated["Ingested"].(float64) != 0 {
			t.Errorf("expected 0 new ingested curated rows on re-run, got %v", curated["Ingested"])
		}
		if capsules["Ingested"].(float64) != 0 {
			t.Errorf("expected 0 new ingested capsule rows on re-run, got %v", capsules["Ingested"])
		}
	})

	// (c) --source curated only
	t.Run("SourceCuratedOnly", func(t *testing.T) {
		freshDB := filepath.Join(tmpDir, "hsme_curated_only.db")
		stdout, err := exec.Command(cliPath, "--db", freshDB, "import-quorum", "--project", "testproj", "--quorum-db", quorumDBPath, "--source", "curated", "--json").CombinedOutput()
		if err != nil {
			t.Fatalf("import-quorum --source curated failed: %v\nOutput: %s", err, stdout)
		}
		var res map[string]any
		if err := json.Unmarshal(stdout, &res); err != nil {
			t.Fatalf("failed to parse JSON: %v", err)
		}
		data := res["data"].(map[string]any)
		if _, ok := data["curated"]; !ok {
			t.Errorf("expected 'curated' key in data")
		}
		if _, ok := data["capsules"]; ok {
			t.Errorf("did not expect 'capsules' key when --source curated")
		}
	})

	// (d) --source capsules only
	t.Run("SourceCapsulesOnly", func(t *testing.T) {
		freshDB := filepath.Join(tmpDir, "hsme_capsules_only.db")
		stdout, err := exec.Command(cliPath, "--db", freshDB, "import-quorum", "--project", "testproj", "--tasks-root", tasksRoot, "--source", "capsules", "--json").CombinedOutput()
		if err != nil {
			t.Fatalf("import-quorum --source capsules failed: %v\nOutput: %s", err, stdout)
		}
		var res map[string]any
		if err := json.Unmarshal(stdout, &res); err != nil {
			t.Fatalf("failed to parse JSON: %v", err)
		}
		data := res["data"].(map[string]any)
		if _, ok := data["capsules"]; !ok {
			t.Errorf("expected 'capsules' key in data")
		}
		if _, ok := data["curated"]; ok {
			t.Errorf("did not expect 'curated' key when --source capsules")
		}
	})

	// (e) Missing --project fails usage
	t.Run("MissingProjectError", func(t *testing.T) {
		stdout, _, err := runCLI("import-quorum", "--json")
		if err == nil {
			t.Errorf("expected non-zero exit when --project is missing")
		}
		var res map[string]any
		if err := json.Unmarshal([]byte(stdout), &res); err != nil {
			t.Fatalf("failed to parse JSON error: %v, stdout: %s", err, stdout)
		}
		if res["ok"] != false {
			t.Errorf("expected ok: false, got %v", res["ok"])
		}
		errObj := res["error"].(map[string]any)
		if errObj["code"] != "MISSING_REQUIRED_FLAG" {
			t.Errorf("expected code MISSING_REQUIRED_FLAG, got %v", errObj["code"])
		}
	})

	// (f) Invalid --source fails usage
	t.Run("InvalidSourceError", func(t *testing.T) {
		stdout, _, err := runCLI("import-quorum", "--project", "testproj", "--source", "invalid", "--json")
		if err == nil {
			t.Errorf("expected non-zero exit when --source is invalid")
		}
		var res map[string]any
		if err := json.Unmarshal([]byte(stdout), &res); err != nil {
			t.Fatalf("failed to parse JSON error: %v, stdout: %s", err, stdout)
		}
		if res["ok"] != false {
			t.Errorf("expected ok: false, got %v", res["ok"])
		}
		errObj := res["error"].(map[string]any)
		if errObj["code"] != "INVALID_ENUM" {
			t.Errorf("expected code INVALID_ENUM, got %v", errObj["code"])
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

		c := exec.Command(cliPath, "--db", filepath.Join(tmpDir, "hsme_home_test.db"), "import-quorum", "--project", "testproj", "--source", "curated", "--json")
		c.Env = append(os.Environ(), "HOME="+fakeHome)
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("import-quorum with default --quorum-db failed: %v\nOutput: %s", err, out)
		}
		var res map[string]any
		if err := json.Unmarshal(out, &res); err != nil {
			t.Fatalf("failed to parse JSON result: %v", err)
		}
		data := res["data"].(map[string]any)
		curated := data["curated"].(map[string]any)
		if curated["Ingested"].(float64) == 0 {
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

	// (i) --dry-run
	t.Run("DryRun", func(t *testing.T) {
		stdout, stderr, err := runCLI("import-quorum", "--project", "testproj", "--quorum-db", quorumDBPath, "--dry-run", "--json")
		if err != nil {
			t.Fatalf("import-quorum --dry-run failed: %v\nStderr: %s\nStdout: %s", err, stderr, stdout)
		}
		var res map[string]any
		if err := json.Unmarshal([]byte(stdout), &res); err != nil {
			t.Fatalf("failed to parse JSON: %v", err)
		}
		if res["ok"] != true {
			t.Errorf("expected ok: true, got %v", res["ok"])
		}
		data := res["data"].(map[string]any)
		if data["dry_run"] != true {
			t.Errorf("expected data.dry_run: true, got %v", data["dry_run"])
		}
	})

	// (j) --schema
	t.Run("Schema", func(t *testing.T) {
		stdout, stderr, err := runCLI("import-quorum", "--schema")
		if err != nil {
			t.Fatalf("import-quorum --schema failed: %v\nStderr: %s\nStdout: %s", err, stderr, stdout)
		}
		var schema map[string]any
		if err := json.Unmarshal([]byte(stdout), &schema); err != nil {
			t.Fatalf("failed to parse schema JSON: %v", err)
		}
		if schema["command"] != "import-quorum" {
			t.Errorf("expected command 'import-quorum', got %v", schema["command"])
		}
	})

	// (k) --project '*' all-projects import
	t.Run("AllProjectsImport", func(t *testing.T) {
		multiQDBPath := filepath.Join(tmpDir, "multi_quorum.db")
		mqDB := setupTestQuorumDB(t, multiQDBPath)
		if _, err := mqDB.Exec(`INSERT INTO projects (id) VALUES ('proja'), ('projb');`); err != nil {
			mqDB.Close()
			t.Fatalf("failed to insert projects: %v", err)
		}
		_, err := mqDB.Exec(`INSERT INTO memory_entries (project_id, id, type, source_task, title, context, content, created_at, content_hash, raw_json) VALUES
			('proja', 'DEC-2026-01-01-001', 'decision', 'HSME-007', 'Title A', 'Context A', 'Content A long enough', '2026-01-01T00:00:00Z', 'hasha', '{}'),
			('projb', 'PAT-2026-01-01-002', 'pattern', 'HSME-007', 'Title B', 'Context B', 'Content B long enough', '2026-01-01T00:00:00Z', 'hashb', '{}')
		`)
		if err != nil {
			mqDB.Close()
			t.Fatalf("failed to insert multi-project entries: %v", err)
		}
		mqDB.Close()

		allProjHSMEDB := filepath.Join(tmpDir, "hsme_all_proj.db")
		cmd := exec.Command(cliPath, "--db", allProjHSMEDB, "import-quorum", "--project", "*", "--quorum-db", multiQDBPath, "--source", "curated", "--json")
		stdout, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("import-quorum --project '*' failed: %v\nOutput: %s", err, stdout)
		}

		var res map[string]any
		if err := json.Unmarshal(stdout, &res); err != nil {
			t.Fatalf("failed to parse JSON result: %v", err)
		}
		data := res["data"].(map[string]any)
		curated := data["curated"].(map[string]any)
		if curated["Ingested"].(float64) != 2 {
			t.Errorf("expected 2 ingested curated rows for '*', got %v", curated["Ingested"])
		}

		hDB, err := sql.Open("sqlite3", allProjHSMEDB)
		if err != nil {
			t.Fatalf("failed to open resulting HSME DB: %v", err)
		}
		defer hDB.Close()

		rows, err := hDB.Query("SELECT project FROM memories ORDER BY project ASC")
		if err != nil {
			t.Fatalf("failed to query memories: %v", err)
		}
		defer rows.Close()

		var projs []string
		for rows.Next() {
			var p string
			if err := rows.Scan(&p); err != nil {
				t.Fatalf("failed to scan memory project: %v", err)
			}
			projs = append(projs, p)
		}
		if len(projs) != 2 || projs[0] != "proja" || projs[1] != "projb" {
			t.Errorf("expected projects ['proja', 'projb'], got %v", projs)
		}
	})
}
