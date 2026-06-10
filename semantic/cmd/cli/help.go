//go:build sqlite_fts5 && sqlite_vec

package main

import (
	"fmt"
	"os"
)

func printTopLevelHelp() {
	fmt.Print(`HSME CLI — Unified command-line interface for HSME

Usage: hsme-cli <subcommand> [flags]

Subcommands:
  store          Ingest content from stdin
  search-fuzzy   Semantic search
  search-exact   Keyword search
  explore        Trace graph dependencies
  status         Show system health
  admin          Admin operations (backup, restore, retry-failed)
  help           Show this help or help for a specific subcommand

Use "hsme-cli help <subcommand>" for detailed usage.
`)
}

func runHelp(args []string) {
	if len(args) == 0 {
		printTopLevelHelp()
		return
	}

	subcommand := args[0]
	switch subcommand {
	case "store":
		fmt.Print(`Usage: hsme-cli store --source-type <type> [--project <proj>] [--supersedes <id>] [--force-reingest]

Ingest content from stdin into HSME.

Flags:
  --source-type string    (required) Type of source (e.g., 'code', 'note', 'log')
  --project string        (optional) Project name
  --supersedes int        (optional) ID of the memory this entry supersedes
  --force-reingest        (optional) Force re-processing even if content exists
`)
	case "search-fuzzy":
		fmt.Print(`Usage: hsme-cli search-fuzzy <query> [--limit <n>] [--project <proj>]

Perform a semantic search using embeddings.

Flags:
  --limit int             (default 10) Maximum number of results
  --project string        (optional) Filter results by project
`)
	case "search-exact":
		fmt.Print(`Usage: hsme-cli search-exact <keyword> [--limit <n>] [--project <proj>]

Perform a lexical search for exact keywords.

Flags:
  --limit int             (default 10) Maximum number of results
  --project string        (optional) Filter results by project
`)
	case "explore":
		fmt.Print(`Usage: hsme-cli explore <entity-name> [--direction upstream|downstream|both] [--max-depth <n>] [--max-nodes <n>]

Trace dependencies in the knowledge graph.

Flags:
  --direction string      (default "both") Direction to trace: upstream, downstream, or both
  --max-depth int         (default 5) Maximum recursion depth
  --max-nodes int         (default 100) Maximum total nodes to return
`)
	case "status":
		fmt.Print(`Usage: hsme-cli status [--watch] [--interval <duration>]

Show system health, worker status, and queue metrics.

Flags:
  --watch                 Update status periodically (requires TTY)
  --interval duration     (default 2s) Update interval in watch mode
`)
	case "admin":
		fmt.Print(`Usage: hsme-cli admin <subcommand> [flags]

Administrative operations.

Subcommands:
  retry-failed            Re-queue failed tasks
  backup                  Create a database backup
  restore                 Restore from a backup
`)
	case "admin retry-failed":
		fmt.Print(`Usage: hsme-cli admin retry-failed

Re-queue all tasks in 'failed' state.
`)
	case "admin backup":
		fmt.Print(`Usage: hsme-cli admin backup [--out <path>]

Create a backup of the current database.

Flags:
  --out string            (optional) Path to save the backup. Defaults to backups/engram-<timestamp>.db
`)
	case "admin restore":
		fmt.Print(`Usage: hsme-cli admin restore (--from <path> | --latest)

Restore the database from a backup.

Flags:
  --from string           Path to the backup file
  --latest                Use the most recent backup in the backups/ directory
`)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand for help: %s\n\n", subcommand)
		printTopLevelHelp()
		os.Exit(1)
	}
}
