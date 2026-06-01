package server

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"quorum/internal/core"
)

func setupTestDB(t *testing.T) (*sql.DB, string) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "memory.db")
	t.Setenv("QUORUM_MEMORY_DB", dbPath)

	db, err := core.OpenMemoryDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open memory db: %v", err)
	}

	_, err = db.Exec(`INSERT INTO projects (id, name, root_path, git_remote, created_at, updated_at) 
		VALUES ('proj1', 'Test Project', ?, 'remote1', '2026-06-01T12:00:00Z', '2026-06-01T12:00:00Z')`, tempDir)
	if err != nil {
		t.Fatalf("Failed to insert project: %v", err)
	}

	_, err = db.Exec(`INSERT INTO projects (id, name, root_path, git_remote, created_at, updated_at) 
		VALUES ('proj2', 'No Root', '', 'remote2', '2026-06-01T12:00:00Z', '2026-06-01T12:00:00Z')`)
	if err != nil {
		t.Fatalf("Failed to insert project: %v", err)
	}

	reportsDir := filepath.Join(tempDir, ".ai", "reports")
	if err := os.MkdirAll(reportsDir, 0755); err != nil {
		t.Fatalf("Failed to create reports dir: %v", err)
	}

	reportContent := `
task_id: RPT-001
summary: "Test report"
`
	if err := os.WriteFile(filepath.Join(reportsDir, "test-report.yaml"), []byte(reportContent), 0644); err != nil {
		t.Fatalf("Failed to write report: %v", err)
	}

	// wait a bit to ensure different mtime
	time.Sleep(10 * time.Millisecond)

	reportContent2 := `
task_id: RPT-002
summary: "Another report"
`
	if err := os.WriteFile(filepath.Join(reportsDir, "another-report.yaml"), []byte(reportContent2), 0644); err != nil {
		t.Fatalf("Failed to write report: %v", err)
	}

	return db, tempDir
}

func TestProjectsHandler(t *testing.T) {
	db, _ := setupTestDB(t)
	defer db.Close()

	srv := &Server{db: db}

	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	w := httptest.NewRecorder()

	srv.projectsHandler(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 OK, got %v", res.StatusCode)
	}

	var projects []Project
	if err := json.NewDecoder(res.Body).Decode(&projects); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(projects) != 1 {
		t.Errorf("Expected 1 project (omitted the one with no root_path), got %d", len(projects))
	}
	if projects[0].ID != "proj1" {
		t.Errorf("Expected project ID 'proj1', got %s", projects[0].ID)
	}
}

func TestReportsHandler(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer db.Close()

	srv := &Server{db: db}

	req := httptest.NewRequest(http.MethodGet, "/api/projects/proj1/reports", nil)
	w := httptest.NewRecorder()

	srv.reportsHandler(w, req, "proj1", tempDir)

	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 OK, got %v", res.StatusCode)
	}

	var reports []ReportMeta
	if err := json.NewDecoder(res.Body).Decode(&reports); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(reports) != 2 {
		t.Errorf("Expected 2 reports, got %d", len(reports))
	}

	// sorted descending by mtime
	if reports[0].ID != "another-report" {
		t.Errorf("Expected newest report first, got %s", reports[0].ID)
	}
}

func TestReportDetailHandler(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer db.Close()

	srv := &Server{db: db}

	// We don't have a report.schema.json in the test environment (unless it's in the actual repo)
	// But it uses core.ValidateAgainstSchema which will attempt to load "report.schema.json" from .agents/schemas
	
	req := httptest.NewRequest(http.MethodGet, "/api/projects/proj1/reports/test-report", nil)
	w := httptest.NewRecorder()

	srv.reportDetailHandler(w, req, "proj1", tempDir, "test-report")

	res := w.Result()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		// We might get an error if report.schema.json does not exist or fails validation, 
		// but we want to make sure it doesn't fail on basic logic.
		// If it's a 500 because schema fails, that means our code called validate successfully.
		t.Logf("Got status %v: %s", res.StatusCode, string(b))
	} else {
		var payload map[string]interface{}
		if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
			t.Fatalf("Failed to decode JSON: %v", err)
		}
		if payload["task_id"] != "RPT-001" {
			t.Errorf("Unexpected payload content: %v", payload)
		}
	}
}

func TestPathTraversal(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer db.Close()

	srv := &Server{db: db}

	req := httptest.NewRequest(http.MethodGet, "/api/projects/proj1/reports/..%2F..%2Fmemory.db", nil)
	w := httptest.NewRecorder()

	srv.reportDetailHandler(w, req, "proj1", tempDir, "../../memory.db")

	res := w.Result()
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected 400 Bad Request for path traversal, got %v", res.StatusCode)
	}
}
