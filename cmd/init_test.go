package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureReportsGitignoreEntryPreservesUnterminatedFinalEntry(t *testing.T) {
	root := t.TempDir()
	gitignorePath := filepath.Join(root, ".gitignore")
	initial := "# Quorum\nworktrees/\n.tmp/"
	if err := os.WriteFile(gitignorePath, []byte(initial), 0644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	if err := ensureReportsGitignoreEntry(root); err != nil {
		t.Fatalf("ensureReportsGitignoreEntry failed: %v", err)
	}

	got, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	want := "# Quorum\nworktrees/\n.tmp/\n.ai/reports/\n"
	if string(got) != want {
		t.Fatalf(".gitignore content mismatch\nwant: %q\n got: %q", want, string(got))
	}
}

func TestEnsureReportsGitignoreEntryIsIdempotent(t *testing.T) {
	root := t.TempDir()
	gitignorePath := filepath.Join(root, ".gitignore")
	initial := ".tmp/\n.ai/reports/\n"
	if err := os.WriteFile(gitignorePath, []byte(initial), 0644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	if err := ensureReportsGitignoreEntry(root); err != nil {
		t.Fatalf("first ensureReportsGitignoreEntry failed: %v", err)
	}
	if err := ensureReportsGitignoreEntry(root); err != nil {
		t.Fatalf("second ensureReportsGitignoreEntry failed: %v", err)
	}

	got, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if string(got) != initial {
		t.Fatalf(".gitignore should be unchanged\nwant: %q\n got: %q", initial, string(got))
	}
}
