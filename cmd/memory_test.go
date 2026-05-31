package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type memoryCommandResult struct {
	ProjectID   string `json:"project_id"`
	MemoryID    string `json:"memory_id"`
	Status      string `json:"status"`
	ContentHash string `json:"content_hash"`
}

type memoryCommandStatus struct {
	DBPath            string `json:"db_path"`
	ProjectID         string `json:"project_id"`
	ProjectName       string `json:"project_name"`
	ProjectRegistered bool   `json:"project_registered"`
	Counts            struct {
		Pattern  int `json:"pattern"`
		Decision int `json:"decision"`
		Lesson   int `json:"lesson"`
	} `json:"counts"`
}

func TestMemorySaveCLIFromStdinAndStatus(t *testing.T) {
	bin, project, dbPath := setupMemoryCLITest(t)
	out := runMemoryCmd(t, project, bin, dbPath, validCLIMemoryJSON("LES-2026-05-31-10"), "memory", "save")
	var result memoryCommandResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid save JSON %q: %v", out, err)
	}
	if result.ProjectID != "quorum" || result.MemoryID != "LES-2026-05-31-10" || result.Status != "inserted" || result.ContentHash == "" {
		t.Fatalf("unexpected save result: %#v", result)
	}

	out = runMemoryCmd(t, project, bin, dbPath, "", "memory", "status")
	var status memoryCommandStatus
	if err := json.Unmarshal([]byte(out), &status); err != nil {
		t.Fatalf("invalid status JSON %q: %v", out, err)
	}
	if status.DBPath != dbPath || !status.ProjectRegistered || status.Counts.Lesson != 1 {
		t.Fatalf("unexpected status: %#v", status)
	}
}

func TestMemorySaveCLIFromFileAndIdempotent(t *testing.T) {
	bin, project, dbPath := setupMemoryCLITest(t)
	payloadPath := filepath.Join(project, "entry.json")
	if err := os.WriteFile(payloadPath, []byte(validCLIMemoryJSON("LES-2026-05-31-11")), 0644); err != nil {
		t.Fatal(err)
	}
	out := runMemoryCmd(t, project, bin, dbPath, "", "memory", "save", "--file", payloadPath)
	if !strings.Contains(out, `"status": "inserted"`) {
		t.Fatalf("expected inserted, got %q", out)
	}
	out = runMemoryCmd(t, project, bin, dbPath, "", "memory", "save", "--file", payloadPath)
	if !strings.Contains(out, `"status": "unchanged"`) {
		t.Fatalf("expected unchanged, got %q", out)
	}
}

func TestMemorySaveCLIErrors(t *testing.T) {
	bin, project, dbPath := setupMemoryCLITest(t)
	payloadPath := filepath.Join(project, "entry.json")
	if err := os.WriteFile(payloadPath, []byte(validCLIMemoryJSON("LES-2026-05-31-12")), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := runMemoryCmdErr(t, project, bin, dbPath, validCLIMemoryJSON("LES-2026-05-31-12"), "memory", "save", "--file", payloadPath)
	if err == nil || !strings.Contains(out, "not both") {
		t.Fatalf("expected stdin/file conflict, got err=%v out=%q", err, out)
	}
	out, err = runMemoryCmdErr(t, project, bin, dbPath, `{"id":`, "memory", "save")
	if err == nil || !strings.Contains(out, "invalid memory JSON") {
		t.Fatalf("expected invalid JSON error, got err=%v out=%q", err, out)
	}
	out, err = runMemoryCmdErr(t, project, bin, dbPath, `{"id":"LES-2026-05-31-13"}`, "memory", "save")
	if err == nil || !strings.Contains(out, "required property") {
		t.Fatalf("expected schema error, got err=%v out=%q", err, out)
	}
	out, err = runMemoryCmdErr(t, project, bin, dbPath, "", "memory", "save", "--file", filepath.Join(project, "missing.json"))
	if err == nil || !strings.Contains(out, "failed to read --file") {
		t.Fatalf("expected missing file error, got err=%v out=%q", err, out)
	}
}

func TestMemoryStatusCLIMissingQuorumConfig(t *testing.T) {
	bin := buildMemoryCLI(t)
	project := t.TempDir()
	if err := exec.Command("git", "init", project).Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}
	dbPath := filepath.Join(project, "memory.db")
	out, err := runMemoryCmdErr(t, project, bin, dbPath, "", "memory", "status")
	if err == nil || !strings.Contains(out, "run quorum init") {
		t.Fatalf("expected missing .quorumrc error, got err=%v out=%q", err, out)
	}
}

func setupMemoryCLITest(t *testing.T) (string, string, string) {
	t.Helper()
	bin := buildMemoryCLI(t)
	project := t.TempDir()
	if err := exec.Command("git", "init", project).Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project, ".quorumrc"), []byte(`{"project_id":"quorum","project_name":"Quorum"}`), 0644); err != nil {
		t.Fatal(err)
	}
	return bin, project, filepath.Join(project, "memory.db")
}

func buildMemoryCLI(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(cwd)
	bin := filepath.Join(t.TempDir(), "quorum")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}
	return bin
}

func runMemoryCmd(t *testing.T, dir, bin, dbPath, stdin string, args ...string) string {
	t.Helper()
	out, err := runMemoryCmdErr(t, dir, bin, dbPath, stdin, args...)
	if err != nil {
		t.Fatalf("%s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

func runMemoryCmdErr(t *testing.T, dir, bin, dbPath, stdin string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "QUORUM_MEMORY_DB="+dbPath, "QUORUM_SCHEMAS_DIR="+filepath.Join(filepath.Dir(mustGetwd(t)), ".agents", "schemas"))
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return cwd
}

func validCLIMemoryJSON(id string) string {
	return `{"id":"` + id + `","source_task":"SQL-02","type":"lesson","title":"Memory CLI behavior","context":"While testing SQL-02.","content":"The memory CLI saves validated entries through SQLite and reports deterministic JSON output.","created_at":"2026-05-31"}`
}
