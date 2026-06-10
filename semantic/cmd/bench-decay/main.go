package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	"github.com/hsme/core/src/core/inference/ollama"
	"github.com/hsme/core/src/core/search"
	_ "github.com/mattn/go-sqlite3"
)

type Config struct {
	DBPath       string  `json:"db_path"`
	EvalPath     string  `json:"eval_path"`
	BaselinePath string  `json:"baseline_path"`
	HalfLife     float64 `json:"half_life_days"`
	OutputDir    string  `json:"output_dir"`
	RunID        string  `json:"run_id"`
	Queries      string  `json:"queries,omitempty"`
	NoVector     bool    `json:"no_vector"`
}

func main() {
	vec.Auto()

	var cfg Config
	flag.StringVar(&cfg.DBPath, "db", "data/engram.db", "Path to SQLite database")
	flag.StringVar(&cfg.EvalPath, "eval", "docs/future-missions/mission-3-eval-set.yaml", "Frozen eval-set JSON/YAML path")
	flag.StringVar(&cfg.BaselinePath, "baseline", "docs/future-missions/mission-3-baseline.json", "Frozen decay-off baseline JSON path")
	flag.Float64Var(&cfg.HalfLife, "half-life", 14.0, "Half-life in days for decay")
	flag.StringVar(&cfg.OutputDir, "out", "data/benchmarks", "Output directory for reports")
	flag.StringVar(&cfg.RunID, "run-id", "", "Explicit benchmark run id (defaults to UTC timestamp)")
	flag.StringVar(&cfg.Queries, "queries", "", "Optional comma-separated ad-hoc query override; by default the frozen eval set is used")
	flag.BoolVar(&cfg.NoVector, "no-vector", false, "Disable vector embeddings for offline smoke tests; production benchmarks should leave this false")
	flag.Parse()

	if cfg.HalfLife <= 0 {
		fmt.Fprintln(os.Stderr, "Error: half-life must be > 0")
		os.Exit(1)
	}

	if cfg.RunID == "" {
		cfg.RunID = time.Now().UTC().Format("20060102T150405Z")
	}

	dsn := fmt.Sprintf("file:%s?mode=ro", cfg.DBPath)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		fmt.Fprintf(os.Stderr, "Error pinging database: %v\n", err)
		os.Exit(1)
	}

	evalSet, err := loadEvalSet(cfg.EvalPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading eval set: %v\n", err)
		os.Exit(1)
	}
	if cfg.Queries != "" {
		evalSet = evalSetFromQueries(strings.Split(cfg.Queries, ","))
	}

	baseline, err := loadBaseline(cfg.BaselinePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading baseline: %v\n", err)
		os.Exit(1)
	}

	runDir := filepath.Join(cfg.OutputDir, cfg.RunID)
	if err := os.MkdirAll(runDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	var embedder search.Embedder
	if !cfg.NoVector {
		embedModel := os.Getenv("EMBEDDING_MODEL")
		if embedModel == "" {
			embedModel = "nomic-embed-text"
		}
		client := ollama.NewClient(os.Getenv("OLLAMA_HOST"))
		embedder = ollama.NewEmbedder(client, embedModel, 768)
	}

	fmt.Printf("Starting benchmark run %s with %d eval queries\n", cfg.RunID, len(evalSet.Queries))
	report, err := runEval(context.Background(), db, embedder, cfg, evalSet, baseline)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running evaluation: %v\n", err)
		os.Exit(1)
	}

	if err := writeReports(runDir, report); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing reports: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Benchmark complete. Reports saved to %s\n", runDir)
}
