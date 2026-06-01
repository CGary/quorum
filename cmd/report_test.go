package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// validSeedTemplate is a report template that is valid by construction against
// report.schema.json. `report new` now validates before writing, so a structurally
// invalid seed would fail at runtime.
const validSeedTemplate = `meta:
  id: "template-id"
  schemaVersion: "1.0"
  date: "2026-06-01T00:00:00Z"
summary: "Seed report."
findings:
  - id: "FINDING-01"
    description: "A finding."
    severity: "info"
evidence:
  - findingId: "FINDING-01"
    path: "path/to/file"
    details: "Evidence details."
risks:
  - id: "RISK-01"
    description: "A risk."
    impact: "low"
actionPlan:
  - step: 1
    action: "First step."
    owner: "unassigned"
`

// validReportPayload returns a schema-valid report whose meta.id equals id.
func validReportPayload(id string) string {
	return `meta:
  id: "` + id + `"
  schemaVersion: "1.0"
  date: "2026-06-01T00:00:00Z"
summary: "A saved report."
findings:
  - id: "FINDING-01"
    description: "A finding."
    severity: "high"
evidence:
  - findingId: "FINDING-01"
    path: "internal/core/schema.go"
    details: "Evidence details."
risks:
  - id: "RISK-01"
    description: "A risk."
    impact: "medium"
actionPlan:
  - step: 1
    action: "First step."
    owner: "unassigned"
`
}

func setupReportTestEnv(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()

	bin := buildMemoryCLI(t)

	if err := exec.Command("git", "init", root).Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}

	// Create a schema-valid .agents/templates/report.yaml seed.
	os.MkdirAll(filepath.Join(root, ".agents", "templates"), 0755)
	os.WriteFile(filepath.Join(root, ".agents", "templates", "report.yaml"), []byte(validSeedTemplate), 0644)

	// Recognize root as a project.
	os.WriteFile(filepath.Join(root, ".quorumrc"), []byte(`{"project_id":"report-demo","project_name":"Report Demo"}`), 0644)

	return bin, root
}

func TestReportNew(t *testing.T) {
	bin, dir := setupReportTestEnv(t)
	dbPath := filepath.Join(t.TempDir(), "memory.db")

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

func TestReportSaveValid(t *testing.T) {
	bin, dir := setupReportTestEnv(t)
	dbPath := filepath.Join(t.TempDir(), "memory.db")

	out, err := runMemoryCmdErr(t, dir, bin, dbPath, validReportPayload("audit-01"), "report", "save", "audit-01")
	if err != nil {
		t.Fatalf("quorum report save failed: %v\nOutput: %s", err, out)
	}

	reportPath := filepath.Join(dir, ".ai", "reports", "audit-01.yaml")
	if _, err := os.Stat(reportPath); os.IsNotExist(err) {
		t.Fatalf("expected report saved at %s, but it was not", reportPath)
	}
}

func TestReportSaveIDMismatchRejected(t *testing.T) {
	bin, dir := setupReportTestEnv(t)
	dbPath := filepath.Join(t.TempDir(), "memory.db")

	// meta.id ("other-id") does not match the filename id ("audit-01").
	out, err := runMemoryCmdErr(t, dir, bin, dbPath, validReportPayload("other-id"), "report", "save", "audit-01")
	if err == nil {
		t.Fatalf("expected save to fail on meta.id mismatch, but it succeeded: %s", out)
	}
	if !strings.Contains(out, "mismatch") {
		t.Errorf("expected identity mismatch error, got: %s", out)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".ai", "reports", "audit-01.yaml")); !os.IsNotExist(statErr) {
		t.Errorf("expected no file written on rejection")
	}
}

func TestReportSaveInvalidIDRegexRejected(t *testing.T) {
	bin, dir := setupReportTestEnv(t)
	dbPath := filepath.Join(t.TempDir(), "memory.db")

	// An ID containing a path separator violates ReportIDPattern.
	out, err := runMemoryCmdErr(t, dir, bin, dbPath, validReportPayload("bad/id"), "report", "save", "bad/id")
	if err == nil {
		t.Fatalf("expected save to fail on invalid id regex, but it succeeded: %s", out)
	}
	if !strings.Contains(out, "invalid report id") {
		t.Errorf("expected invalid report id error, got: %s", out)
	}
}

func TestReportSaveSchemaInvalidRejected(t *testing.T) {
	bin, dir := setupReportTestEnv(t)
	dbPath := filepath.Join(t.TempDir(), "memory.db")

	// The body is a palette (meta-only is valid), but the catalog is CLOSED:
	// an invented top-level component must be rejected by schema validation
	// (additionalProperties:false) at the write path.
	payload := "meta:\n  id: \"audit-02\"\n  schemaVersion: \"1.0\"\n  date: \"2026-06-01T00:00:00Z\"\ndiagram: \"graph TD; A-->B\"\n"
	out, err := runMemoryCmdErr(t, dir, bin, dbPath, payload, "report", "save", "audit-02")
	if err == nil {
		t.Fatalf("expected save to fail on schema-invalid payload, but it succeeded: %s", out)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".ai", "reports", "audit-02.yaml")); !os.IsNotExist(statErr) {
		t.Errorf("expected no file written on schema rejection")
	}
}
