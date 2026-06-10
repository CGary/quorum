//go:build sqlite_fts5 && sqlite_vec

package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromEnv(t *testing.T) {
	// Set some env vars
	os.Setenv("SQLITE_DB_PATH", "test.db")
	os.Setenv("OLLAMA_HOST", "http://test:11434")
	os.Setenv("EMBEDDING_MODEL", "test-model")
	defer func() {
		os.Unsetenv("SQLITE_DB_PATH")
		os.Unsetenv("OLLAMA_HOST")
		os.Unsetenv("EMBEDDING_MODEL")
	}()

	cfg := LoadFromEnv()

	if cfg.DBPath != "test.db" {
		t.Errorf("expected DBPath test.db, got %s", cfg.DBPath)
	}
	if cfg.OllamaHost != "http://test:11434" {
		t.Errorf("expected OllamaHost http://test:11434, got %s", cfg.OllamaHost)
	}
	if cfg.EmbeddingModel != "test-model" {
		t.Errorf("expected EmbeddingModel test-model, got %s", cfg.EmbeddingModel)
	}
	if cfg.EmbeddingDim != 768 {
		t.Errorf("expected EmbeddingDim 768, got %d", cfg.EmbeddingDim)
	}
}

func TestLoadFromEnvDefaults(t *testing.T) {
	os.Unsetenv("SQLITE_DB_PATH")
	os.Unsetenv("OLLAMA_HOST")
	os.Unsetenv("EMBEDDING_MODEL")

	cfg := LoadFromEnv()

	if cfg.DBPath != "data/engram.db" {
		t.Errorf("expected DBPath data/engram.db, got %s", cfg.DBPath)
	}
	if cfg.OllamaHost != "" {
		t.Errorf("expected empty OllamaHost, got %s", cfg.OllamaHost)
	}
	if cfg.EmbeddingModel != "nomic-embed-text" {
		t.Errorf("expected EmbeddingModel nomic-embed-text, got %s", cfg.EmbeddingModel)
	}
}

func TestOpenDB(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "bootstrap-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	cfg := Config{
		DBPath: dbPath,
	}

	db, err := OpenDB(cfg)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer db.Close()

	if db == nil {
		t.Fatal("expected db to be non-nil")
	}

	// Verify it's actually a DB by pinging
	if err := db.Ping(); err != nil {
		t.Errorf("db ping failed: %v", err)
	}
}

func TestOpenDBInvalidPath(t *testing.T) {
	cfg := Config{
		DBPath: "/nonexistent/path/to/db.db",
	}

	db, err := OpenDB(cfg)
	if err == nil {
		db.Close()
		t.Fatal("expected OpenDB to fail for invalid path")
	}
}
