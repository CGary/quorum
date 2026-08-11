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
	var fields string
	var agentFlags AgentFlags

	fs.IntVar(&limit, "limit", 10, "Maximum number of results")
	fs.StringVar(&project, "project", "", "Filter results by project")
	fs.StringVar(&fields, "fields", "", "comma-separated subset of result fields")

	RegisterAgentFlags(fs, &agentFlags)
	RegisterDBFlags(fs, &cfg)

	fs.Parse(args)
	ScanTrailingFlags(fs)

	// 1. --schema check first
	if agentFlags.Schema {
		WriteSchemaEnvelope(os.Stdout, searchFuzzySchema())
		os.Exit(0)
	}

	// 2. Required argument validation
	if fs.NArg() < 1 {
		emitErrorAndExit("search-fuzzy", errMissingRequired, "missing required argument: query", "query", "", true, "hsme-cli search-fuzzy <query> --json", agentFlags.JSON)
	}
	query := fs.Arg(0)

	// 3. Open DB with embedder
	db, embedder, err := bootstrap.OpenWithEmbedder(cfg)
	if err != nil {
		emitErrorAndExit("search-fuzzy", errInternal, fmt.Sprintf("failed to open database: %v", err), "", "", false, "", agentFlags.JSON)
	}
	defer db.Close()

	// 4. Run fuzzy search
	results, err := search.FuzzySearch(context.Background(), db, embedder, query, limit, project)
	if err != nil {
		emitErrorAndExit("search-fuzzy", errInternal, err.Error(), "", "", false, "", agentFlags.JSON)
	}

	// 5. Fields projection
	var finalResults any = results
	fieldList := parseFields(fields)
	if len(fieldList) > 0 {
		finalResults = projectSlice(results, fieldList)
	}

	res := map[string]interface{}{
		"results": finalResults,
	}

	// 6. Handle --output
	if agentFlags.Output != "" {
		if err := writeOutputFile(agentFlags.Output, res); err != nil {
			emitErrorAndExit("search-fuzzy", errInternal, fmt.Sprintf("cannot write --output %s: %v", agentFlags.Output, err), "output", agentFlags.Output, false, "", agentFlags.JSON)
		}
		if agentFlags.JSON {
			WriteSuccessEnvelope(os.Stdout, SuccessEnvelope{
				OK:          true,
				Command:     "search-fuzzy",
				Summary:     "results written to " + agentFlags.Output,
				Data:        map[string]any{"result_file": agentFlags.Output},
				NextActions: []NextAction{},
			})
		} else {
			fmt.Printf("results written to %s\n", agentFlags.Output)
		}
		return
	}

	// 7. Output result
	if agentFlags.JSON {
		WriteSuccessEnvelope(os.Stdout, SuccessEnvelope{
			OK:          true,
			Command:     "search-fuzzy",
			Summary:     fmt.Sprintf("Found %d results", len(results)),
			Data:        res,
			NextActions: []NextAction{},
		})
	} else {
		fmt.Print(FormatSearchResults(map[string]interface{}{"results": results}))
	}
}

func runSearchExact(args []string, cfg bootstrap.Config) {
	fs := flag.NewFlagSet("search-exact", flag.ExitOnError)
	var limit int
	var project string
	var fields string
	var agentFlags AgentFlags

	fs.IntVar(&limit, "limit", 10, "Maximum number of results")
	fs.StringVar(&project, "project", "", "Filter results by project")
	fs.StringVar(&fields, "fields", "", "comma-separated subset of result fields")

	RegisterAgentFlags(fs, &agentFlags)
	RegisterDBFlags(fs, &cfg)

	fs.Parse(args)
	ScanTrailingFlags(fs)

	// 1. --schema check first
	if agentFlags.Schema {
		WriteSchemaEnvelope(os.Stdout, searchExactSchema())
		os.Exit(0)
	}

	// 2. Required argument validation
	if fs.NArg() < 1 {
		emitErrorAndExit("search-exact", errMissingRequired, "missing required argument: keyword", "query", "", true, "hsme-cli search-exact <keyword> --json", agentFlags.JSON)
	}
	keyword := fs.Arg(0)

	// 3. Open DB
	db, err := bootstrap.OpenDB(cfg)
	if err != nil {
		emitErrorAndExit("search-exact", errInternal, fmt.Sprintf("failed to open database: %v", err), "", "", false, "", agentFlags.JSON)
	}
	defer db.Close()

	// 4. Run exact search
	results, err := search.ExactSearch(context.Background(), db, keyword, limit, project)
	if err != nil {
		emitErrorAndExit("search-exact", errInternal, err.Error(), "", "", false, "", agentFlags.JSON)
	}

	// 5. Fields projection
	var finalResults any = results
	fieldList := parseFields(fields)
	if len(fieldList) > 0 {
		finalResults = projectSlice(results, fieldList)
	}

	res := map[string]interface{}{
		"results": finalResults,
	}

	// 6. Handle --output
	if agentFlags.Output != "" {
		if err := writeOutputFile(agentFlags.Output, res); err != nil {
			emitErrorAndExit("search-exact", errInternal, fmt.Sprintf("cannot write --output %s: %v", agentFlags.Output, err), "output", agentFlags.Output, false, "", agentFlags.JSON)
		}
		if agentFlags.JSON {
			WriteSuccessEnvelope(os.Stdout, SuccessEnvelope{
				OK:          true,
				Command:     "search-exact",
				Summary:     "results written to " + agentFlags.Output,
				Data:        map[string]any{"result_file": agentFlags.Output},
				NextActions: []NextAction{},
			})
		} else {
			fmt.Printf("results written to %s\n", agentFlags.Output)
		}
		return
	}

	// 7. Output result
	if agentFlags.JSON {
		WriteSuccessEnvelope(os.Stdout, SuccessEnvelope{
			OK:          true,
			Command:     "search-exact",
			Summary:     fmt.Sprintf("Found %d exact matches", len(results)),
			Data:        res,
			NextActions: []NextAction{},
		})
	} else {
		fmt.Print(FormatExactResults(map[string]interface{}{"results": results}))
	}
}

func searchFuzzySchema() map[string]any {
	return map[string]any{
		"command":     "search-fuzzy",
		"description": "Perform a semantic search using embeddings. (Note: cursor pagination is unsupported)",
		"input": map[string]any{
			"required": []string{"query"},
			"properties": map[string]any{
				"query":   map[string]any{"type": "string", "description": "positional search text"},
				"limit":   map[string]any{"type": "integer", "default": 10, "description": "maximum number of results"},
				"project": map[string]any{"type": "string", "description": "filter results by project"},
				"fields":  map[string]any{"type": "string", "description": "comma-separated subset of result fields"},
				"output":  map[string]any{"type": "string", "description": "write full result to this file; returns data.result_file"},
			},
		},
		"output": map[string]any{"type": "object", "required": []string{"ok", "command", "summary", "data"}},
		"errors": []string{errMissingRequired, errInternal, errNetworkError},
	}
}

func searchExactSchema() map[string]any {
	return map[string]any{
		"command":     "search-exact",
		"description": "Perform a lexical search for exact keywords. (Note: cursor pagination is unsupported)",
		"input": map[string]any{
			"required": []string{"keyword"},
			"properties": map[string]any{
				"keyword": map[string]any{"type": "string", "description": "positional keyword text"},
				"limit":   map[string]any{"type": "integer", "default": 10, "description": "maximum number of results"},
				"project": map[string]any{"type": "string", "description": "filter results by project"},
				"fields":  map[string]any{"type": "string", "description": "comma-separated subset of result fields"},
				"output":  map[string]any{"type": "string", "description": "write full result to this file; returns data.result_file"},
			},
		},
		"output": map[string]any{"type": "object", "required": []string{"ok", "command", "summary", "data"}},
		"errors": []string{errMissingRequired, errInternal},
	}
}
