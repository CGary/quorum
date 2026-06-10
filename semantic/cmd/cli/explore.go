//go:build sqlite_fts5 && sqlite_vec

package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/hsme/core/src/bootstrap"
	"github.com/hsme/core/src/core/search"
)

func runExplore(args []string, cfg bootstrap.Config) {
	fs := flag.NewFlagSet("explore", flag.ExitOnError)
	var direction string
	var maxDepth int
	var maxNodes int

	fs.StringVar(&direction, "direction", "both", "Direction of exploration (upstream, downstream, both)")
	fs.IntVar(&maxDepth, "max-depth", 5, "Maximum depth of exploration")
	fs.IntVar(&maxNodes, "max-nodes", 100, "Maximum number of nodes to return")

	RegisterDBFlags(fs, &cfg)
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "error: entity_name is required")
		os.Exit(exitUsage)
	}
	entityName := fs.Arg(0)

	db, err := bootstrap.OpenDB(cfg)
	if err != nil {
		WriteError(os.Stderr, fmt.Errorf("failed to open database: %w", err), exitRuntime, outputFormat)
		os.Exit(exitRuntime)
	}
	defer db.Close()

	result, err := search.TraceDependencies(context.Background(), db, entityName, direction, maxDepth, maxNodes)
	if err != nil {
		WriteError(os.Stderr, err, exitRuntime, outputFormat)
		os.Exit(exitRuntime)
	}

	if outputFormat == "json" {
	        WriteResult(os.Stdout, result, outputFormat)
	} else {
	        WriteResult(os.Stdout, FormatExploreResult(result), outputFormat)
	}
	}

