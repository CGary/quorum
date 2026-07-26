package core_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"quorum/internal/core"
)

func TestReportService_NewReport_HappyPath(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("QUORUM_MEMORY_DB", filepath.Join(dir, "memory.db"))
	t.Cleanup(func() { os.Unsetenv("QUORUM_MEMORY_DB") })

	// Needs valid generic template in the project
	os.MkdirAll(filepath.Join(dir, ".agents", "templates"), 0755)
	os.WriteFile(filepath.Join(dir, ".agents", "templates", "report.yaml"), []byte("meta:\n  id: temp\n  schemaVersion: \"1.1\"\nkind: generic\npresentation:\n  profile: cognitive\n  density: medium\n  audience: engineer\n  language: en\ncontent:\n  title: T\n  verdict:\n    text: V\n  sections:\n    - id: s\n      role: analysis\n      title: T\n      body: B\n"), 0644)

	svc := core.ReportService{ProjectRoot: dir}
	res, err := svc.NewReport("audit-01", core.ReportNewOptions{})
	if err != nil {
		t.Fatalf("NewReport failed: %v", err)
	}
	if res.IsScaffold {
		t.Errorf("expected IsScaffold to be false")
	}
	if !strings.HasSuffix(res.Path, "audit-01.yaml") {
		t.Errorf("unexpected path: %s", res.Path)
	}

	b, err := os.ReadFile(res.Path)
	if err != nil {
		t.Fatalf("failed to read persisted report: %v", err)
	}
	if !strings.Contains(string(b), "id: audit-01") {
		t.Errorf("report id not substituted")
	}
}

func TestReportService_NewReport_InvalidID(t *testing.T) {
	svc := core.ReportService{ProjectRoot: t.TempDir()}
	_, err := svc.NewReport("bad/id", core.ReportNewOptions{})
	if err == nil {
		t.Errorf("expected invalid ID error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid report id") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestReportService_NewReport_TemplateLoadError(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("QUORUM_MEMORY_DB", filepath.Join(dir, "memory.db"))
	t.Cleanup(func() { os.Unsetenv("QUORUM_MEMORY_DB") })
	// No template on disk, and no embedded agents bundle set up for this specific unit test run (or kind doesn't exist).
	svc := core.ReportService{ProjectRoot: dir}
	_, err := svc.NewReport("audit-02", core.ReportNewOptions{Kind: "nonexistent"})
	if err == nil {
		t.Errorf("expected template load error, got nil")
	}
	if !strings.Contains(err.Error(), "not found on disk") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestReportService_SaveReport_HappyPath(t *testing.T) {
	dir := t.TempDir()
	svc := core.ReportService{ProjectRoot: dir}

	raw := []byte("meta:\n  id: audit-03\nkind: generic\npresentation:\n  profile: cognitive\n  density: medium\n  audience: engineer\n  language: en\ncontent:\n  title: T\n  verdict:\n    text: V\n  sections:\n    - id: s\n      role: analysis\n      title: T\n      body: B\n")
	res, err := svc.SaveReport("audit-03", raw, core.ReportSaveOptions{})
	if err != nil {
		t.Fatalf("SaveReport failed: %v", err)
	}
	if res.DryRun {
		t.Errorf("expected DryRun to be false")
	}

	if _, err := os.Stat(res.Path); err != nil {
		t.Errorf("expected report to be saved, got: %v", err)
	}
}

func TestReportService_SaveReport_InvalidID(t *testing.T) {
	svc := core.ReportService{ProjectRoot: t.TempDir()}
	_, err := svc.SaveReport("bad/id", []byte{}, core.ReportSaveOptions{})
	if err == nil {
		t.Errorf("expected invalid ID error, got nil")
	}
}

func TestReportService_SaveReport_DryRun(t *testing.T) {
	dir := t.TempDir()
	svc := core.ReportService{ProjectRoot: dir}

	raw := []byte("meta:\n  id: audit-04\nkind: generic\npresentation:\n  profile: cognitive\n  density: medium\n  audience: engineer\n  language: en\ncontent:\n  title: T\n  verdict:\n    text: V\n  sections:\n    - id: s\n      role: analysis\n      title: T\n      body: B\n")
	res, err := svc.SaveReport("audit-04", raw, core.ReportSaveOptions{DryRun: true})
	if err != nil {
		t.Fatalf("SaveReport dry-run failed: %v", err)
	}
	if !res.DryRun {
		t.Errorf("expected DryRun to be true")
	}

	if _, err := os.Stat(res.Path); !os.IsNotExist(err) {
		t.Errorf("expected report to NOT be saved in dry-run mode")
	}
}
