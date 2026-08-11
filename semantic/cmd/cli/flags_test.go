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
	if !noColorFlag {
		t.Errorf("got noColorFlag false, want true")
	}
}

func TestRegisterAgentFlags(t *testing.T) {
	fs := flag.NewFlagSet("agent-test", flag.ContinueOnError)
	var a AgentFlags

	RegisterAgentFlags(fs, &a)

	// Verify defaults
	if a.JSON || a.NoInput || a.Quiet || a.Verbose || a.Schema || a.Output != "" || a.Timeout != 0 {
		t.Errorf("expected zero-value defaults, got %+v", a)
	}

	err := fs.Parse([]string{
		"-json",
		"-no-input",
		"-quiet",
		"-verbose",
		"-output", "out.json",
		"-timeout", "42",
		"-schema",
	})
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if !a.JSON {
		t.Errorf("expected JSON true")
	}
	if !a.NoInput {
		t.Errorf("expected NoInput true")
	}
	if !a.Quiet {
		t.Errorf("expected Quiet true")
	}
	if !a.Verbose {
		t.Errorf("expected Verbose true")
	}
	if a.Output != "out.json" {
		t.Errorf("got Output %q, want %q", a.Output, "out.json")
	}
	if a.Timeout != 42 {
		t.Errorf("got Timeout %d, want 42", a.Timeout)
	}
	if !a.Schema {
		t.Errorf("expected Schema true")
	}
}
