# Contract: Exit Codes

**Mission**: `hsme-unified-cli-01KQ59MV`

| Exit Code | Meaning | Examples |
|-----------|---------|----------|
| `0` | Success | All subcommands on happy path. `status` interrupted by SIGINT during `--watch` also returns 0. |
| `1` | Usage error (operator-actionable, before any DB work) | Unknown subcommand · Missing required flag · Mutually exclusive flags both set (`--from` and `--latest`) · `store` invoked from TTY without redirection · `--watch` requested when stdout is not a TTY |
| `2` | Runtime error (operator-non-actionable without investigation) | DB cannot be opened · Embedder validation fails · Query returns an error · Backup integrity check fails · `--latest` finds no backups · IO error during backup/restore |

## Rules

- **No partial commits** on exit code 2. The `admin restore` subcommand must leave the live DB in its prior state (no WAL/SHM cleanup either) if any pre-rename step fails. Sidecar cleanup happens **only after** integrity check passes and the temp copy is staged.
- **stderr always carries the error message** (text or JSON), even when `--format=json` is set.
- **stdout receives only valid result payloads** (or is silent on error).

## Test surface

Each subcommand has at least one test for each exit code that is reachable from its surface:

- `usage_error_unknown_subcommand` → exit 1
- `usage_error_missing_required_flag` (per subcommand that has required args) → exit 1
- `usage_error_mutually_exclusive_flags` (`admin restore` `--from`/`--latest`) → exit 1
- `runtime_error_db_unreachable` → exit 2
- `runtime_error_backup_integrity_failure` → exit 2 (integration test)
- `success_path` per subcommand → exit 0
