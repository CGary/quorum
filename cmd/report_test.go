package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func setupReportTestEnv(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()

	bin := buildMemoryCLI(t)
	
	if err := exec.Command("git", "init", root).Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}
	
	// Create .agents/templates
	os.MkdirAll(filepath.Join(root, ".agents", "templates"), 0755)
	os.WriteFile(filepath.Join(root, ".agents", "templates", "report.yaml"), []byte("meta:\n  id: \"test\"\n  schemaVersion: \"1.0\"\n  date: \"\""), 0644)

	return bin, root
}

func TestReportNew(t *testing.T) {
	bin, dir := setupReportTestEnv(t)

	// Ensure memory db path
	dbPath := filepath.Join(t.TempDir(), "memory.db")

	// Create a .quorumrc in dir so it recognizes it as project root
	os.WriteFile(filepath.Join(dir, ".quorumrc"), []byte(`{"project_id":"report-demo","project_name":"Report Demo"}`), 0644)

	// Run `report new demo`
	out, err := runMemoryCmdErr(t, dir, bin, dbPath, "", "report", "new", "demo")
	if err != nil {
		t.Fatalf("quorum report new failed: %v\nOutput: %s", err, out)
	}

	reportPath := filepath.Join(dir, ".ai", "reports", "demo.yaml")
	if _, err := os.Stat(reportPath); os.IsNotExist(err) {
		t.Fatalf("expected report to be created at %s, but it was not", reportPath)
	}

	// Run it again, should fail
	out2, err2 := runMemoryCmdErr(t, dir, bin, dbPath, "", "report", "new", "demo")
	if err2 == nil {
		t.Fatalf("expected second report new to fail, but it succeeded: %s", out2)
	}
	if !strings.Contains(out2, "already exists") {
		t.Errorf("expected already exists error, got: %s", out2)
	}
}
