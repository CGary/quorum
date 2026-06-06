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
  schemaVersion: "1.1"
  date: "2026-06-01T00:00:00Z"
kind: "generic"
presentation:
  profile: "cognitive"
  density: "medium"
  audience: "engineer"
  language: "en"
content:
  title: "Seed report"
  kicker: ""
  summary: "Seed report."
  verdict:
    text: "Seed."
    confidence: "high"
  sections:
    - id: "surface"
      role: "decision_surface"
      title: "Decision Surface"
      body:
        "Status": "Seeded"
`

// validReportPayload returns a schema-valid report whose meta.id equals id.
func validReportPayload(id string) string {
	return `meta:
  id: "` + id + `"
  schemaVersion: "1.1"
  date: "2026-06-01T00:00:00Z"
kind: "generic"
presentation:
  profile: "cognitive"
  density: "medium"
  audience: "engineer"
  language: "en"
content:
  title: "Report Title"
  verdict:
    text: "Verdict."
    confidence: "high"
  sections:
    - id: "surface"
      role: "decision_surface"
      title: "Decision Surface"
      body:
        "Status": "Saved"
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
	payload := "meta:\n  id: \"audit-02\"\n  schemaVersion: \"1.1\"\n  date: \"2026-06-01T00:00:00Z\"\nkind: \"generic\"\npresentation:\n  profile: \"cognitive\"\n  density: \"medium\"\n  audience: \"engineer\"\n  language: \"en\"\ncontent:\n  title: \"T\"\n  verdict:\n    text: \"Bottom line.\"\n  sections:\n    - id: \"s\"\n      role: \"decision_surface\"\n      title: \"T\"\n      body:\n        \"S\": \"S\"\n      extraInvalidField: true\n"
	out, err := runMemoryCmdErr(t, dir, bin, dbPath, payload, "report", "save", "audit-02")
	if err == nil {
		t.Fatalf("expected save to fail on schema-invalid payload, but it succeeded: %s", out)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".ai", "reports", "audit-02.yaml")); !os.IsNotExist(statErr) {
		t.Errorf("expected no file written on schema rejection")
	}
}

func TestReportSaveWithFileArgument(t *testing.T) {
	bin, dir := setupReportTestEnv(t)
	dbPath := filepath.Join(t.TempDir(), "memory.db")

	// Write payload to a temporary file
	payloadPath := filepath.Join(dir, "temp_test_report.yaml")
	payloadContent := validReportPayload("audit-03")
	if err := os.WriteFile(payloadPath, []byte(payloadContent), 0644); err != nil {
		t.Fatalf("failed to write temp report: %v", err)
	}

	// Save using --file (the convention shared with `memory save`), passing NO stdin.
	out, err := runMemoryCmdErr(t, dir, bin, dbPath, "", "report", "save", "audit-03", "--file", payloadPath)
	if err != nil {
		t.Fatalf("quorum report save --file failed: %v\nOutput: %s", err, out)
	}

	reportPath := filepath.Join(dir, ".ai", "reports", "audit-03.yaml")
	if _, err := os.Stat(reportPath); os.IsNotExist(err) {
		t.Fatalf("expected report saved at %s, but it was not", reportPath)
	}
}

func TestReportNewOutputScaffold(t *testing.T) {
	bin, dir := setupReportTestEnv(t)
	dbPath := filepath.Join(t.TempDir(), "memory.db")

	// Scaffold a draft into .tmp/ (staging), not .ai/reports/.
	out, err := runMemoryCmdErr(t, dir, bin, dbPath, "", "report", "new", "draft-99", "--output", ".tmp/draft-99.yaml")
	if err != nil {
		t.Fatalf("report new --output failed: %v\n%s", err, out)
	}

	scaffoldPath := filepath.Join(dir, ".tmp", "draft-99.yaml")
	data, err := os.ReadFile(scaffoldPath)
	if err != nil {
		t.Fatalf("expected scaffold at %s: %v", scaffoldPath, err)
	}
	if !strings.Contains(string(data), `id: "draft-99"`) {
		t.Errorf("scaffold must stamp the id; got:\n%s", data)
	}
	if strings.Contains(string(data), "template-id") {
		t.Errorf("scaffold must replace the template placeholder id")
	}

	// --output must NOT touch the final reports directory.
	if _, statErr := os.Stat(filepath.Join(dir, ".ai", "reports", "draft-99.yaml")); !os.IsNotExist(statErr) {
		t.Errorf("--output must not write into .ai/reports/")
	}

	// The scaffold is valid and saveable as-is.
	if out, err := runMemoryCmdErr(t, dir, bin, dbPath, "", "report", "save", "draft-99", "--file", ".tmp/draft-99.yaml"); err != nil {
		t.Fatalf("saving the scaffold should succeed: %v\n%s", err, out)
	}
}

func TestReportNewUsesEmbeddedTemplateFallback(t *testing.T) {
	bin, dir := setupReportTestEnv(t)
	dbPath := filepath.Join(t.TempDir(), "memory.db")

	// Remove the on-disk template so only the binary-embedded bundle can serve
	// it — this mirrors a consumer project where `quorum init` never placed
	// .agents/templates/report.yaml where `report new` looks.
	if err := os.Remove(filepath.Join(dir, ".agents", "templates", "report.yaml")); err != nil {
		t.Fatalf("remove on-disk template: %v", err)
	}

	out, err := runMemoryCmdErr(t, dir, bin, dbPath, "", "report", "new", "emb-01", "--output", ".tmp/emb-01.yaml")
	if err != nil {
		t.Fatalf("report new should fall back to the embedded template, got: %v\n%s", err, out)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".tmp", "emb-01.yaml"))
	if err != nil {
		t.Fatalf("expected scaffold from embedded template: %v", err)
	}
	if !strings.Contains(string(data), `id: "emb-01"`) {
		t.Errorf("embedded scaffold must stamp the id; got:\n%s", data)
	}
}

func TestReportSaveDryRun(t *testing.T) {
	bin, dir := setupReportTestEnv(t)
	dbPath := filepath.Join(t.TempDir(), "memory.db")

	// Valid payload + --dry-run: passes preflight, persists nothing.
	out, err := runMemoryCmdErr(t, dir, bin, dbPath, validReportPayload("dry-01"), "report", "save", "dry-01", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run on a valid report should pass: %v\n%s", err, out)
	}
	if !strings.Contains(out, "dry-run") {
		t.Errorf("expected a dry-run confirmation, got: %s", out)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".ai", "reports", "dry-01.yaml")); !os.IsNotExist(statErr) {
		t.Errorf("dry-run must not write the report file")
	}

	// Identity mismatch is caught by the dry-run preflight too.
	if out, err := runMemoryCmdErr(t, dir, bin, dbPath, validReportPayload("other"), "report", "save", "dry-01", "--dry-run"); err == nil {
		t.Fatalf("dry-run must fail on meta.id mismatch, got: %s", out)
	}
}

func TestReportSaveFillsMetadata(t *testing.T) {
	bin, dir := setupReportTestEnv(t)
	dbPath := filepath.Join(t.TempDir(), "memory.db")

	// Draft carries only meta.id (no schemaVersion, no date): save must fill them
	// before validation so a hand-written draft does not fail on missing metadata.
	payload := "meta:\n  id: \"fill-01\"\nkind: \"generic\"\npresentation:\n  profile: \"cognitive\"\n  density: \"medium\"\n  audience: \"engineer\"\n  language: \"en\"\ncontent:\n  title: \"T\"\n  verdict:\n    text: \"Bottom line.\"\n  sections:\n    - id: \"s\"\n      role: \"decision_surface\"\n      title: \"T\"\n      body:\n        \"S\": \"S\"\n"
	out, err := runMemoryCmdErr(t, dir, bin, dbPath, payload, "report", "save", "fill-01")
	if err != nil {
		t.Fatalf("save should auto-fill missing meta and succeed: %v\n%s", err, out)
	}

	saved, err := os.ReadFile(filepath.Join(dir, ".ai", "reports", "fill-01.yaml"))
	if err != nil {
		t.Fatalf("expected saved report: %v", err)
	}
	s := string(saved)
	if !strings.Contains(s, "schemaVersion") {
		t.Errorf("save must inject meta.schemaVersion; got:\n%s", s)
	}
	if !strings.Contains(s, "date") {
		t.Errorf("save must inject meta.date; got:\n%s", s)
	}
}

func TestValidateSchemaOverride(t *testing.T) {
	bin, dir := setupReportTestEnv(t)
	dbPath := filepath.Join(t.TempDir(), "memory.db")

	// A temp-located report with no "reports/" path segment: path-based
	// detection cannot classify it.
	draftDir := filepath.Join(dir, ".tmp")
	if err := os.MkdirAll(draftDir, 0755); err != nil {
		t.Fatalf("mkdir .tmp: %v", err)
	}
	if err := os.WriteFile(filepath.Join(draftDir, "draft.yaml"), []byte(validReportPayload("draft-01")), 0644); err != nil {
		t.Fatalf("write draft: %v", err)
	}

	// --schema report makes it validate despite the temp location.
	if out, err := runMemoryCmdErr(t, dir, bin, dbPath, "", "validate", "--schema", "report", ".tmp/draft.yaml"); err != nil {
		t.Fatalf("validate --schema report should pass, got: %v\n%s", err, out)
	}

	// Without the override, path-based detection rejects it as unsupported.
	if out, err := runMemoryCmdErr(t, dir, bin, dbPath, "", "validate", ".tmp/draft.yaml"); err == nil {
		t.Fatalf("validate without --schema should fail on an unclassifiable path, got: %s", out)
	}

	// An unknown schema name is rejected by the whitelist.
	out, err := runMemoryCmdErr(t, dir, bin, dbPath, "", "validate", "--schema", "bogus", ".tmp/draft.yaml")
	if err == nil {
		t.Fatalf("validate --schema bogus should fail, got: %s", out)
	}
	if !strings.Contains(out, "unknown schema") {
		t.Errorf("expected 'unknown schema' error, got: %s", out)
	}
}
