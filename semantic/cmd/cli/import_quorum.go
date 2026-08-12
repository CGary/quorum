//go:build sqlite_fts5 && sqlite_vec

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hsme/core/src/bootstrap"
	"github.com/hsme/core/src/core/capsule"
	"github.com/hsme/core/src/core/quorumdelta"
)

func defaultQuorumDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".quorum", "memory.db")
	}
	return filepath.Join(home, ".quorum", "memory.db")
}

func runImportQuorum(args []string, cfg bootstrap.Config) {
	fs := flag.NewFlagSet("import-quorum", flag.ExitOnError)

	var project string
	var quorumProject string
	var quorumDB string
	var tasksRoot string
	var source string
	var dryRun bool
	var agentFlags AgentFlags

	fs.StringVar(&project, "project", "", "(required) HSME project namespace for both sources ('*' imports all Quorum projects, tagging each by source project_id)")
	fs.StringVar(&quorumProject, "quorum-project", "", "(optional) Filter for Quorum project_id in curated-memory (defaults to --project)")
	fs.StringVar(&quorumDB, "quorum-db", "", "(optional) Path to Quorum memory.db (defaults to ~/.quorum/memory.db)")
	fs.StringVar(&tasksRoot, "tasks-root", ".ai/tasks", "(optional) Path to .ai/tasks root directory")
	fs.StringVar(&source, "source", "all", "(optional) Source to import: curated|capsules|all")
	fs.BoolVar(&dryRun, "dry-run", false, "simulate import without writing to database")

	RegisterAgentFlags(fs, &agentFlags)
	RegisterDBFlags(fs, &cfg)
	_ = fs.Parse(args)
	ScanTrailingFlags(fs)

	// 1. --schema check first
	if agentFlags.Schema {
		WriteSchemaEnvelope(os.Stdout, importQuorumSchema())
		os.Exit(0)
	}

	// 2. Required flag validation
	if project == "" {
		emitErrorAndExit("import-quorum", errMissingRequired, "missing required flag: --project", "project", "", true, "hsme-cli import-quorum --project <proj>", agentFlags.JSON)
	}

	if quorumProject == "" {
		quorumProject = project
	}

	if quorumDB == "" {
		quorumDB = defaultQuorumDBPath()
	}

	if source != "curated" && source != "capsules" && source != "all" {
		emitErrorAndExit("import-quorum", errInvalidEnum, fmt.Sprintf("invalid source %q: must be one of curated|capsules|all", source), "source", source, false, "hsme-cli import-quorum --project "+project+" --source all", agentFlags.JSON)
	}

	// 3. Open HSME DB
	hsmeDB, err := bootstrap.OpenDB(cfg)
	if err != nil {
		emitErrorAndExit("import-quorum", errInternal, fmt.Sprintf("failed to open HSME database: %v", err), "", "", false, "", agentFlags.JSON)
	}
	defer hsmeDB.Close()

	// 4. --dry-run check
	if dryRun {
		data := map[string]any{
			"dry_run":   true,
			"source":    source,
			"quorum_db": quorumDB,
			"project":   project,
		}
		if agentFlags.JSON {
			WriteSuccessEnvelope(os.Stdout, SuccessEnvelope{
				OK:          true,
				Command:     "import-quorum",
				Summary:     fmt.Sprintf("dry-run: would import %s from %s into project %s", source, quorumDB, project),
				Data:        data,
				NextActions: []NextAction{},
			})
		} else {
			fmt.Printf("dry-run: would import %s from %s into project %s\n", source, quorumDB, project)
		}
		os.Exit(0)
	}

	resultMap := make(map[string]interface{})

	// 5. Curated import
	if source == "curated" || source == "all" {
		if _, err := os.Stat(quorumDB); err != nil {
			emitErrorAndExit("import-quorum", errFileNotFound, fmt.Sprintf("quorum database does not exist: %s", quorumDB), "quorum-db", quorumDB, false, "", agentFlags.JSON)
		}

		qDB, err := quorumdelta.OpenReadOnly(quorumDB)
		if err != nil {
			emitErrorAndExit("import-quorum", errInternal, fmt.Sprintf("failed to open Quorum database: %v", err), "", "", false, "", agentFlags.JSON)
		}

		res, err := quorumdelta.Import(qDB, hsmeDB, quorumProject, project)
		qDB.Close()
		if err != nil {
			emitErrorAndExit("import-quorum", errInternal, fmt.Sprintf("curated memory import failed: %v", err), "", "", false, "", agentFlags.JSON)
		}
		resultMap["curated"] = res
	}

	// 6. Capsule import
	if source == "capsules" || source == "all" {
		res, err := capsule.Import(hsmeDB, tasksRoot, project)
		if err != nil {
			emitErrorAndExit("import-quorum", errInternal, fmt.Sprintf("capsule import failed: %v", err), "", "", false, "", agentFlags.JSON)
		}
		resultMap["capsules"] = res
	}

	// 7. Output result
	if agentFlags.Output != "" {
		if err := writeOutputFile(agentFlags.Output, resultMap); err != nil {
			emitErrorAndExit("import-quorum", errInternal, fmt.Sprintf("cannot write --output %s: %v", agentFlags.Output, err), "output", agentFlags.Output, false, "", agentFlags.JSON)
		}
		if agentFlags.JSON {
			WriteSuccessEnvelope(os.Stdout, SuccessEnvelope{
				OK:          true,
				Command:     "import-quorum",
				Summary:     "results written to " + agentFlags.Output,
				Data:        map[string]any{"result_file": agentFlags.Output},
				NextActions: []NextAction{},
			})
		} else {
			fmt.Printf("results written to %s\n", agentFlags.Output)
		}
		return
	}

	if agentFlags.JSON {
		WriteSuccessEnvelope(os.Stdout, SuccessEnvelope{
			OK:          true,
			Command:     "import-quorum",
			Summary:     "Quorum import completed",
			Data:        resultMap,
			NextActions: []NextAction{},
		})
	} else {
		fmt.Print(FormatImportQuorumResult(resultMap))
	}
}

func FormatImportQuorumResult(res map[string]interface{}) string {
	out := "Import Quorum Summary:\n"
	if cur, ok := res["curated"]; ok {
		if cRes, ok := cur.(*quorumdelta.ImportResult); ok {
			out += fmt.Sprintf("  Curated Memory: %d fetched, %d ingested, %d skipped, %d errored\n",
				cRes.Fetched, cRes.Ingested, cRes.Skipped, cRes.Errored)
		} else if m, ok := cur.(map[string]interface{}); ok {
			out += fmt.Sprintf("  Curated Memory: %v fetched, %v ingested, %v skipped, %v errored\n",
				m["Fetched"], m["Ingested"], m["Skipped"], m["Errored"])
		}
	}
	if capRes, ok := res["capsules"]; ok {
		if cRes, ok := capRes.(*capsule.ImportResult); ok {
			out += fmt.Sprintf("  Capsules:       %d scanned, %d ingested, %d skipped, %d errored\n",
				cRes.Scanned, cRes.Ingested, cRes.Skipped, cRes.Errored)
		} else if m, ok := capRes.(map[string]interface{}); ok {
			out += fmt.Sprintf("  Capsules:       %v scanned, %v ingested, %v skipped, %v errored\n",
				m["Scanned"], m["Ingested"], m["Skipped"], m["Errored"])
		}
	}
	return out
}

func importQuorumSchema() map[string]any {
	return map[string]any{
		"command":     "import-quorum",
		"description": "Import Quorum curated memory and task capsules into HSME.",
		"input": map[string]any{
			"required": []string{"project"},
			"properties": map[string]any{
				"project":        map[string]any{"type": "string", "description": "HSME project namespace for both sources ('*' imports all Quorum projects, tagging each by source project_id)"},
				"quorum-project": map[string]any{"type": "string", "description": "filter for Quorum project_id in curated-memory (defaults to --project)"},
				"quorum-db":      map[string]any{"type": "string", "description": "path to Quorum memory.db (defaults to ~/.quorum/memory.db)"},
				"tasks-root":     map[string]any{"type": "string", "default": ".ai/tasks", "description": "path to .ai/tasks root directory"},
				"source":         map[string]any{"type": "string", "enum": []string{"curated", "capsules", "all"}, "default": "all", "description": "source to import"},
				"dry-run":        map[string]any{"type": "boolean", "description": "simulate import without writing to database"},
				"output":         map[string]any{"type": "string", "description": "write result to this file"},
			},
		},
		"output": map[string]any{"type": "object", "required": []string{"ok", "command", "summary", "data"}},
		"errors": []string{errMissingRequired, errInvalidEnum, errFileNotFound, errInternal},
	}
}
