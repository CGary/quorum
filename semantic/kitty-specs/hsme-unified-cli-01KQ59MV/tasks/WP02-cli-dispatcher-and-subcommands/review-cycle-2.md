---
affected_files: []
cycle_number: 2
mission_slug: hsme-unified-cli-01KQ59MV
reproduction_command:
reviewed_at: '2026-04-26T18:08:16Z'
reviewer_agent: unknown
verdict: rejected
wp_id: WP02
---

# Review Feedback: WP02 - CLI Dispatcher and Subcommand Handlers

The implementation of WP02 fails to meet several key requirements and introduces logic errors in flag handling.

## Blocking Issues

### 1. Global Flag Parsing Bug in `main.go`
The `main()` function in `cmd/cli/main.go` registers global flags using `RegisterDBFlags(flag.CommandLine, &cfg)` but fails to call `flag.Parse()`. This causes any flags passed before the subcommand (e.g., `hsme-cli --format json status`) to be treated as an "unknown subcommand". 
- **Remediation**: Call `flag.Parse()` before dispatching, and use `flag.Arg(0)` to identify the subcommand.

### 2. Incomplete Admin Subcommands
The `admin` subcommand in `cmd/cli/admin.go` provides only placeholder `fmt.Println` statements for `backup` and `restore`. These placeholders do not respect the `--format` flag (e.g., they don't return JSON when requested) and do not follow the error handling contract.
- **Remediation**: Implement the handlers for `backup` and `restore` as specified in T017. If core functions from `src/core/admin` are not yet available (planned for WP04), they must be stubbed or the plan adjusted. However, the handlers must at least respect the output formatting logic and error contracts.

### 3. Missing Worker Detection in `status`
The `status` subcommand in `cmd/cli/status.go` does not implement worker detection via `/proc/*/comm` or `pgrep` as required by T016 and the data model. It also lacks the specific `queue` and `graph` structure requested in the prompt.
- **Remediation**: Implement worker detection logic and align the `StatusInfo` struct with the specified data model.

### 4. Direct SQL in `admin retry-failed`
T017 specifically required calling `admin.RetryFailed(ctx, db)` from `src/core/admin/retry.go`. The current implementation inlines the SQL directly.
- **Remediation**: While `src/core/admin/retry.go` is assigned to WP04, if WP02 is tasked with calling it, the function should at least be defined (possibly as a stub in the correct package) to ensure architectural consistency.

## Verdict
**REJECTED**. Please address the flag parsing logic in `main.go` as a priority, and complete the missing requirements for `admin` and `status`.
