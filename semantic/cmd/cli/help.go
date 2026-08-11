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
  import-quorum  Import Quorum curated memory and task capsules
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
		fmt.Print(`Usage: hsme-cli store --source-type <type> [--project <proj>] [--supersedes <id>] [--force-reingest] [--dry-run] [flags]

Ingest content from stdin into HSME.

Flags:
  --source-type string    (required) Type of source (e.g., 'code', 'note', 'log')
  --project string        (optional) Project name
  --supersedes int        (optional) ID of the memory this entry supersedes
  --force-reingest        (optional) Force re-processing even if content exists
  --dry-run               (optional) Simulate ingestion without writing to database
  --json                  Emit one JSON envelope on stdout
  --no-input              Never prompt interactively
  --quiet                 Suppress non-essential output
  --verbose               Verbose output
  --output string         Write result to this file
  --timeout int           Seconds before timeout
  --schema                Print JSON schema contract and exit
`)
	case "search-fuzzy":
		fmt.Print(`Usage: hsme-cli search-fuzzy <query> [--limit <n>] [--project <proj>] [--fields <fields>] [--output <file>] [flags]

Perform a semantic search using embeddings.

Flags:
  --limit int             (default 10) Maximum number of results
  --project string        (optional) Filter results by project
  --fields string         (optional) Comma-separated subset of result fields
  --output string         (optional) Write full result to this file
  --json                  Emit one JSON envelope on stdout
  --no-input              Never prompt interactively
  --quiet                 Suppress non-essential output
  --verbose               Verbose output
  --timeout int           Seconds before timeout
  --schema                Print JSON schema contract and exit
`)
	case "search-exact":
		fmt.Print(`Usage: hsme-cli search-exact <keyword> [--limit <n>] [--project <proj>] [--fields <fields>] [--output <file>] [flags]

Perform a lexical search for exact keywords.

Flags:
  --limit int             (default 10) Maximum number of results
  --project string        (optional) Filter results by project
  --fields string         (optional) Comma-separated subset of result fields
  --output string         (optional) Write full result to this file
  --json                  Emit one JSON envelope on stdout
  --no-input              Never prompt interactively
  --quiet                 Suppress non-essential output
  --verbose               Verbose output
  --timeout int           Seconds before timeout
  --schema                Print JSON schema contract and exit
`)
	case "explore":
		fmt.Print(`Usage: hsme-cli explore <entity-name> [--direction upstream|downstream|both] [--max-depth <n>] [--max-nodes <n>] [--fields <fields>] [--output <file>] [flags]

Trace dependencies in the knowledge graph.

Flags:
  --direction string      (default "both") Direction to trace: upstream, downstream, or both
  --max-depth int         (default 5) Maximum recursion depth
  --max-nodes int         (default 100) Maximum total nodes to return
  --fields string         (optional) Comma-separated subset of result fields
  --output string         (optional) Write full result to this file
  --json                  Emit one JSON envelope on stdout
  --no-input              Never prompt interactively
  --quiet                 Suppress non-essential output
  --verbose               Verbose output
  --timeout int           Seconds before timeout
  --schema                Print JSON schema contract and exit
`)
	case "status":
		fmt.Print(`Usage: hsme-cli status [--watch] [--interval <duration>] [flags]

Show system health, worker status, and queue metrics.

Flags:
  --watch                 Update status periodically (requires TTY)
  --interval duration     (default 2s) Update interval in watch mode
  --output string         Write result to this file
  --json                  Emit one JSON envelope on stdout
  --no-input              Never prompt interactively
  --quiet                 Suppress non-essential output
  --verbose               Verbose output
  --timeout int           Seconds before timeout
  --schema                Print JSON schema contract and exit
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
		fmt.Print(`Usage: hsme-cli admin retry-failed [flags]

Re-queue all tasks in 'failed' state.

Flags:
  --output string         Write result to this file
  --json                  Emit one JSON envelope on stdout
  --no-input              Never prompt interactively
  --quiet                 Suppress non-essential output
  --verbose               Verbose output
  --timeout int           Seconds before timeout
  --schema                Print JSON schema contract and exit
`)
	case "admin backup":
		fmt.Print(`Usage: hsme-cli admin backup [--dest <path>] [flags]

Create a backup of the current database.

Flags:
  --dest string           (optional) Path to save the backup. Defaults to backups/engram-<timestamp>.db
  --output string         Write result to this file
  --json                  Emit one JSON envelope on stdout
  --no-input              Never prompt interactively
  --quiet                 Suppress non-essential output
  --verbose               Verbose output
  --timeout int           Seconds before timeout
  --schema                Print JSON schema contract and exit
`)
	case "admin restore":
		fmt.Print(`Usage: hsme-cli admin restore (--from <path> | --latest) [--dry-run] [flags]

Restore the database from a backup.

Flags:
  --from string           Path to the backup file
  --latest                Use the most recent backup in the backups/ directory
  --dry-run               (optional) Simulate restore without modifying database
  --output string         Write result to this file
  --json                  Emit one JSON envelope on stdout
  --no-input              Never prompt interactively
  --quiet                 Suppress non-essential output
  --verbose               Verbose output
  --timeout int           Seconds before timeout
  --schema                Print JSON schema contract and exit
`)
	case "import-quorum":
		fmt.Print(`Usage: hsme-cli import-quorum --project <proj> [--quorum-project <qproj>] [--quorum-db <path>] [--tasks-root <path>] [--source curated|capsules|all] [--dry-run] [flags]

Import Quorum curated memory and task capsules into HSME.

Flags:
  --project string        (required) HSME project namespace for both sources
  --quorum-project string (optional) Filter for Quorum project_id in curated-memory (defaults to --project)
  --quorum-db string      (optional) Path to Quorum memory.db (defaults to ~/.quorum/memory.db)
  --tasks-root string     (optional) Path to .ai/tasks root directory (defaults to .ai/tasks)
  --source string         (optional) Source to import: curated|capsules|all (defaults to all)
  --dry-run               (optional) Simulate import without writing to database
  --output string         Write result to this file
  --json                  Emit one JSON envelope on stdout
  --no-input              Never prompt interactively
  --quiet                 Suppress non-essential output
  --verbose               Verbose output
  --timeout int           Seconds before timeout
  --schema                Print JSON schema contract and exit

Note (ADR 0013 section 4): The capsule corpus is read from gitignored .ai/tasks state and cannot be rebuilt from a clean clone, so a fresh checkout will see zero capsules until archived tasks accumulate again — this is accepted, not a bug.
`)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand for help: %s\n\n", subcommand)
		printTopLevelHelp()
		os.Exit(1)
	}
}
