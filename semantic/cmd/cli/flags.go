//go:build sqlite_fts5 && sqlite_vec

package main

import (
	"flag"

	"github.com/hsme/core/src/bootstrap"
)

var (
	outputFormat string = "text"
	noColorFlag  bool
)

func RegisterDBFlags(fs *flag.FlagSet, cfg *bootstrap.Config) {
	fs.StringVar(&cfg.DBPath, "db", cfg.DBPath, "Path to SQLite database")
	fs.StringVar(&cfg.OllamaHost, "ollama-host", cfg.OllamaHost, "Ollama API host")
	fs.StringVar(&cfg.EmbeddingModel, "embedding-model", cfg.EmbeddingModel, "Model for generating embeddings")
	fs.StringVar(&outputFormat, "format", outputFormat, "Output format (text|json)")
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
