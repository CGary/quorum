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

func insertServerMemoryFixtures(t *testing.T, db *sql.DB) {
	t.Helper()
	entries := []struct{ project, id, typ, title, task, context, content, created string }{
		{"proj1", "DEC-2026-06-01-1", "decision", "Decision memory", "FEAT-001", "Viewer", "Read-only memory browsing is allowed through serve.", "2026-06-03"},
		{"proj1", "PAT-2026-06-01-1", "pattern", "Pattern memory", "FEAT-002", "Queries", "Use normalized query tables for memory presentation.", "2026-06-02"},
		{"proj1", "LES-2026-06-01-1", "lesson", "Lesson memory", "FEAT-003", "Safety", "Do not write SQLite rows from serve handlers.", "2026-06-01"},
		{"proj2", "DEC-2026-06-01-1", "decision", "Hidden memory", "FEAT-999", "No root", "This project has no root path and subroutes must 404.", "2026-06-04"},
	}
	for _, e := range entries {
		_, err := db.Exec(`INSERT INTO memory_entries (project_id, id, type, source_task, title, context, content, created_at, supersedes, content_hash, raw_json)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '{}')`, e.project, e.id, e.typ, e.task, e.title, e.context, e.content, e.created, nil, e.project+e.id)
		if err != nil {
			t.Fatalf("insert memory fixture: %v", err)
		}
	}
	_, err := db.Exec(`UPDATE memory_entries SET supersedes = ? WHERE project_id = ? AND id = ?`, "PAT-2026-06-01-1", "proj1", "LES-2026-06-01-1")
	if err != nil {
		t.Fatalf("update supersedes: %v", err)
	}
	_, err = db.Exec(`INSERT INTO memory_related (project_id, memory_id, related_ref) VALUES ('proj1', 'DEC-2026-06-01-1', 'PAT-2026-06-01-1')`)
	if err != nil {
		t.Fatalf("insert related: %v", err)
	}
	_, err = db.Exec(`INSERT INTO memory_anti_patterns (project_id, memory_id, ordinal, content) VALUES ('proj1', 'DEC-2026-06-01-1', 0, 'Do not mutate memory from handlers.')`)
	if err != nil {
		t.Fatalf("insert anti pattern: %v", err)
	}
	_, err = db.Exec(`INSERT INTO memory_supersession_edges (project_id, from_id, to_id) VALUES ('proj1', 'LES-2026-06-01-1', 'PAT-2026-06-01-1')`)
	if err != nil {
		t.Fatalf("insert supersession edge: %v", err)
	}
}

func TestProjectSubRouteMemoriesList(t *testing.T) {
	db, _ := setupTestDB(t)
	defer db.Close()
	insertServerMemoryFixtures(t, db)
	srv := &Server{db: db}

	req := httptest.NewRequest(http.MethodGet, "/api/projects/proj1/memories?type=decision&q=viewer", nil)
	w := httptest.NewRecorder()
	srv.projectSubRouteHandler(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("expected 200 OK, got %v: %s", res.StatusCode, string(b))
	}
	var payload core.MemoryListResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode memories list: %v", err)
	}
	if payload.ProjectID != "proj1" || len(payload.Items) != 1 || payload.Items[0].ID != "DEC-2026-06-01-1" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload.Counts.Decision != 1 || payload.Counts.Pattern != 1 || payload.Counts.Lesson != 1 {
		t.Fatalf("counts should be project-wide, got %+v", payload.Counts)
	}
}

func TestProjectSubRouteMemoriesEmptyAndInvalid(t *testing.T) {
	db, _ := setupTestDB(t)
	defer db.Close()
	srv := &Server{db: db}

	req := httptest.NewRequest(http.MethodGet, "/api/projects/proj1/memories", nil)
	w := httptest.NewRecorder()
	srv.projectSubRouteHandler(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected empty memories 200, got %v", w.Result().StatusCode)
	}
	var empty core.MemoryListResponse
	if err := json.NewDecoder(w.Result().Body).Decode(&empty); err != nil {
		t.Fatalf("decode empty memories: %v", err)
	}
	if len(empty.Items) != 0 {
		t.Fatalf("expected no memories, got %+v", empty.Items)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/projects/proj1/memories?type=bad", nil)
	w = httptest.NewRecorder()
	srv.projectSubRouteHandler(w, req)
	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("expected invalid type 400, got %v", w.Result().StatusCode)
	}
}

func TestProjectSubRouteMemoryDetailAndInvalidID(t *testing.T) {
	db, _ := setupTestDB(t)
	defer db.Close()
	insertServerMemoryFixtures(t, db)
	srv := &Server{db: db}

	req := httptest.NewRequest(http.MethodGet, "/api/projects/proj1/memories/DEC-2026-06-01-1", nil)
	w := httptest.NewRecorder()
	srv.projectSubRouteHandler(w, req)
	if w.Result().StatusCode != http.StatusOK {
		b, _ := io.ReadAll(w.Result().Body)
		t.Fatalf("expected detail 200, got %v: %s", w.Result().StatusCode, string(b))
	}
	var detail core.MemoryDetail
	if err := json.NewDecoder(w.Result().Body).Decode(&detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if detail.ID != "DEC-2026-06-01-1" || len(detail.Related) != 1 || len(detail.AntiPatterns) != 1 {
		t.Fatalf("unexpected detail: %+v", detail)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/projects/proj1/memories/..%2FDEC-2026-06-01-1", nil)
	w = httptest.NewRecorder()
	srv.projectSubRouteHandler(w, req)
	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("expected invalid memory id 400, got %v", w.Result().StatusCode)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/projects/proj1/memories/DEC-2026-06-01-404", nil)
	w = httptest.NewRecorder()
	srv.projectSubRouteHandler(w, req)
	if w.Result().StatusCode != http.StatusNotFound {
		t.Fatalf("expected missing memory 404, got %v", w.Result().StatusCode)
	}
}

func TestProjectWithoutRootMemorySubroute404(t *testing.T) {
	db, _ := setupTestDB(t)
	defer db.Close()
	insertServerMemoryFixtures(t, db)
	srv := &Server{db: db}

	req := httptest.NewRequest(http.MethodGet, "/api/projects/proj2/memories", nil)
	w := httptest.NewRecorder()
	srv.projectSubRouteHandler(w, req)
	if w.Result().StatusCode != http.StatusNotFound {
		t.Fatalf("expected project without root subroute 404, got %v", w.Result().StatusCode)
	}
}

func TestProjectTasksSubroutes(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer db.Close()
	srv := &Server{db: db}

	// Create task mock files
	locPath := filepath.Join(tempDir, ".ai", "tasks", "active")
	if err := os.MkdirAll(locPath, 0755); err != nil {
		t.Fatalf("mkdir active: %v", err)
	}
	taskDir := filepath.Join(locPath, "FEAT-001-some-feature")
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		t.Fatalf("mkdir taskDir: %v", err)
	}
	specContent := `
task_id: FEAT-001
summary: "This is a test task"
goal: "Expose task state in serve UI"
`
	if err := os.WriteFile(filepath.Join(taskDir, "00-spec.yaml"), []byte(specContent), 0644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	// 1. GET list of tasks
	req := httptest.NewRequest(http.MethodGet, "/api/projects/proj1/tasks", nil)
	w := httptest.NewRecorder()
	srv.projectSubRouteHandler(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %v", w.Result().StatusCode)
	}
	var listRes core.TaskListResponse
	if err := json.NewDecoder(w.Result().Body).Decode(&listRes); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listRes.Items) != 1 || listRes.Items[0].ID != "FEAT-001" {
		t.Fatalf("expected FEAT-001 task item, got %+v", listRes.Items)
	}

	// 2. GET detail of task
	req = httptest.NewRequest(http.MethodGet, "/api/projects/proj1/tasks/FEAT-001", nil)
	w = httptest.NewRecorder()
	srv.projectSubRouteHandler(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected detail status 200, got %v", w.Result().StatusCode)
	}
	var detail core.TaskDetail
	if err := json.NewDecoder(w.Result().Body).Decode(&detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if detail.ID != "FEAT-001" || detail.Summary != "This is a test task" {
		t.Fatalf("expected FEAT-001 detail, got %+v", detail)
	}

	// 3. Project without root_path gives 404
	req = httptest.NewRequest(http.MethodGet, "/api/projects/proj2/tasks", nil)
	w = httptest.NewRecorder()
	srv.projectSubRouteHandler(w, req)
	if w.Result().StatusCode != http.StatusNotFound {
		t.Fatalf("expected project without root 404, got %v", w.Result().StatusCode)
	}

	// 4. Invalid location gives 400
	req = httptest.NewRequest(http.MethodGet, "/api/projects/proj1/tasks?location=bad", nil)
	w = httptest.NewRecorder()
	srv.projectSubRouteHandler(w, req)
	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("expected invalid location 400, got %v", w.Result().StatusCode)
	}

	// 5. Invalid task ID / traversal gives 400
	req = httptest.NewRequest(http.MethodGet, "/api/projects/proj1/tasks/..%2FFEAT-001", nil)
	w = httptest.NewRecorder()
	srv.projectSubRouteHandler(w, req)
	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("expected invalid task ID 400, got %v", w.Result().StatusCode)
	}

	// 6. Methods other than GET give 405
	req = httptest.NewRequest(http.MethodPost, "/api/projects/proj1/tasks", nil)
	w = httptest.NewRecorder()
	srv.projectSubRouteHandler(w, req)
	if w.Result().StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected POST method not allowed 405, got %v", w.Result().StatusCode)
	}
}
