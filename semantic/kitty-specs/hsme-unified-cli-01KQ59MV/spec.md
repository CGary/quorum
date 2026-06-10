# Specification: HSME Unified CLI

**Mission ID**: `01KQ59MVCJZ7EPETGSGG04VR54`
**Mission Slug**: `hsme-unified-cli-01KQ59MV`
**Mission Type**: software-dev
**Status**: Draft
**Created**: 2026-04-26
**Target Branch**: `main`

## Purpose

### TLDR

Unified `hsme-cli` binary that exposes MCP tools, status inspection, and admin operations from the terminal — replacing fragmented shell scripts with typed, testable Go subcommands.

### Stakeholder Context

The HSME ecosystem is fragmented today. Agents query memory through the MCP protocol, but operators have no terminal-native way to inspect or manage the system. They depend on a mix of bash scripts (`scripts/status.sh`, `scripts/backup_hot.sh`) and inline shell logic in the `justfile` (`backup`, `restore`, `retry-failed`) for everyday diagnostics, recovery, and queue maintenance.

This fragmentation has three concrete costs:

1. **Operator friction**: There is no way to perform an ad-hoc memory query (e.g. "what did we say about Ollama last week?") from the terminal without spinning up an MCP agent.
2. **Shell fragility**: Critical operations like backup and restore depend on shell pipelines (`ls -t | head -1`, `sqlite3 .backup`, `awk` arithmetic) that are hard to test, easy to break, and tied to Linux/macOS toolchains.
3. **Drift between binaries**: Each existing binary (`hsme`, `hsme-worker`, `hsme-ops`) duplicates ~25 lines of database/embedder initialization, and a fourth binary would compound the problem.

This mission delivers a single typed Go binary that absorbs all four MCP tools, the status dashboard, and the three operational commands (backup / restore / retry-failed), backed by a shared bootstrap package consumed by every binary in the repo.

## User Scenarios & Testing

### Primary Scenario — Operator inspects system health from terminal

**Actor**: HSME operator
**Trigger**: Operator wants a quick read on system state (worker, queue, graph) without an MCP agent.
**Steps**:
1. Operator runs `hsme-cli status`.
2. CLI reports worker state (online/offline), queue counts (pending/processing/completed/failed), and graph counts (nodes, edges).
3. Operator optionally adds `--watch` to refresh every N seconds.
4. Operator optionally adds `--format=json` to pipe to other tooling.

**Success outcome**: Operator gets the same information today's `scripts/status.sh` provides, plus structured JSON output, without any bash dependency.

### Secondary Scenario — Engineer scripts a recurring memory query

**Actor**: Engineer / cron job
**Trigger**: Need to check stored context programmatically (cron, git hook, ops script).
**Steps**:
1. Script runs `hsme-cli search-exact "deploy" --format=json --limit 10`.
2. CLI returns structured results.
3. Downstream tooling consumes JSON via `jq` or similar.

**Success outcome**: Memory becomes scriptable from any terminal-based automation without speaking JSON-RPC.

### Tertiary Scenario — Operator performs emergency recovery

**Actor**: HSME operator after data loss / corruption.
**Trigger**: Database is unusable, latest known-good backup must be restored.
**Steps**:
1. Operator runs `hsme-cli admin restore --latest`.
2. CLI selects the most recent backup, verifies its integrity, then atomically replaces the active DB.
3. CLI reports which backup was used and confirms the restore.

**Success outcome**: Recovery completes with verified integrity, without operator having to remember backup paths or run integrity checks manually.

### Quaternary Scenario — Engineer ingests a large note from a file

**Actor**: Engineer wanting to seed memory with prepared context.
**Trigger**: A markdown file or output of another tool needs to land as a memory.
**Steps**:
1. Engineer runs `hsme-cli store --source-type note < notes/decision-log.md`.
2. CLI reads stdin, calls the same core indexer the MCP path uses, returns the new memory ID.

**Success outcome**: Pipe-friendly ingestion without needing the MCP wrapper.

### Edge Cases

- **Subcommand needs embedder, Ollama is down**: CLI fails with a clear error before any DB work, and only when the subcommand actually requires the embedder.
- **`store` invoked from a TTY without redirection**: CLI detects the missing pipe and prints a usage hint instead of hanging on stdin.
- **Concurrent MCP server during write subcommand**: SQLite WAL mode coordinates writes; no extra locking expected, but documented.
- **Backup directory does not exist**: CLI creates it.
- **Restore with no backups present**: CLI exits with a clear error and a non-zero exit code.
- **Backup file fails integrity check**: CLI refuses to overwrite the live DB and reports which backup is corrupt.
- **Help on unknown subcommand**: CLI lists available subcommands and exits with usage error code.
- **JSON output requested but a runtime error occurs**: Error is also emitted as JSON to stderr so scripts can parse it.

## Domain Language

| Term | Canonical meaning | Notes |
|------|------------------|-------|
| **Subcommand** | A named action under `hsme-cli` (e.g. `store`, `status`, `admin backup`) | Not "command", to disambiguate from the binary itself |
| **Bootstrap** | Shared initialization of DB connection and (optionally) embedder | New package introduced by this mission |
| **MCP tool** | One of the four memory operations exposed today over JSON-RPC | `store_context`, `search_fuzzy`, `search_exact`, `explore_knowledge_graph` |
| **Admin operation** | Maintenance action against the database (retry-failed, backup, restore) | Grouped under the `admin` subcommand namespace |

## Functional Requirements

### Core dispatcher and help

| ID | Requirement | Status |
|----|-------------|--------|
| FR-001 | The system must expose a single `hsme-cli` binary invokable from the terminal that routes to subcommands. | Required |
| FR-002 | Running `hsme-cli` with no arguments must print top-level help and exit with usage error code. | Required |
| FR-003 | The system must support `hsme-cli help` and `hsme-cli help <subcommand>` for top-level and per-subcommand help text. | Required |
| FR-004 | Every subcommand must accept `--help`/`-h` and emit subcommand-specific usage. | Required |
| FR-005 | Help output for unknown subcommands must list the available subcommands and exit with usage error code. | Required |

### MCP tool subcommands

| ID | Requirement | Status |
|----|-------------|--------|
| FR-010 | The system must expose `hsme-cli store` that ingests content from stdin and accepts `--source-type`, `--supersedes`, `--force-reingest`. | Required |
| FR-011 | The system must expose `hsme-cli search-fuzzy` that runs a semantic search and accepts `--limit`. | Required |
| FR-012 | The system must expose `hsme-cli search-exact` that runs a lexical/keyword search and accepts `--limit`. | Required |
| FR-013 | The system must expose `hsme-cli explore` that traces graph dependencies and accepts `--direction`, `--max-depth`, `--max-nodes`. | Required |
| FR-014 | All four MCP-tool subcommands must invoke the existing core functions without altering their signatures. | Required |

### Status subcommand

| ID | Requirement | Status |
|----|-------------|--------|
| FR-020 | The system must expose `hsme-cli status` that reports worker state, async-task queue counts, and graph node/edge counts. | Required |
| FR-021 | `hsme-cli status --watch` must refresh the dashboard on a configurable interval until interrupted. | Required |
| FR-022 | `hsme-cli status --interval <duration>` must control the refresh rate (default 2s). | Required |
| FR-023 | The status subcommand must replicate the information today shown by `scripts/status.sh` at parity or better. | Required |

### Admin subcommands

| ID | Requirement | Status |
|----|-------------|--------|
| FR-030 | The system must expose `hsme-cli admin retry-failed` that requeues failed and exhausted tasks and reports the count of affected rows. | Required |
| FR-031 | The system must expose `hsme-cli admin backup` that produces a WAL-safe atomic backup of the active database. | Required |
| FR-032 | `hsme-cli admin backup --out <path>` must write the backup to the specified path; default is timestamped under `backups/`. | Required |
| FR-033 | The system must expose `hsme-cli admin restore` with mutually exclusive selectors `--from <path>` and `--latest`. | Required |
| FR-034 | The restore operation must verify backup integrity before replacing the active database. | Required |
| FR-035 | The restore operation must clean WAL/SHM sidecars and atomically replace the DB so a partial failure cannot leave the system in a broken state. | Required |
| FR-036 | Restore must exit with non-zero code and a clear message when no backup is found or when integrity verification fails. | Required |

### Output formatting

| ID | Requirement | Status |
|----|-------------|--------|
| FR-040 | Every subcommand must accept `--format=text|json` (default `text`). | Required |
| FR-041 | Text format must produce human-readable output with ANSI colors when stdout is a TTY and no colors otherwise. | Required |
| FR-042 | JSON format must emit structured output suitable for piping (no decorative wrappers). | Required |
| FR-043 | Runtime errors when JSON format is selected must be emitted as JSON to stderr. | Required |

### Configuration

| ID | Requirement | Status |
|----|-------------|--------|
| FR-050 | The system must accept `--db <path>`, `--ollama-host <url>`, and `--embedding-model <name>` flags. | Required |
| FR-051 | The system must honor existing environment variables `SQLITE_DB_PATH`, `OLLAMA_HOST`, `EMBEDDING_MODEL`. | Required |
| FR-052 | Subcommands that do not require the embedder (`search-exact`, `explore`, `status`, all `admin/*`, `help`) must not require Ollama to be reachable. | Required |
| FR-053 | Subcommands that do require the embedder (`store`, `search-fuzzy`) must initialize and validate it before running. | Required |

### Shared bootstrap

| ID | Requirement | Status |
|----|-------------|--------|
| FR-060 | The system must introduce a shared initialization package consumed by all binaries (`hsme`, `hsme-worker`, `hsme-ops`, `hsme-cli`) so DB and embedder setup is not duplicated. | Required |
| FR-061 | The shared bootstrap must support DB-only initialization (without embedder) for callers that don't need it. | Required |
| FR-062 | The shared bootstrap must support full initialization (DB + validated embedder) for callers that do. | Required |
| FR-063 | Existing binaries must behave identically after switching to the shared bootstrap. | Required |

### Build and integration

| ID | Requirement | Status |
|----|-------------|--------|
| FR-070 | The justfile must include `cli-build` and `cli-install` targets that produce the new binary with the project's standard build tags. | Required |
| FR-071 | The default `just install` flow must include the CLI binary alongside `hsme`, `hsme-worker`, `hsme-ops`. | Required |
| FR-072 | The justfile targets `status`, `backup`, `restore`, `retry-failed` must be reduced to thin wrappers around the new CLI subcommands or removed entirely; `scripts/status.sh` is removed once parity is verified. | Required |

### Exit codes

| ID | Requirement | Status |
|----|-------------|--------|
| FR-080 | Successful operations must exit with code `0`. | Required |
| FR-081 | Usage errors (unknown subcommand, missing required flag) must exit with code `1`. | Required |
| FR-082 | Runtime errors (DB failure, embedder validation failure, query failure) must exit with code `2`. | Required |

## Non-Functional Requirements

| ID | Requirement | Threshold | Status |
|----|-------------|-----------|--------|
| NFR-001 | The CLI must not introduce new third-party Go dependencies; only the standard library and packages already used in the repo are allowed. | 0 new external modules in `go.mod` | Required |
| NFR-002 | `hsme-cli status` must complete in well under a second on the current corpus. | p95 wall-clock under 500 ms on a 1k–10k memory corpus | Required |
| NFR-003 | `hsme-cli admin backup` must be safe under concurrent reads/writes from a running MCP server. | Zero corruption observed across 100 backup runs while server is active | Required |
| NFR-004 | The bootstrap refactor must not regress any existing functionality. | Existing test suite (`just test`) passes with zero failures, zero skips added | Required |
| NFR-005 | Compiled CLI binary size must stay reasonable for ops distribution. | Stripped binary ≤ 30 MB on linux/amd64 | Required |
| NFR-006 | New code paths (CLI, bootstrap) must be covered by tests at the level the rest of the repo applies. | Subcommand parsing, output formatting, and error paths exercised by table-driven tests; admin operations covered by integration tests against a temporary DB | Required |

## Constraints

| ID | Constraint | Status |
|----|-----------|--------|
| C-001 | Build tags `sqlite_fts5 sqlite_vec` are required for any binary that opens the DB. | Mandatory |
| C-002 | The CLI installs to `$INSTALL_PATH` (default `~/go/bin`) consistent with existing binaries. | Mandatory |
| C-003 | The MCP wire protocol and existing core function signatures must not change as part of this mission. | Mandatory |
| C-004 | The CLI must coexist with a running MCP server. SQLite WAL mode is the coordination point; no additional locking is introduced. | Mandatory |
| C-005 | The schema/embedder consistency check (`ValidateEmbeddingConfig`) must run at startup whenever the embedder is initialized. | Mandatory |
| C-006 | The four existing binaries (`hsme`, `hsme-worker`, `hsme-ops`, `migrate-legacy`) keep their current names and entry points; only their internal init wiring is touched by the bootstrap refactor. | Mandatory |

## Success Criteria

1. **Operator can run a complete daily ops loop without bash**: `status`, `admin retry-failed`, `admin backup`, `admin restore --latest` all work end-to-end via the CLI.
2. **`scripts/status.sh` is removed** and replaced by `hsme-cli status` with at least the same information available, plus a `--format=json` mode that the bash version does not have.
3. **`justfile` targets for backup/restore/retry-failed are removed or reduced to one-line wrappers** over the CLI; no inline SQL or shell pipelines remain in the justfile for those operations.
4. **Bootstrap is consumed by all four binaries**: `hsme`, `hsme-worker`, `hsme-ops`, `hsme-cli`, with no duplicated init code.
5. **Existing tests pass with zero regressions** and new CLI/bootstrap tests are added.
6. **Restore reliably refuses to operate on a corrupt backup** in 100% of cases where `PRAGMA integrity_check` does not return `ok`.
7. **Engineers can pipe CLI output into shell automation** (`jq`, `xargs`, etc.) for at least the four MCP-tool subcommands and `status`.

## Key Entities

| Entity | Description |
|--------|-------------|
| `hsme-cli` binary | New artifact produced by `cmd/cli/`, installed alongside existing binaries. |
| Shared bootstrap package | New package owning DB-only and DB+embedder initialization. |
| Subcommand handlers | Nine total: `store`, `search-fuzzy`, `search-exact`, `explore`, `status`, `admin retry-failed`, `admin backup`, `admin restore`, `help`. |
| Output formatter | Component that switches between human-readable text (TTY-aware ANSI) and structured JSON. |
| Justfile targets | Updated `cli-build`, `cli-install`, simplified or removed `status`/`backup`/`restore`/`retry-failed`. |

## Assumptions

- The existing `mattn/go-sqlite3` driver (already used in the repo) exposes the SQLite Online Backup API needed for `admin backup`. If it does not, backup falls back to copying the file under a transaction with `BEGIN IMMEDIATE`; this is a plan-phase decision, not a spec change.
- The existing core functions (`indexer.StoreContext`, `search.FuzzySearch`, `search.ExactSearch`, `search.TraceDependencies`, `indexer.CanonicalizeName`) are stable and will not be refactored as part of this mission.
- The existing bash status script's information set is the floor for the new `status` subcommand; extra fields can be added but none can be lost.
- Operators run the CLI on Linux or macOS. Windows support is not in scope for this mission.
- The `ideas/cli-tool.md` file is the design starting point for MCP-tool subcommands and will be deleted once this mission merges (since the spec/plan will supersede it).

## Out of Scope

- Interactive REPL mode for the CLI.
- Reindexing operations (`admin reindex`) — deferred to a future mission.
- Migrating `verify-cutover` (one-off legacy migration tool, no recurring value).
- Migrating `serve`, `work`, `work-bg`, `ops`, `ops-loop`, `migrate` justfile targets (trivial launchers, no value in moving them).
- Changes to MCP protocol, core function signatures, or storage schema.
- Cross-platform Windows support.
