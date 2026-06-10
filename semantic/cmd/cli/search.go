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

func runSearchFuzzy(args []string, cfg bootstrap.Config) {
	fs := flag.NewFlagSet("search-fuzzy", flag.ExitOnError)
	var limit int
	var project string

	fs.IntVar(&limit, "limit", 10, "Maximum number of results")
	fs.StringVar(&project, "project", "", "Filter results by project")

	RegisterDBFlags(fs, &cfg)
	fs.Parse(args)
	ScanTrailingFlags(fs)
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "error: query is required")
		os.Exit(exitUsage)
	}
	query := fs.Arg(0)

	db, embedder, err := bootstrap.OpenWithEmbedder(cfg)
	if err != nil {
		WriteError(os.Stderr, fmt.Errorf("failed to open database: %w", err), exitRuntime, outputFormat)
		os.Exit(exitRuntime)
	}
	defer db.Close()

	results, err := search.FuzzySearch(context.Background(), db, embedder, query, limit, project)
	if err != nil {
		WriteError(os.Stderr, err, exitRuntime, outputFormat)
		os.Exit(exitRuntime)
	}

	res := map[string]interface{}{
		"results": results,
	}
	if outputFormat == "json" {
		WriteResult(os.Stdout, res, outputFormat)
	} else {
		WriteResult(os.Stdout, FormatSearchResults(res), outputFormat)
	}
}

func runSearchExact(args []string, cfg bootstrap.Config) {
	fs := flag.NewFlagSet("search-exact", flag.ExitOnError)
	var limit int
	var project string

	fs.IntVar(&limit, "limit", 10, "Maximum number of results")
	fs.StringVar(&project, "project", "", "Filter results by project")

	RegisterDBFlags(fs, &cfg)
	fs.Parse(args)
	ScanTrailingFlags(fs)
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "error: keyword is required")
		os.Exit(exitUsage)
	}
	keyword := fs.Arg(0)

	db, err := bootstrap.OpenDB(cfg)
	if err != nil {
		WriteError(os.Stderr, fmt.Errorf("failed to open database: %w", err), exitRuntime, outputFormat)
		os.Exit(exitRuntime)
	}
	defer db.Close()

	results, err := search.ExactSearch(context.Background(), db, keyword, limit, project)
	if err != nil {
		WriteError(os.Stderr, err, exitRuntime, outputFormat)
		os.Exit(exitRuntime)
	}

	res := map[string]interface{}{
		"results": results,
	}
	if outputFormat == "json" {
		WriteResult(os.Stdout, res, outputFormat)
	} else {
		WriteResult(os.Stdout, FormatExactResults(res), outputFormat)
	}
}
