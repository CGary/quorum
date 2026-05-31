package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"quorum/internal/core"
	"strings"
	"testing"
)

func TestMemorySearchCommandJSONOutput(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(tempDir, ".git"), 0755); err != nil {
		t.Fatalf("create .git failed: %v", err)
	}
	if err := core.WriteQuorumConfigTo(&core.QuorumConfig{ProjectID: "quorum", ProjectName: "Quorum"}, tempDir); err != nil {
		t.Fatalf("write .quorumrc failed: %v", err)
	}
	dbPath := filepath.Join(tempDir, "memory.db")
	t.Setenv("QUORUM_MEMORY_DB", dbPath)
	db, err := core.OpenMemoryDB(dbPath)
	if err != nil {
		t.Fatalf("OpenMemoryDB failed: %v", err)
	}
	_, err = db.Exec(`INSERT INTO memory_entries (id, project_id, type, source_task, title, context, content, hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, "DEC-2026-05-31-1", "quorum", "decision", "SQL-05", "Search decision", "CLI", "Search memory through JSON output.", "hash")
	if err != nil {
		t.Fatalf("insert memory failed: %v", err)
	}
	_ = db.Close()

	oldWd, _ := os.Getwd()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	defer os.Chdir(oldWd)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"memory", "search", "JSON", "--json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("command failed: %v", err)
		}
	})

	var payload struct {
		Count   int                       `json:"count"`
		Results []core.MemorySearchResult `json:"results"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("invalid JSON output %q: %v", out, err)
	}
	if payload.Count != 1 || payload.Results[0].ID != "DEC-2026-05-31-1" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestMemorySearchCommandRegistered(t *testing.T) {
	memory, _, err := rootCmd.Find([]string{"memory"})
	if err != nil || memory == nil {
		t.Fatalf("memory command not registered: %v", err)
	}
	search, _, err := rootCmd.Find([]string{"memory", "search"})
	if err != nil || search == nil || !strings.Contains(search.Use, "search") {
		t.Fatalf("memory search command not registered: %v", err)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe failed: %v", err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}
