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

	fs.StringVar(&project, "project", "", "(required) HSME project namespace for both sources")
	fs.StringVar(&quorumProject, "quorum-project", "", "(optional) Filter for Quorum project_id in curated-memory (defaults to --project)")
	fs.StringVar(&quorumDB, "quorum-db", "", "(optional) Path to Quorum memory.db (defaults to ~/.quorum/memory.db)")
	fs.StringVar(&tasksRoot, "tasks-root", ".ai/tasks", "(optional) Path to .ai/tasks root directory")
	fs.StringVar(&source, "source", "all", "(optional) Source to import: curated|capsules|all")

	RegisterDBFlags(fs, &cfg)
	_ = fs.Parse(args)
	ScanTrailingFlags(fs)

	if project == "" {
		WriteError(os.Stderr, fmt.Errorf("missing required flag: --project"), exitUsage, outputFormat)
		os.Exit(exitUsage)
	}

	if quorumProject == "" {
		quorumProject = project
	}

	if quorumDB == "" {
		quorumDB = defaultQuorumDBPath()
	}

	if source != "curated" && source != "capsules" && source != "all" {
		WriteError(os.Stderr, fmt.Errorf("invalid source %q: must be one of curated|capsules|all", source), exitUsage, outputFormat)
		os.Exit(exitUsage)
	}

	hsmeDB, err := bootstrap.OpenDB(cfg)
	if err != nil {
		WriteError(os.Stderr, fmt.Errorf("failed to open HSME database: %w", err), exitRuntime, outputFormat)
		os.Exit(exitRuntime)
	}
	defer hsmeDB.Close()

	resultMap := make(map[string]interface{})

	if source == "curated" || source == "all" {
		qDB, err := quorumdelta.OpenReadOnly(quorumDB)
		if err != nil {
			WriteError(os.Stderr, fmt.Errorf("failed to open Quorum database: %w", err), exitRuntime, outputFormat)
			os.Exit(exitRuntime)
		}

		res, err := quorumdelta.Import(qDB, hsmeDB, quorumProject, project)
		qDB.Close()
		if err != nil {
			WriteError(os.Stderr, fmt.Errorf("curated memory import failed: %w", err), exitRuntime, outputFormat)
			os.Exit(exitRuntime)
		}
		resultMap["curated"] = res
	}

	if source == "capsules" || source == "all" {
		res, err := capsule.Import(hsmeDB, tasksRoot, project)
		if err != nil {
			WriteError(os.Stderr, fmt.Errorf("capsule import failed: %w", err), exitRuntime, outputFormat)
			os.Exit(exitRuntime)
		}
		resultMap["capsules"] = res
	}

	if outputFormat == "json" {
		WriteResult(os.Stdout, resultMap, outputFormat)
	} else {
		WriteResult(os.Stdout, FormatImportQuorumResult(resultMap), outputFormat)
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
