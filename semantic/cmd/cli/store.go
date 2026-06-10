//go:build sqlite_fts5 && sqlite_vec

package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/hsme/core/src/bootstrap"
	"github.com/hsme/core/src/core/indexer"
)

func runStore(args []string, cfg bootstrap.Config) {
	fs := flag.NewFlagSet("store", flag.ExitOnError)
	var sourceType string
	var project string
	var supersedesID int64
	var forceReingest bool
	var hasSupersedes bool

	fs.StringVar(&sourceType, "source-type", "", "Type of source (required)")
	fs.StringVar(&project, "project", "", "Project name")
	fs.Int64Var(&supersedesID, "supersedes", 0, "ID of the memory this entry supersedes")
	fs.BoolVar(&forceReingest, "force-reingest", false, "Force re-processing even if content exists")

	// Register global flags to allow overriding from subcommand
	RegisterDBFlags(fs, &cfg)

	fs.Parse(args)
	ScanTrailingFlags(fs)
	// Check if supersedes was actually set
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "supersedes" {
			hasSupersedes = true
		}
	})

	if sourceType == "" {
		fmt.Fprintln(os.Stderr, "error: --source-type is required")
		fs.Usage()
		os.Exit(exitUsage)
	}

	// Read stdin
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		fmt.Println("store: no input on stdin. Usage: hsme-cli store --source-type <type> < notes.md")
		os.Exit(exitUsage)
	}

	contentBytes, err := io.ReadAll(os.Stdin)
	if err != nil {
		WriteError(os.Stderr, fmt.Errorf("failed to read stdin: %w", err), exitRuntime, outputFormat)
		os.Exit(exitRuntime)
	}
	content := string(contentBytes)
	if content == "" {
		fmt.Println("store: empty input on stdin")
		os.Exit(exitUsage)
	}

	db, _, err := bootstrap.OpenWithEmbedder(cfg)
	if err != nil {
		WriteError(os.Stderr, fmt.Errorf("failed to open database: %w", err), exitRuntime, outputFormat)
		os.Exit(exitRuntime)
	}
	defer db.Close()

	var sID *int64
	if hasSupersedes {
		sID = &supersedesID
	}

	id, err := indexer.StoreContext(db, content, sourceType, project, sID, forceReingest)
	if err != nil {
		WriteError(os.Stderr, err, exitRuntime, outputFormat)
		os.Exit(exitRuntime)
	}

	res := map[string]interface{}{
	        "memory_id": id,
	        "status":    "stored",
	}
	if outputFormat == "json" {
	        WriteResult(os.Stdout, res, outputFormat)
	} else {
	        WriteResult(os.Stdout, FormatStoreResult(res), outputFormat)
	}
	}

