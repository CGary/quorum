//go:build sqlite_fts5 && sqlite_vec

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/hsme/core/src/bootstrap"
	"github.com/hsme/core/src/core/indexer"
)

func runStore(args []string, cfg bootstrap.Config) {
	fs := flag.NewFlagSet("store", flag.ExitOnError)
	var sourceType string
	var project string
	var supersedesID int64
	var forceReingest bool
	var dryRun bool
	var hasSupersedes bool
	var agentFlags AgentFlags

	fs.StringVar(&sourceType, "source-type", "", "Type of source (required)")
	fs.StringVar(&project, "project", "", "Project name")
	fs.Int64Var(&supersedesID, "supersedes", 0, "ID of the memory this entry supersedes")
	fs.BoolVar(&forceReingest, "force-reingest", false, "Force re-processing even if content exists")
	fs.BoolVar(&dryRun, "dry-run", false, "simulate ingestion without writing to database")

	RegisterAgentFlags(fs, &agentFlags)
	RegisterDBFlags(fs, &cfg)

	fs.Parse(args)
	ScanTrailingFlags(fs)

	// 1. --schema check first
	if agentFlags.Schema {
		WriteSchemaEnvelope(os.Stdout, storeSchema())
		os.Exit(0)
	}

	// Check if supersedes was actually set
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "supersedes" {
			hasSupersedes = true
		}
	})

	// 2. Required-flag validation
	if sourceType == "" {
		emitErrorAndExit("store", errMissingRequired, "missing required flag: --source-type", "source-type", "", true, "hsme-cli store --source-type <type> < notes.md", agentFlags.JSON)
	}

	// 3. Read stdin
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		emitErrorAndExit("store", errValidationFailed, "store: no input on stdin. Usage: hsme-cli store --source-type <type> < notes.md", "stdin", "", true, "hsme-cli store --source-type <type> < notes.md", agentFlags.JSON)
	}

	contentBytes, err := io.ReadAll(os.Stdin)
	if err != nil {
		emitErrorAndExit("store", errInternal, fmt.Sprintf("failed to read stdin: %v", err), "stdin", "", false, "", agentFlags.JSON)
	}
	content := string(contentBytes)
	if content == "" {
		emitErrorAndExit("store", errValidationFailed, "store: empty input on stdin", "stdin", "", true, "hsme-cli store --source-type <type> < notes.md", agentFlags.JSON)
	}

	// 4. --dry-run check
	if dryRun {
		data := map[string]any{
			"dry_run":         true,
			"source_type":     sourceType,
			"project":         project,
			"content_bytes":   len(contentBytes),
			"would_supersede": hasSupersedes,
		}
		if agentFlags.JSON {
			WriteSuccessEnvelope(os.Stdout, SuccessEnvelope{
				OK:          true,
				Command:     "store",
				Summary:     fmt.Sprintf("dry-run: would store %d bytes with source_type %s", len(contentBytes), sourceType),
				Data:        data,
				NextActions: []NextAction{},
			})
		} else {
			fmt.Printf("dry-run: would store %d bytes (source_type: %s, project: %s, would_supersede: %v)\n", len(contentBytes), sourceType, project, hasSupersedes)
		}
		os.Exit(0)
	}

	// 5. Open DB
	db, _, err := bootstrap.OpenWithEmbedder(cfg)
	if err != nil {
		emitErrorAndExit("store", errInternal, fmt.Sprintf("failed to open database: %v", err), "", "", false, "", agentFlags.JSON)
	}
	defer db.Close()

	var sID *int64
	if hasSupersedes {
		sID = &supersedesID
	}

	// 6. Store context
	id, err := indexer.StoreContext(db, content, sourceType, project, sID, forceReingest)
	if err != nil {
		if strings.HasPrefix(err.Error(), "DUPLICATE_CONTENT:") {
			emitErrorAndExit("store", errConflict, err.Error(), "supersedes", "", true, "hsme-cli store --source-type "+sourceType+" --supersedes <id>", agentFlags.JSON)
		}
		emitErrorAndExit("store", errInternal, err.Error(), "", "", false, "", agentFlags.JSON)
	}

	res := map[string]interface{}{
		"memory_id": id,
		"status":    "stored",
	}

	if agentFlags.Output != "" {
		if err := writeOutputFile(agentFlags.Output, res); err != nil {
			emitErrorAndExit("store", errInternal, fmt.Sprintf("cannot write --output %s: %v", agentFlags.Output, err), "output", agentFlags.Output, false, "", agentFlags.JSON)
		}
		if agentFlags.JSON {
			WriteSuccessEnvelope(os.Stdout, SuccessEnvelope{
				OK:          true,
				Command:     "store",
				Summary:     "result written to " + agentFlags.Output,
				Data:        map[string]any{"result_file": agentFlags.Output},
				NextActions: []NextAction{},
			})
		} else {
			fmt.Printf("result written to %s\n", agentFlags.Output)
		}
		return
	}

	if agentFlags.JSON {
		WriteSuccessEnvelope(os.Stdout, SuccessEnvelope{
			OK:          true,
			Command:     "store",
			Summary:     fmt.Sprintf("Memory stored successfully. ID: %v", id),
			Data:        res,
			NextActions: []NextAction{},
		})
	} else {
		fmt.Println(FormatStoreResult(res))
	}
}

func storeSchema() map[string]any {
	return map[string]any{
		"command":     "store",
		"description": "Ingest content from stdin into HSME.",
		"input": map[string]any{
			"required": []string{"source-type"},
			"properties": map[string]any{
				"source-type":    map[string]any{"type": "string", "description": "type of source (e.g., 'code', 'note', 'log')"},
				"project":        map[string]any{"type": "string", "description": "project name"},
				"supersedes":     map[string]any{"type": "integer", "description": "ID of the memory this entry supersedes"},
				"force-reingest": map[string]any{"type": "boolean", "description": "force re-processing even if content exists"},
				"dry-run":        map[string]any{"type": "boolean", "description": "simulate ingestion without writing to database"},
				"output":         map[string]any{"type": "string", "description": "write result to this file"},
			},
		},
		"output": map[string]any{"type": "object", "required": []string{"ok", "command", "summary", "data"}},
		"errors": []string{errMissingRequired, errValidationFailed, errConflict, errInternal},
	}
}
