package bootstrap

import (
	"flag"
	"os"
)

// Config holds the application configuration.
type Config struct {
        DBPath          string // SQLITE_DB_PATH or --db; default "data/engram.db"
        OllamaHost      string // OLLAMA_HOST or --ollama-host; default "" → driver picks default
        EmbeddingModel  string // EMBEDDING_MODEL or --embedding-model; default "nomic-embed-text"
        EmbeddingDim    int    // hard-coded to 768 (matches existing code)
        ExtractionModel string // EXTRACTION_MODEL or --extraction-model; default "phi3.5"
}

// LoadFromEnv reads configuration from environment variables and applies defaults.
func LoadFromEnv() Config {
        dbPath := os.Getenv("SQLITE_DB_PATH")
        if dbPath == "" {
                dbPath = "data/engram.db"
        }

        ollamaHost := os.Getenv("OLLAMA_HOST")

        embedModel := os.Getenv("EMBEDDING_MODEL")
        if embedModel == "" {
                embedModel = "nomic-embed-text"
        }

        extractModel := os.Getenv("EXTRACTION_MODEL")
        if extractModel == "" {
                extractModel = "phi3.5"
        }

        return Config{
                DBPath:          dbPath,
                OllamaHost:      ollamaHost,
                EmbeddingModel:  embedModel,
                EmbeddingDim:    768,
                ExtractionModel: extractModel,
        }
}

// ApplyFlagOverrides overlays fields if flags were set on the command line.
func (c *Config) ApplyFlagOverrides(flags *flag.FlagSet) {
        if f := flags.Lookup("db"); f != nil && f.Value.String() != "" {
                // Only override if it's not the default value of the flag
                // But wait, flag.Lookup doesn't tell us if it was actually set.
                // However, the task says: "Only override if the flag value is not the zero value (empty string for these)."
                c.DBPath = f.Value.String()
        }
        if f := flags.Lookup("ollama-host"); f != nil && f.Value.String() != "" {
                c.OllamaHost = f.Value.String()
        }
        if f := flags.Lookup("embedding-model"); f != nil && f.Value.String() != "" {
                c.EmbeddingModel = f.Value.String()
        }
        if f := flags.Lookup("extraction-model"); f != nil && f.Value.String() != "" {
                c.ExtractionModel = f.Value.String()
        }
}
