//go:build sqlite_fts5 && sqlite_vec

package main

import (
	"flag"
	"testing"

	"github.com/hsme/core/src/bootstrap"
)

func TestRegisterDBFlags(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cfg := bootstrap.Config{
		DBPath:         "default.db",
		OllamaHost:     "localhost:11434",
		EmbeddingModel: "nomic-embed-text",
	}

	RegisterDBFlags(fs, &cfg)

	// Test default values
	if GetDBPath(fs) != "default.db" {
		t.Errorf("got %q, want %q", GetDBPath(fs), "default.db")
	}

	// Parse custom values
	err := fs.Parse([]string{
		"-db", "custom.db",
		"-ollama-host", "other:11434",
		"-embedding-model", "other-model",
		"-format", "json",
		"-no-color",
	})
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if cfg.DBPath != "custom.db" {
		t.Errorf("got %q, want %q", cfg.DBPath, "custom.db")
	}
	if cfg.OllamaHost != "other:11434" {
		t.Errorf("got %q, want %q", cfg.OllamaHost, "other:11434")
	}
	if cfg.EmbeddingModel != "other-model" {
		t.Errorf("got %q, want %q", cfg.EmbeddingModel, "other-model")
	}
	if outputFormat != "json" {
		t.Errorf("got %q, want %q", outputFormat, "json")
	}
	if !noColorFlag {
		t.Errorf("got noColorFlag false, want true")
	}
}
