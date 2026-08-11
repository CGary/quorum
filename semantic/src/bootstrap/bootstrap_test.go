//go:build sqlite_fts5 && sqlite_vec

package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hsme/core/src/core/inference/ollama"
	"github.com/hsme/core/src/core/inference/openrouter"
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
	os.Unsetenv("EXTRACTION_TIMEOUT_SHORT_S")
	os.Unsetenv("EXTRACTION_TIMEOUT_LONG_S")

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
	if cfg.ExtractionTimeoutShortS != 300 {
		t.Errorf("expected default ExtractionTimeoutShortS 300, got %d", cfg.ExtractionTimeoutShortS)
	}
	if cfg.ExtractionTimeoutLongS != 900 {
		t.Errorf("expected default ExtractionTimeoutLongS 900, got %d", cfg.ExtractionTimeoutLongS)
	}
}

func TestLoadFromEnvExtractionTimeouts(t *testing.T) {
	t.Setenv("EXTRACTION_TIMEOUT_SHORT_S", "120")
	t.Setenv("EXTRACTION_TIMEOUT_LONG_S", "600")

	cfg := LoadFromEnv()
	if cfg.ExtractionTimeoutShortS != 120 {
		t.Errorf("expected ExtractionTimeoutShortS 120, got %d", cfg.ExtractionTimeoutShortS)
	}
	if cfg.ExtractionTimeoutLongS != 600 {
		t.Errorf("expected ExtractionTimeoutLongS 600, got %d", cfg.ExtractionTimeoutLongS)
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

func TestOpenWithWorkerProviderBranch(t *testing.T) {
	tmpDir := t.TempDir()
	cases := []struct {
		provider     string
		wantFallback bool
	}{{"", false}, {"openrouter", true}}
	for i, tc := range cases {
		cfg := Config{
			DBPath:                  filepath.Join(tmpDir, fmt.Sprintf("test-%d.db", i)),
			ExtractionProvider:      tc.provider,
			ExtractionModel:         "test-model",
			FallbackExtractionModel: "phi3.5",
			ExtractionTimeoutShortS: 300,
			ExtractionTimeoutLongS:  900,
			OpenRouterBaseURL:       "http://127.0.0.1:0",
		}
		db, _, extractor, err := OpenWithWorker(cfg)
		if err != nil {
			t.Fatalf("provider %q: OpenWithWorker failed: %v", tc.provider, err)
		}
		db.Close()
		chain, isFallback := extractor.(*openrouter.ChainExtractor)
		_, isOllama := extractor.(*ollama.Extractor)
		if tc.wantFallback != isFallback || tc.wantFallback == isOllama {
			t.Errorf("provider %q: got %T, wantFallback=%v", tc.provider, extractor, tc.wantFallback)
		}

		if tc.wantFallback {
			tiers := chain.Tiers()
			if len(tiers) != 3 {
				t.Fatalf("expected 3 tiers in openrouter chain, got %d", len(tiers))
			}
			if tiers[0].Name != "openrouter-short" || tiers[0].Timeout != 300*time.Second {
				t.Errorf("tier 0 mismatch: %+v", tiers[0])
			}
			if tiers[1].Name != "openrouter-long" || tiers[1].Timeout != 900*time.Second {
				t.Errorf("tier 1 mismatch: %+v", tiers[1])
			}
			if tiers[2].Name != "ollama-fallback" || tiers[2].Timeout != 0 {
				t.Errorf("tier 2 mismatch: %+v", tiers[2])
			}
		}
	}
}

func TestLoadFromEnvFallbackExtractionModel(t *testing.T) {
	t.Setenv("EXTRACTION_PROVIDER", "openrouter")
	t.Setenv("FALLBACK_EXTRACTION_MODEL", "")

	cfg := LoadFromEnv()
	if cfg.FallbackExtractionModel != "phi3.5" {
		t.Errorf("expected default FallbackExtractionModel phi3.5, got %s", cfg.FallbackExtractionModel)
	}

	t.Setenv("FALLBACK_EXTRACTION_MODEL", "somemodel")
	cfg = LoadFromEnv()
	if cfg.FallbackExtractionModel != "somemodel" {
		t.Errorf("expected FallbackExtractionModel somemodel, got %s", cfg.FallbackExtractionModel)
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
