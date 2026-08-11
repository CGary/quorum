package bootstrap

import (
	"flag"
	"os"
	"strconv"
)

// Config holds the application configuration.
type Config struct {
	DBPath                  string // SQLITE_DB_PATH or --db; default "data/engram.db"
	OllamaHost              string // OLLAMA_HOST or --ollama-host; default "" → driver picks default
	EmbeddingModel          string // EMBEDDING_MODEL or --embedding-model; default "nomic-embed-text"
	EmbeddingDim            int    // hard-coded to 768 (matches existing code)
	ExtractionModel         string // EXTRACTION_MODEL or --extraction-model; default "phi3.5" (or "nvidia/nemotron-nano-9b-v2:free" for openrouter)
	ExtractionProvider      string // EXTRACTION_PROVIDER or --extraction-provider; default "ollama"
	FallbackExtractionModel string // FALLBACK_EXTRACTION_MODEL; default "phi3.5"
	ExtractionTimeoutShortS int    // EXTRACTION_TIMEOUT_SHORT_S; default 300
	ExtractionTimeoutLongS  int    // EXTRACTION_TIMEOUT_LONG_S; default 900
	OpenRouterAPIKey        string // OPENROUTER_API_KEY
	OpenRouterBaseURL       string // OPENROUTER_BASE_URL; default "https://openrouter.ai/api/v1"
	ImportIntervalS         int    // IMPORT_INTERVAL_S; default 0
	ImportProject           string // IMPORT_PROJECT; default ""
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

	extractProvider := os.Getenv("EXTRACTION_PROVIDER")
	if extractProvider == "" {
		extractProvider = "ollama"
	}

	extractModel := os.Getenv("EXTRACTION_MODEL")
	if extractModel == "" {
		if extractProvider == "openrouter" {
			extractModel = "nvidia/nemotron-nano-9b-v2:free"
		} else {
			extractModel = "phi3.5"
		}
	}

	fallbackExtractModel := os.Getenv("FALLBACK_EXTRACTION_MODEL")
	if fallbackExtractModel == "" {
		fallbackExtractModel = "phi3.5"
	}

	timeoutShort := 300
	if v := os.Getenv("EXTRACTION_TIMEOUT_SHORT_S"); v != "" {
		if s, err := strconv.Atoi(v); err == nil && s > 0 {
			timeoutShort = s
		}
	}

	timeoutLong := 900
	if v := os.Getenv("EXTRACTION_TIMEOUT_LONG_S"); v != "" {
		if s, err := strconv.Atoi(v); err == nil && s > 0 {
			timeoutLong = s
		}
	}

	openRouterAPIKey := os.Getenv("OPENROUTER_API_KEY")

	openRouterBaseURL := os.Getenv("OPENROUTER_BASE_URL")
	if openRouterBaseURL == "" {
		openRouterBaseURL = "https://openrouter.ai/api/v1"
	}

	importIntervalS := 0
	if v := os.Getenv("IMPORT_INTERVAL_S"); v != "" {
		if s, err := strconv.Atoi(v); err == nil && s > 0 {
			importIntervalS = s
		}
	}

	importProject := os.Getenv("IMPORT_PROJECT")

	return Config{
		DBPath:                  dbPath,
		OllamaHost:              ollamaHost,
		EmbeddingModel:          embedModel,
		EmbeddingDim:            768,
		ExtractionModel:         extractModel,
		ExtractionProvider:      extractProvider,
		FallbackExtractionModel: fallbackExtractModel,
		ExtractionTimeoutShortS: timeoutShort,
		ExtractionTimeoutLongS:  timeoutLong,
		OpenRouterAPIKey:        openRouterAPIKey,
		OpenRouterBaseURL:       openRouterBaseURL,
		ImportIntervalS:         importIntervalS,
		ImportProject:           importProject,
	}
}

// ApplyFlagOverrides overlays fields if flags were set on the command line.
func (c *Config) ApplyFlagOverrides(flags *flag.FlagSet) {
	if f := flags.Lookup("db"); f != nil && f.Value.String() != "" {
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
	if f := flags.Lookup("extraction-provider"); f != nil && f.Value.String() != "" {
		c.ExtractionProvider = f.Value.String()
	}
	if f := flags.Lookup("openrouter-api-key"); f != nil && f.Value.String() != "" {
		c.OpenRouterAPIKey = f.Value.String()
	}
	if f := flags.Lookup("openrouter-base-url"); f != nil && f.Value.String() != "" {
		c.OpenRouterBaseURL = f.Value.String()
	}
}
