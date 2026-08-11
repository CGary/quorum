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
	var fields string
	var agentFlags AgentFlags

	fs.StringVar(&direction, "direction", "both", "Direction of exploration (upstream, downstream, both)")
	fs.IntVar(&maxDepth, "max-depth", 5, "Maximum depth of exploration")
	fs.IntVar(&maxNodes, "max-nodes", 100, "Maximum number of nodes to return")
	fs.StringVar(&fields, "fields", "", "comma-separated subset of result fields")

	RegisterAgentFlags(fs, &agentFlags)
	RegisterDBFlags(fs, &cfg)

	fs.Parse(args)
	ScanTrailingFlags(fs)

	// 1. --schema check first
	if agentFlags.Schema {
		WriteSchemaEnvelope(os.Stdout, exploreSchema())
		os.Exit(0)
	}

	// 2. Required argument validation
	if fs.NArg() < 1 {
		emitErrorAndExit("explore", errMissingRequired, "missing required argument: entity_name", "entity-name", "", true, "hsme-cli explore <entity_name> --json", agentFlags.JSON)
	}
	entityName := fs.Arg(0)

	// 3. Direction enum validation before opening DB
	if direction != "upstream" && direction != "downstream" && direction != "both" {
		emitErrorAndExit("explore", errInvalidEnum, fmt.Sprintf("invalid direction %q: must be one of upstream|downstream|both", direction), "direction", direction, false, "hsme-cli explore "+entityName+" --direction both", agentFlags.JSON)
	}

	// 4. Open DB
	db, err := bootstrap.OpenDB(cfg)
	if err != nil {
		emitErrorAndExit("explore", errInternal, fmt.Sprintf("failed to open database: %v", err), "", "", false, "", agentFlags.JSON)
	}
	defer db.Close()

	// 5. Trace dependencies
	result, err := search.TraceDependencies(context.Background(), db, entityName, direction, maxDepth, maxNodes)
	if err != nil {
		emitErrorAndExit("explore", errInternal, err.Error(), "", "", false, "", agentFlags.JSON)
	}

	// 6. Fields projection
	var finalNodes any = result.Nodes
	var finalEdges any = result.Edges
	fieldList := parseFields(fields)
	if len(fieldList) > 0 {
		projNodes := make([]map[string]any, len(result.Nodes))
		for i, n := range result.Nodes {
			projNodes[i] = projectItem(n, fieldList)
		}
		projEdges := make([]map[string]any, len(result.Edges))
		for i, e := range result.Edges {
			projEdges[i] = projectItem(e, fieldList)
		}
		finalNodes = projNodes
		finalEdges = projEdges
	}

	res := map[string]interface{}{
		"entity":    result.Entity,
		"nodes":     finalNodes,
		"edges":     finalEdges,
		"truncated": result.Truncated,
	}

	// 7. Handle --output
	if agentFlags.Output != "" {
		if err := writeOutputFile(agentFlags.Output, res); err != nil {
			emitErrorAndExit("explore", errInternal, fmt.Sprintf("cannot write --output %s: %v", agentFlags.Output, err), "output", agentFlags.Output, false, "", agentFlags.JSON)
		}
		if agentFlags.JSON {
			WriteSuccessEnvelope(os.Stdout, SuccessEnvelope{
				OK:          true,
				Command:     "explore",
				Summary:     "results written to " + agentFlags.Output,
				Data:        map[string]any{"result_file": agentFlags.Output},
				NextActions: []NextAction{},
			})
		} else {
			fmt.Printf("results written to %s\n", agentFlags.Output)
		}
		return
	}

	// 8. Output result
	if agentFlags.JSON {
		WriteSuccessEnvelope(os.Stdout, SuccessEnvelope{
			OK:          true,
			Command:     "explore",
			Summary:     fmt.Sprintf("Exploration for: %s (Nodes: %d, Edges: %d)", result.Entity, len(result.Nodes), len(result.Edges)),
			Data:        res,
			NextActions: []NextAction{},
		})
	} else {
		fmt.Print(FormatExploreResult(result))
	}
}

func exploreSchema() map[string]any {
	return map[string]any{
		"command":     "explore",
		"description": "Trace dependencies in the knowledge graph. (Note: cursor pagination is unsupported)",
		"input": map[string]any{
			"required": []string{"entity_name"},
			"properties": map[string]any{
				"entity_name": map[string]any{"type": "string", "description": "positional name of the entity to explore"},
				"direction":   map[string]any{"type": "string", "enum": []string{"upstream", "downstream", "both"}, "default": "both", "description": "direction of exploration"},
				"max-depth":   map[string]any{"type": "integer", "default": 5, "description": "maximum depth of exploration"},
				"max-nodes":   map[string]any{"type": "integer", "default": 100, "description": "maximum number of nodes to return"},
				"fields":      map[string]any{"type": "string", "description": "comma-separated subset of result fields"},
				"output":      map[string]any{"type": "string", "description": "write full result to this file; returns data.result_file"},
			},
		},
		"output": map[string]any{"type": "object", "required": []string{"ok", "command", "summary", "data"}},
		"errors": []string{errMissingRequired, errInvalidEnum, errInternal},
	}
}
