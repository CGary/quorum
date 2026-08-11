//go:build sqlite_fts5 && sqlite_vec

package bootstrap

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/hsme/core/src/core/inference/ollama"
	"github.com/hsme/core/src/core/inference/openrouter"
	"github.com/hsme/core/src/core/search"
	"github.com/hsme/core/src/core/worker"
	"github.com/hsme/core/src/storage/sqlite"
)

// OpenDB opens the SQLite database and loads decay configuration.
func OpenDB(cfg Config) (*sql.DB, error) {
	db, err := sqlite.InitDB(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("bootstrap open db: %w", err)
	}

	decayCfg, err := search.LoadDecayConfig()
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("bootstrap load decay config: %w", err)
	}
	search.GlobalDecayConfig = decayCfg

	return db, nil
}

// openWithEmbedderInternal is a helper that returns the shared ollama client.
func openWithEmbedderInternal(cfg Config) (*sql.DB, *ollama.Client, *ollama.Embedder, error) {
	db, err := OpenDB(cfg)
	if err != nil {
		return nil, nil, nil, err
	}

	client := ollama.NewClient(cfg.OllamaHost)
	embedder := ollama.NewEmbedder(client, cfg.EmbeddingModel, cfg.EmbeddingDim)

	if err := sqlite.ValidateEmbeddingConfig(db, embedder); err != nil {
		db.Close()
		return nil, nil, nil, fmt.Errorf("bootstrap validate embedding: %w", err)
	}

	return db, client, embedder, nil
}

// OpenWithEmbedder opens the database and initializes a validated embedder.
func OpenWithEmbedder(cfg Config) (*sql.DB, *ollama.Embedder, error) {
	db, _, embedder, err := openWithEmbedderInternal(cfg)
	return db, embedder, err
}

// OpenWithWorker opens the database and initializes embedder and extractor.
func OpenWithWorker(cfg Config) (*sql.DB, *ollama.Embedder, worker.GraphExtractor, error) {
	db, client, embedder, err := openWithEmbedderInternal(cfg)
	if err != nil {
		return nil, nil, nil, err
	}

	ollamaExtractor := ollama.NewExtractor(client, cfg.ExtractionModel)

	if cfg.ExtractionProvider == "openrouter" {
		orClient := openrouter.NewClient(cfg.OpenRouterBaseURL, cfg.OpenRouterAPIKey)
		orExtractor := openrouter.NewExtractor(orClient, cfg.ExtractionModel)
		ollamaFallback := ollama.NewExtractor(client, cfg.FallbackExtractionModel)
		shortTimeout := time.Duration(cfg.ExtractionTimeoutShortS) * time.Second
		longTimeout := time.Duration(cfg.ExtractionTimeoutLongS) * time.Second
		chain := openrouter.NewChainExtractor(
			openrouter.Tier{
				Name:      "openrouter-short",
				Extractor: orExtractor,
				Timeout:   shortTimeout,
			},
			openrouter.Tier{
				Name:      "openrouter-long",
				Extractor: orExtractor,
				Timeout:   longTimeout,
			},
			openrouter.Tier{
				Name:      "ollama-fallback",
				Extractor: ollamaFallback,
				Timeout:   0,
			},
		)
		return db, embedder, chain, nil
	}

	return db, embedder, ollamaExtractor, nil
}
