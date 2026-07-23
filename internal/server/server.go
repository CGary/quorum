package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"quorum/internal/core"
)

type Server struct {
	db          *sql.DB
	projectRoot string

	// bindHost/bindPort/fleetToken/loopbackBind back the FLEET-026 hardening
	// of POST /api/fleet/toggle (see fleet_security.go). They are populated
	// by Start; a zero-value Server (as constructed directly in tests) is
	// treated as an unconfigured/loopback bind by fleetSecurityConfig.
	bindHost     string
	bindPort     int
	fleetToken   string
	loopbackBind bool
}

func NewServer() (*Server, error) {
	db, err := core.OpenMemoryDB("")
	if err != nil {
		return nil, err
	}
	// core.ProjectRoot() failure must never fail NewServer(): the multi-project
	// report/task/memory viewer must keep working when quorum serve is started
	// outside a git-tracked directory. Only the new fleet endpoints degrade (503).
	root, _ := core.ProjectRoot()
	return &Server{db: db, projectRoot: root}, nil
}

func (s *Server) Start(host string, port int) error {
	if host == "" {
		host = "127.0.0.1"
	}
	s.bindHost = host
	s.bindPort = port
	s.loopbackBind = isLoopbackHost(host)
	if !s.loopbackBind {
		token, err := newFleetToken()
		if err != nil {
			return fmt.Errorf("generate fleet toggle token: %w", err)
		}
		s.fleetToken = token
		log.Printf("Fleet toggle token (required as X-Quorum-Fleet-Token on this non-loopback bind): %s", token)
	}
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	mux := http.NewServeMux()

	mux.HandleFunc("/api/projects", s.projectsHandler)
	mux.HandleFunc("/api/projects/", s.projectSubRouteHandler)

	mux.HandleFunc("/fleet", s.fleetPageHandler)
	mux.HandleFunc("/api/fleet/status", s.fleetStatusHandler)
	mux.HandleFunc("/api/fleet/dispatches", s.fleetDispatchesHandler)
	mux.HandleFunc("/api/fleet/toggle", s.fleetToggleHandler)

	s.MountEmbeddedViewer(mux)

	log.Printf("Starting read-only Quorum server on %s", addr)
	return http.ListenAndServe(addr, mux)
}

func (s *Server) MountEmbeddedViewer(mux *http.ServeMux) {
	mux.Handle("/", AssetHandler())
}

type Project struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	RootPath  string `json:"root_path"`
	GitRemote string `json:"git_remote"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func (s *Server) projectsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rows, err := s.db.Query("SELECT id, name, COALESCE(root_path, ''), COALESCE(git_remote, ''), created_at, updated_at FROM projects")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.RootPath, &p.GitRemote, &p.CreatedAt, &p.UpdatedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if p.RootPath == "" {
			log.Printf("Warning: Omitted project %s lacking valid root_path", p.ID)
			continue
		}
		projects = append(projects, p)
	}

	if projects == nil {
		projects = []Project{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(projects)
}

func (s *Server) projectSubRouteHandler(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(r.URL.Path, "..") {
		http.Error(w, "Invalid path: traversal detected", http.StatusBadRequest)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/projects/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || (parts[1] != "reports" && parts[1] != "memories" && parts[1] != "tasks") || parts[0] == "" {
		http.NotFound(w, r)
		return
	}

	projectID := parts[0]
	var rootPath string
	err := s.db.QueryRow("SELECT COALESCE(root_path, ''), name FROM projects WHERE id = ?", projectID).Scan(&rootPath, new(string)) // scan both to match query or keep it simple
	if err == sql.ErrNoRows {
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if rootPath == "" {
		http.Error(w, "Project has no valid root_path", http.StatusNotFound)
		return
	}

	if parts[1] == "reports" {
		if len(parts) == 2 {
			s.reportsHandler(w, r, projectID, rootPath)
		} else if len(parts) == 3 {
			reportID := parts[2]
			s.reportDetailHandler(w, r, projectID, rootPath, reportID)
		} else {
			http.NotFound(w, r)
		}
		return
	}

	if parts[1] == "tasks" {
		if len(parts) == 2 {
			s.tasksHandler(w, r, projectID, rootPath)
		} else if len(parts) == 3 {
			taskID := parts[2]
			s.taskDetailHandler(w, r, projectID, rootPath, taskID)
		} else {
			http.NotFound(w, r)
		}
		return
	}

	if len(parts) == 2 {
		s.memoriesHandler(w, r, projectID)
	} else if len(parts) == 3 {
		s.memoryDetailHandler(w, r, projectID, parts[2])
	} else {
		http.Error(w, "Invalid memory path", http.StatusBadRequest)
	}
}

func (s *Server) tasksHandler(w http.ResponseWriter, r *http.Request, projectID, rootPath string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limit, err := parseOptionalInt(r.URL.Query().Get("limit"), 0)
	if err != nil {
		http.Error(w, "Invalid limit", http.StatusBadRequest)
		return
	}
	offset, err := parseOptionalInt(r.URL.Query().Get("offset"), 0)
	if err != nil {
		http.Error(w, "Invalid offset", http.StatusBadRequest)
		return
	}

	res, err := core.QueryTasks(core.TaskListOptions{
		ProjectRoot: rootPath,
		Location:    r.URL.Query().Get("location"),
		Query:       r.URL.Query().Get("q"),
		ParentTask:  r.URL.Query().Get("parent_task"),
		Limit:       limit,
		Offset:      offset,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func (s *Server) taskDetailHandler(w http.ResponseWriter, r *http.Request, projectID, rootPath, taskID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := core.ValidateTaskID(taskID); err != nil {
		http.Error(w, "Invalid task ID", http.StatusBadRequest)
		return
	}

	detail, err := core.GetTaskDetailIn(rootPath, taskID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if detail == nil {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(detail)
}

func (s *Server) memoriesHandler(w http.ResponseWriter, r *http.Request, projectID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limit, err := parseOptionalInt(r.URL.Query().Get("limit"), core.DefaultMemoryListLimit)
	if err != nil {
		http.Error(w, "Invalid limit", http.StatusBadRequest)
		return
	}
	offset, err := parseOptionalInt(r.URL.Query().Get("offset"), 0)
	if err != nil {
		http.Error(w, "Invalid offset", http.StatusBadRequest)
		return
	}

	result, err := core.ListProjectMemories(s.db, core.MemoryListOptions{
		ProjectID: projectID,
		Type:      r.URL.Query().Get("type"),
		Query:     r.URL.Query().Get("q"),
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) memoryDetailHandler(w http.ResponseWriter, r *http.Request, projectID, memoryID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := core.ValidateMemoryID(memoryID); err != nil {
		http.Error(w, "Invalid memory ID", http.StatusBadRequest)
		return
	}
	detail, err := core.GetProjectMemory(s.db, projectID, memoryID)
	if err == sql.ErrNoRows {
		http.Error(w, "Memory not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(detail)
}

func parseOptionalInt(raw string, defaultValue int) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultValue, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	return value, nil
}

type ReportMeta struct {
	ID        string `json:"id"`
	UpdatedAt string `json:"updated_at"`
}

func (s *Server) reportsHandler(w http.ResponseWriter, r *http.Request, projectID, rootPath string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	reportsDir := filepath.Join(rootPath, ".ai", "reports")
	entries, err := os.ReadDir(reportsDir)
	if err != nil {
		if os.IsNotExist(err) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]\n"))
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var reports []ReportMeta
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		reports = append(reports, ReportMeta{
			ID:        strings.TrimSuffix(entry.Name(), ".yaml"),
			UpdatedAt: info.ModTime().UTC().Format(http.TimeFormat),
		})
	}

	sort.Slice(reports, func(i, j int) bool {
		return reports[i].UpdatedAt > reports[j].UpdatedAt
	})

	if reports == nil {
		reports = []ReportMeta{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reports)
}

func (s *Server) reportDetailHandler(w http.ResponseWriter, r *http.Request, projectID, rootPath, reportID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cleanReportID := filepath.Clean(reportID)
	if strings.Contains(cleanReportID, string(filepath.Separator)) || cleanReportID == ".." || cleanReportID == "." {
		http.Error(w, "Invalid report ID", http.StatusBadRequest)
		return
	}

	reportPath := filepath.Join(rootPath, ".ai", "reports", cleanReportID+".yaml")

	reportsDir := filepath.Join(rootPath, ".ai", "reports")
	rel, err := filepath.Rel(reportsDir, reportPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		http.Error(w, "Invalid report path", http.StatusBadRequest)
		return
	}

	payload, err := core.LoadArtifactPayload(reportPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "Report not found", http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	if err := core.ValidateAgainstSchema("report.schema.json", reportPath, payload); err != nil {
		http.Error(w, fmt.Sprintf("Report validation failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(payload)
}
