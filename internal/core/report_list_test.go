package core_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"quorum/internal/core"
	"testing"
)

func writeReportListFile(t *testing.T, dir, name, id, kind, title, date string) {
	t.Helper()
	payload := "meta:\n  id: \"" + id + "\"\n  schemaVersion: \"1.1\"\n  date: \"" + date + "\"\nkind: \"" + kind + "\"\ncontent:\n  title: \"" + title + "\"\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(payload), 0644); err != nil {
		t.Fatalf("write report fixture: %v", err)
	}
}

func TestListReportsPopulated(t *testing.T) {
	dir := t.TempDir()
	writeReportListFile(t, dir, "b.yaml", "report-b", "audit", "Beta", "2026-07-02T00:00:00Z")
	writeReportListFile(t, dir, "a.yaml", "report-a", "generic", "Alpha", "2026-07-01T00:00:00Z")

	got, err := core.ListReports(dir)
	if err != nil {
		t.Fatalf("ListReports returned error: %v", err)
	}

	want := []core.ReportSummary{
		{ID: "report-a", Kind: "generic", Title: "Alpha", Date: "2026-07-01T00:00:00Z"},
		{ID: "report-b", Kind: "audit", Title: "Beta", Date: "2026-07-02T00:00:00Z"},
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d reports, got %d: %#v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("report %d mismatch: got %#v want %#v", i, got[i], want[i])
		}
	}
}

func TestListReportsEmptyAndNonexistentReturnNonNilEmptySlice(t *testing.T) {
	for _, dir := range []string{t.TempDir(), filepath.Join(t.TempDir(), "missing")} {
		got, err := core.ListReports(dir)
		if err != nil {
			t.Fatalf("ListReports(%s) returned error: %v", dir, err)
		}
		if got == nil {
			t.Fatalf("ListReports(%s) returned nil slice", dir)
		}
		if len(got) != 0 {
			t.Fatalf("ListReports(%s) returned reports: %#v", dir, got)
		}
		raw, err := json.Marshal(got)
		if err != nil {
			t.Fatalf("marshal empty reports: %v", err)
		}
		if string(raw) != "[]" {
			t.Fatalf("empty reports must marshal as [], got %s", raw)
		}
	}
}

func TestListReportsSkipsCorruptYAML(t *testing.T) {
	dir := t.TempDir()
	writeReportListFile(t, dir, "valid.yaml", "good", "technical_analysis", "Good", "2026-07-03T00:00:00Z")
	if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("meta:\n  id: [unterminated\n"), 0644); err != nil {
		t.Fatalf("write corrupt fixture: %v", err)
	}

	got, err := core.ListReports(dir)
	if err != nil {
		t.Fatalf("ListReports returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected only the valid report, got %#v", got)
	}
	if got[0].ID != "good" || got[0].Title != "Good" {
		t.Fatalf("valid report summary mismatch: %#v", got[0])
	}
}
