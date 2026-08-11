//go:build sqlite_fts5 && sqlite_vec

package main

import (
	"flag"

	"github.com/hsme/core/src/bootstrap"
)

var (
	noColorFlag bool
)

type AgentFlags struct {
	JSON    bool
	NoInput bool
	Quiet   bool
	Verbose bool
	Output  string
	Timeout int
	Schema  bool
}

func RegisterAgentFlags(fs *flag.FlagSet, a *AgentFlags) {
	fs.BoolVar(&a.JSON, "json", false, "emit one JSON envelope on stdout")
	fs.BoolVar(&a.NoInput, "no-input", false, "never prompt interactively (agent default)")
	fs.BoolVar(&a.Quiet, "quiet", false, "suppress non-essential output")
	fs.BoolVar(&a.Verbose, "verbose", false, "verbose output")
	fs.StringVar(&a.Output, "output", "", "write full result to this file (returned as data.result_file)")
	fs.IntVar(&a.Timeout, "timeout", 0, "seconds before timeout (0 = default)")
	fs.BoolVar(&a.Schema, "schema", false, "print the input/output JSON contract and exit")
}

func RegisterDBFlags(fs *flag.FlagSet, cfg *bootstrap.Config) {
	fs.StringVar(&cfg.DBPath, "db", cfg.DBPath, "Path to SQLite database")
	fs.StringVar(&cfg.OllamaHost, "ollama-host", cfg.OllamaHost, "Ollama API host")
	fs.StringVar(&cfg.EmbeddingModel, "embedding-model", cfg.EmbeddingModel, "Model for generating embeddings")
	fs.BoolVar(&noColorFlag, "no-color", noColorFlag, "Disable ANSI color output")
}

func GetDBPath(fs *flag.FlagSet) string {
	f := fs.Lookup("db")
	if f != nil {
		return f.Value.String()
	}
	return ""
}

func GetOllamaHost(fs *flag.FlagSet) string {
	f := fs.Lookup("ollama-host")
	if f != nil {
		return f.Value.String()
	}
	return ""
}

func GetEmbeddingModel(fs *flag.FlagSet) string {
	f := fs.Lookup("embedding-model")
	if f != nil {
		return f.Value.String()
	}
	return ""
}
