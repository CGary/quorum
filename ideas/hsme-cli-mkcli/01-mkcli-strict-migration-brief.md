# hsme-cli Strict mk-cli Migration — Planning Brief

## 0. Metadata

| Field | Value |
|---|---|
| Date | 2026-08-11 |
| Status | planned |
| Future task id | `HSME-006` |
| Decision owner | Gary |
| Decisions summary | STRICT migration of `hsme-cli` to the mk-cli agent-CLI conventions (`/home/gary/Documents/make-clis/mk-cli-EN.md`). Breaking change: the old `--format text\|json` / ad-hoc error surface is REMOVED, not aliased. Every verified caller is updated in the same change. Full scope: all 7 gap areas, all 7 subcommands. Executed through Quorum's SDC lifecycle as task `HSME-006`, decomposed into 3 sequential children (`HSME-006-a/b/c`). |

**This document is the sole input for the agent that runs `/q-brief` (and the
rest of the SDC lifecycle) for `HSME-006`.** It must not need to ask the human
anything. Every flag name, JSON shape, error code, file path, and function
name below is either quoted verbatim from a real source file or is an
explicit, justified decision — never a placeholder to be resolved later.

---

## 1. What This Migration Is

`hsme-cli` (`/home/gary/dev/quorum/semantic/cmd/cli/*.go`, 13 files total
(10 non-test + 3 test), ~1500 lines, build tag `sqlite_fts5 sqlite_vec`, part
of the `github.com/hsme/core`
module under `semantic/`) is HSME's unified CLI. It must be brought into
**strict** compliance with the mk-cli conventions spec
(`/home/gary/Documents/make-clis/mk-cli-EN.md`, 872 lines). "Strict" means:
where the current CLI surface conflicts with mk-cli (e.g. `--format
text|json`), the mk-cli convention wins and the old surface is deleted. This
is a deliberate, human-approved breaking change (2026-08-11), not an
additive/aliased migration.

Because it is breaking, **every verified caller must be updated in the same
change** — see §8 for the exact inventory and edits.

---

## 2. Source Inventory (current state, as read 2026-08-11)

### 2.1 `semantic/cmd/cli/` files

| File | Role |
|---|---|
| `main.go` | Dispatcher: loads `bootstrap.Config`, registers global flags via `RegisterDBFlags`, parses, dispatches on `args[0]` to `runStore`/`runSearchFuzzy`/`runSearchExact`/`runExplore`/`runStatus`/`runAdmin`/`runImportQuorum`/`runHelp`. Defines `exitUsage = 1`, `exitRuntime = 2`. Also implements `ScanTrailingFlags` (lets flags appear after positional args) and `isBoolFlag`. |
| `flags.go` | `outputFormat string = "text"` (package var), `noColorFlag bool`, `RegisterDBFlags(fs, cfg)` registers `--db`, `--ollama-host`, `--embedding-model`, **`--format`** (text\|json, unvalidated string), `--no-color`. Also `GetDBPath`/`GetOllamaHost`/`GetEmbeddingModel`. |
| `output.go` | `IsTTY`, `ShouldColor`, `FormatJSON`, `FormatText`, per-command `Format*Result` human-text formatters (`FormatStoreResult`, `FormatSearchResults`, `FormatExactResults`, `FormatExploreResult`, `FormatAdminBackupResult`, `FormatAdminRestoreResult`, `FormatAdminRetryResult`), **`WriteResult(w, v, format string)`** and **`WriteError(w, err error, code int, format string)`** (ad-hoc `{"error": msg, "code": int}` shape), `Green`/`Red`/`Yellow` ANSI helpers. |
| `help.go` | `printTopLevelHelp()`, `runHelp(args)` — layered per-subcommand help text (already mk-cli-compliant pattern; only doc-strings change in this migration). |
| `store.go` | `runStore` — flags `--source-type` (required), `--project`, `--supersedes`, `--force-reingest`. Rejects interactive TTY stdin (line 48: `if (stat.Mode() & os.ModeCharDevice) != 0`). Calls `indexer.StoreContext`. |
| `search.go` | `runSearchFuzzy`, `runSearchExact` — flags `--limit` (default 10), `--project`; positional query/keyword arg. Calls `search.FuzzySearch` / `search.ExactSearch`. |
| `explore.go` | `runExplore` — flags `--direction` (default `"both"`, **not validated as an enum today**), `--max-depth` (default 5), `--max-nodes` (default 100); positional `entity_name`. Calls `search.TraceDependencies`. |
| `status.go` | `runStatus` — flags `--watch`, `--interval`. Queries `memories`/`kg_nodes`/`kg_edge_evidence`/`async_tasks` tables directly, plus `checkWorkerRunning()` (proc/pgrep sniff for `hsme-worker`). `StatusInfo`/`GraphStats` structs. |
| `admin.go` | `runAdmin` dispatches `retry-failed`/`backup`/`restore` to `runAdminRetryFailed`, `runAdminBackup`, `runAdminRestore`. `restore` validates "exactly one of `--from`/`--latest`" with a **generic error, not enum-coded**. `findLatestBackup()` globs `backups/engram-*.db`. |
| `import_quorum.go` | `runImportQuorum` — flags `--project` (required), `--quorum-project`, `--quorum-db` (default `~/.quorum/memory.db` via `defaultQuorumDBPath()`), `--tasks-root` (default `.ai/tasks`), `--source` (enum `curated\|capsules\|all`, **already validated** at line 56-59, but via a bare `fmt.Errorf`, not an error-code envelope). Calls `quorumdelta.Import` and/or `capsule.Import`. |

### 2.2 Already mk-cli-compliant (do not regress)

- Atomic subcommands via `main.go`'s `switch subcommand` dispatcher.
- `--format text|json` global flag exists (but the *values* are wrong per
  mk-cli — see §3; this flag itself is being removed and replaced, not kept).
- stdout = data, stderr = human-readable errors/help (`WriteError` today
  writes to whatever `io.Writer` is passed, which is always `os.Stderr` at
  call sites regardless of `--format`). Post-migration the split is made
  explicit and total, defined once in §5.1 — do not re-derive it here or in
  §6.2: under `--json`, both success **and error** envelopes go to
  **stdout** (mirroring `cmd/fleet_agentio.go`; a deliberate behavior change
  for the JSON case, since today's JSON errors go to stderr); without
  `--json` (human mode), errors print a concise human-readable line to
  **stderr** and nothing is written to stdout. This bullet and §6.2 must be
  read as consistent with §5.1, not as independent rules.
- Stable exit codes: `exitUsage = 1`, `exitRuntime = 2` (`main.go`).
- `store` rejects interactive TTY without piped stdin (`store.go:48`).
- Closed enum already enforced for `import-quorum --source`
  (`import_quorum.go:56-59`), just needs its error re-coded.
- Layered help (`help.go`: `printTopLevelHelp`, `runHelp`).

### 2.3 Test files pinning the *current* surface (must be rewritten, not deleted)

| File | Lines | What it pins today |
|---|---|---|
| `semantic/tests/modules/cli_test.go` | 98 | Builds the real binary, runs `store`/`status`/`search-exact`/`search-fuzzy`/`help` against it. Line 79 uses `runCLI("--format", "json", "search-fuzzy", "test")` and asserts on the old bare-map JSON shape (`"results"` key at top level). |
| `semantic/tests/modules/cli_import_quorum_test.go` | 204 | Lines 56, 85, 106, 125, 179 all invoke `runCLI`/`exec.Command` with `--format json import-quorum ...` and assert on `FormatImportQuorumResult`'s bare-map JSON. |
| `semantic/cmd/cli/flags_test.go` | 54 | `TestRegisterDBFlags` parses `-format json -no-color` and asserts on the package var `outputFormat`. |
| `semantic/cmd/cli/output_test.go` | 119 | `TestFormatJSON`, `TestWriteResult` (asserts `WriteResult(&buf, v, "json"/"text")`), `TestWriteError` (asserts the old `{"error":"...", "code": 2}` shape), `TestColorFunctions`. |
| `semantic/cmd/cli/help_test.go` | 76 | `TestPrintTopLevelHelp`, `TestRunHelp` — asserts on doc-string substrings (`"Usage: hsme-cli <subcommand>"`, `"hsme-cli store"`, etc.). Structurally unaffected but doc-strings inside `help.go` change, so expected substrings may need touch-ups. |

---

## 3. mk-cli Spec Citations Used

All section titles below are verbatim headings from
`/home/gary/Documents/make-clis/mk-cli-EN.md`.

- **"Recommended JSON Contract" → "Minimal Successful Output"**: `{ok, command,
  summary, data, next_actions}`.
- **"Recommended JSON Contract" → "Error Output"**: `{ok:false, command,
  error:{code, message, field, received}, retryable, suggested_fix:{command}}`.
- **"JSON Rules"**, item 6: `next_actions` must be small, **maximum 3
  suggestions**.
- **"Argument Design" → "Recommended Universal Arguments"**: the full list is
  `--json --plain --quiet --verbose --debug --cwd --config --output --limit
  --cursor --timeout --dry-run --yes --no-input --schema --version`.
- **"`--schema` Mode"**: every command answers `--schema` with a JSON contract
  `{command, description, input:{required, properties}, output:{type,
  required}}`, without executing.
- **"Output Size Control to Save Tokens"**: commands returning significant
  content should support `--limit --fields --summary --output --cursor`; rule
  9 of "JSON Rules" — "Do not return huge lists by default"; rule 10 — "For
  large results, return `result_file`, `cursor`, or `next_page_token`."
- **"Error Handling for Agents" → recommended codes**: `INVALID_ARGUMENT,
  MISSING_REQUIRED_FLAG, INVALID_ENUM, FILE_NOT_FOUND, CONFIG_NOT_FOUND,
  VALIDATION_FAILED, PERMISSION_DENIED, CONFLICT, TIMEOUT, NETWORK_ERROR,
  PARTIAL_SUCCESS, INTERNAL_ERROR`.
- **"Documentation for Agents" → "AGENTS.md"**: template block ("Default agent
  flags: Always pass `--json`. Always pass `--no-input`...").
- **"Documentation for Agents" → "Skill"**: progressive-disclosure skill doc
  — name/description with trigger words, then "When to use / when not to use
  / required flags / 5-10 common commands / JSON contract / error rules,"
  explicitly **not** a paste of full CLI help.
- **"Separate Read, Plan, and Write Operations"**: justifies `--dry-run` on
  `store`, `admin restore`, `import-quorum`.
- **"Design Principles" #3 "Prefer Enums Over Free Text"**: justifies coding
  `explore --direction` as a validated enum (today it is an unvalidated
  string default, §2.1).

---

## 4. Reference Implementation to Mirror

`quorum fleet run` (root module, **not** `semantic/`) already implements this
exact mk-cli contract in Go, in this repo, today:

- `/home/gary/dev/quorum/cmd/fleet_run.go` — the command itself (`runFleetRun`,
  `fleetRunSchema`, the `--dry-run` short-circuit at line 168-179, the
  `--schema` short-circuit at line 63-66 which runs **before** any required-flag
  validation).
- `/home/gary/dev/quorum/cmd/fleet_agentio.go` — the shared envelope/error
  plumbing:
  - Error code constants (lines 16-23): `errCodeInvalidArgument =
    "INVALID_ARGUMENT"`, `errCodeMissingRequired = "MISSING_REQUIRED_FLAG"`,
    `errCodeInvalidEnum = "INVALID_ENUM"`, `errCodeFileNotFound =
    "FILE_NOT_FOUND"`, `errCodeTimeout = "TIMEOUT"`, `errCodeInternal =
    "INTERNAL_ERROR"`.
  - `fleetSuccessEnvelope` struct (lines 33-39): `OK bool`, `Command string`,
    `Summary string`, `Data any`, `NextActions []fleetNextAction`.
  - `fleetErrorBody` (41-47): `Code, Message, Field, Received string`
    (`Field`/`Received` are `omitempty`).
  - `fleetErrorEnvelope` (56-62): `OK bool`, `Command string`, `Error
    fleetErrorBody`, `Retryable bool`, `SuggestedFix *fleetSuggestedFix`
    (`omitempty`).
  - `fleetAgentError(command, code, message, field, received string,
    retryable bool, fix string) fleetErrorEnvelope` (66-76) — the single
    constructor every error site calls.
  - `fleetEmit{JSON, Plain, Quiet bool}` with `.success()`, `.failure()`,
    `.schema()` methods (81-115) — under `JSON`, exactly one compact JSON
    object goes to **stdout** (not stderr — line 88 `success(stdout, _
    io.Writer, ...)`, line 102 `failure(stdout, _ io.Writer, ...)`: the
    second writer parameter is explicitly unused, i.e. errors are written to
    **stdout** so an agent parsing stdout always gets a structured answer,
    per the comment at line 100-101). **This is a deliberate deviation from
    the historical hsme-cli convention** (which wrote `WriteError` to
    stderr) and from mk-cli's abstract stdout/stderr split — mirror the
    reference's actual behavior: JSON error envelopes go to stdout.
  - `writeCompactJSON` (118-125).
- `fleetRunSchema(transport)` (`fleet_run.go:223-241`) — the concrete
  `--schema` output shape to mirror, including an **`errors: []string`**
  field beyond the mk-cli spec's minimal example (`fleet_run.go:239`) — this
  is a useful, non-conflicting enhancement; replicate it.

**Retryable convention actually used by the reference** (read directly off
the `fleetAgentError(...)` call sites in `fleet_run.go`, not off mk-cli's own
illustrative example, which is inconsistent with this repo's practice —
`retryable: true` in the mk-cli guide's `INVALID_ENUM` example, `false` at
`fleet_run.go:97`):

| Error code | `retryable` in `fleet_run.go` | Call site |
|---|---|---|
| `MISSING_REQUIRED_FLAG` | `true` | lines 75, 80, 84 |
| `INVALID_ARGUMENT` | `false` | lines 68, 154, 163 |
| `FILE_NOT_FOUND` | `false` | lines 90, 106 |
| `INVALID_ENUM` | `false` | line 97 |
| `TIMEOUT` | `true` | line 192 |
| `INTERNAL_ERROR` | `false` | lines 139, 206 |

Use this exact table for the codes it covers. For codes not present in
`fleet_run.go` but needed by `hsme-cli` (see §6.2), this brief assigns:
`CONFLICT: true`, `VALIDATION_FAILED: true`, `NETWORK_ERROR: true`,
`PERMISSION_DENIED: false`. Rationale: `retryable: true` means "the same
command, with a trivial fix, is expected to succeed" — flag-shaped and
transient failures are `true`; structural/permission failures are `false`.

**Hard constraint (ADR 0008): the two Go modules must never import each
other.** `semantic/` (module `github.com/hsme/core`) must never import
anything from the root `quorum` module, and vice versa. Mirror the *pattern*
from `cmd/fleet_run.go` / `cmd/fleet_agentio.go` by writing new, analogous Go
code inside `semantic/cmd/cli/` (e.g. a new `semantic/cmd/cli/envelope.go`)
— never by importing `quorum/cmd` or `quorum/internal/core` packages. The
CI acid test (root builds with `CGO_ENABLED=0`, no C compiler, `semantic/`
absent) must keep passing untouched.

**Structural difference to account for**: `cmd/fleet_run.go` is a Cobra
command (`github.com/spf13/cobra`); `semantic/cmd/cli/*.go` uses the stdlib
`flag` package with the `RegisterDBFlags(fs, cfg)` / `ScanTrailingFlags(fs)`
pattern (`main.go`, `flags.go`). Port only the **envelope structs, error
code constants, `fleetAgentError`-equivalent constructor, and the
control-flow ordering** (schema-check before required-flag validation,
dry-run short-circuit before the real write) into idiomatic stdlib-`flag`
Go — do not introduce Cobra into `semantic/`.

---

## 5. Target Architecture

### 5.1 New file: `semantic/cmd/cli/envelope.go`

Create this file (mirrors `cmd/fleet_agentio.go`, adapted to stdlib `flag`):

```go
//go:build sqlite_fts5 && sqlite_vec

package main

const (
	errInvalidArgument   = "INVALID_ARGUMENT"
	errMissingRequired   = "MISSING_REQUIRED_FLAG"
	errInvalidEnum       = "INVALID_ENUM"
	errFileNotFound      = "FILE_NOT_FOUND"
	errValidationFailed  = "VALIDATION_FAILED"
	errPermissionDenied  = "PERMISSION_DENIED"
	errConflict          = "CONFLICT"
	errTimeout           = "TIMEOUT"
	errNetworkError      = "NETWORK_ERROR"
	errInternal          = "INTERNAL_ERROR"
)

type NextAction struct {
	Command string `json:"command"`
	Reason  string `json:"reason"`
}

type SuccessEnvelope struct {
	OK          bool         `json:"ok"`
	Command     string       `json:"command"`
	Summary     string       `json:"summary"`
	Data        interface{}  `json:"data"`
	NextActions []NextAction `json:"next_actions"`
}

type ErrorBody struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Field    string `json:"field,omitempty"`
	Received string `json:"received,omitempty"`
}

type SuggestedFix struct {
	Command string `json:"command"`
}

type ErrorEnvelope struct {
	OK           bool          `json:"ok"`
	Command      string        `json:"command"`
	Error        ErrorBody     `json:"error"`
	Retryable    bool          `json:"retryable"`
	SuggestedFix *SuggestedFix `json:"suggested_fix,omitempty"`
}
```

Plus a constructor `NewErrorEnvelope(command, code, message, field, received
string, retryable bool, fix string) ErrorEnvelope` (identical shape to
`fleetAgentError`), and three writers:

- `WriteSuccessEnvelope(w io.Writer, env SuccessEnvelope)` — marshals to a
  single compact JSON line (`json.Marshal`, not `MarshalIndent` — the
  reference uses compact for `--json`; keep `MarshalIndent` only for
  `--schema`, matching `fleetEmit.schema`'s `enc.SetIndent("", "  ")`).
- `WriteErrorEnvelope(w io.Writer, env ErrorEnvelope)` — same, compact JSON.
  Called with `w = os.Stdout` under `--json` (per §4's stdout convention for
  errors too), never `os.Stderr`. This is the **only** JSON error path.
- `WriteHumanError(w io.Writer, env ErrorEnvelope)` — the human-mode (no
  `--json`) counterpart, replacing `WriteError`'s old text branch
  (`output.go:180`, `fmt.Fprintf(w, "error: %v\n", err)`). Prints one
  concise line to `w = os.Stderr`: `error: <env.Error.Message>` and, when
  `env.SuggestedFix != nil`, a second line `try: <env.SuggestedFix.Command>`.

Every call site builds the same `ErrorEnvelope` once via `NewErrorEnvelope`
and then branches only on the writer:
`if agentFlags.JSON { WriteErrorEnvelope(os.Stdout, env) } else { WriteHumanError(os.Stderr, env) }`,
then `os.Exit(<code>)` using the exit-code rule defined in §6.2 (usage/validation
codes → `exitUsage`, runtime/IO/network codes → `exitRuntime`).

This is the complete, exhaustive definition of both error paths. Human mode
was previously undefined by this migration beyond "keep the discipline"
(§2.2); it is now fully specified: **stderr, one concise line, no JSON.**
There is no contradiction with §2.2: `--json` errors go to stdout (the one
documented exception), everything else — including every human-mode error —
stays on stderr.

`NextActions` must always be a non-nil `[]NextAction{}` (empty slice, not
`nil`) so it serializes as `[]`, never `null` — mk-cli's JSON rules require
the key to exist; an empty list is the common case for terminal
success/failure results here.

### 5.2 `flags.go` rewrite

Remove `outputFormat string = "text"` and the `--format` registration.
Remove nothing else from `RegisterDBFlags` (`--db`, `--ollama-host`,
`--embedding-model`, `--no-color` are untouched — `--no-color` is **not**
part of the mk-cli gap list and is retained as-is, only for the human-text
output path).

Add a second registration function, called alongside `RegisterDBFlags` at
every existing call site (`main.go`'s `flag.CommandLine` registration, and
each `runXxx`'s per-subcommand `fs`):

```go
func RegisterAgentFlags(fs *flag.FlagSet, a *AgentFlags)
```

with a struct:

```go
type AgentFlags struct {
	JSON     bool
	NoInput  bool
	Quiet    bool
	Verbose  bool
	Output   string
	Timeout  int
	Schema   bool
}
```

registering `--json` (bool, replaces `--format json`; absence = human text
mode, matching the removed `--format text` default), `--no-input` (bool),
`--quiet` (bool), `--verbose` (bool), `--output <path>` (string), `--timeout
<seconds>` (int, `0` = no bound / transport-appropriate default), `--schema`
(bool). Additionally add a package-level `--version` flag registered **only**
on `flag.CommandLine` in `main.go` (not per-subcommand — it is a top-level
concern), handled before subcommand dispatch:

```go
const cliVersion = "2.0.0" // SemVer major bump: breaking mk-cli migration
```

`hsme-cli --version` (no `--json`) prints `hsme-cli 2.0.0` to stdout, exit 0.
`hsme-cli --version --json` prints `{"ok":true,"command":"version","summary":"hsme-cli 2.0.0","data":{"version":"2.0.0"},"next_actions":[]}`, exit 0.

**Explicitly out of scope, do not add** (see §9 for the full out-of-scope
list): `--plain`, `--debug`, `--cwd`, `--config`, `--yes`, `--cursor`. `--plain`
is redundant with the existing default text mode; `--debug` has no stack-trace
use case here yet; `--cwd`/`--config` don't apply (there is no repo-relative
working directory concept in `hsme-cli`, `--db` already plays the "explicit
config" role); `--yes` has no destructive-confirmation prompt to gate (no
subcommand ever blocks on a y/n prompt today, and none should gain one —
`--no-input` already covers "never prompt"); `--cursor` — investigated and
rejected for this migration, see §6.6.

### 5.3 `--json`/`--schema`/`--dry-run` control-flow ordering (every subcommand)

Mirror `runFleetRun`'s ordering exactly, adapted per subcommand:

1. Register + parse flags (`RegisterDBFlags`, `RegisterAgentFlags`, plus the
   subcommand's own flags), `ScanTrailingFlags`.
2. **`--schema` check first** — print the schema, exit 0. No DB open, no
   network, no stdin read.
3. Required-flag validation (`MISSING_REQUIRED_FLAG` / positional-arg
   equivalent).
4. Enum validation (`INVALID_ENUM`).
5. **`--dry-run` check** (only on `store`, `admin restore`, `import-quorum`)
   — after validation, before any DB write / file write, return
   `data.dry_run: true` describing what *would* happen, exit 0.
6. Perform the real operation; map errors per §6.2's code table.
7. Emit `SuccessEnvelope` (or the existing human-text `Format*Result`
   path when `--json` is absent — **the human-text formatters in
   `output.go` are kept unchanged**; only the `--json` branch's payload
   shape changes from a bare `map[string]interface{}` to `SuccessEnvelope`).

### 5.4 `command` field values (10 distinct envelope values across 7 subcommands, mirrors `fleetRunCommand = "fleet.run"`)

| Subcommand | `command` field |
|---|---|
| `store` | `"store"` |
| `search-fuzzy` | `"search-fuzzy"` |
| `search-exact` | `"search-exact"` |
| `explore` | `"explore"` |
| `status` | `"status"` |
| `admin retry-failed` | `"admin.retry-failed"` |
| `admin backup` | `"admin.backup"` |
| `admin restore` | `"admin.restore"` |
| `import-quorum` | `"import-quorum"` |
| `--version` | `"version"` |

**Canonical count (use this number everywhere else in this document,
especially the ACs in §12)**: `hsme-cli` has **7 subcommands** (`store`,
`search-fuzzy`, `search-exact`, `explore`, `status`, `admin`,
`import-quorum`). `admin` alone dispatches to 3 sub-actions, so the
envelope's `command` field takes **9 distinct values across those 7
subcommands** (`admin` contributes 3 — `admin.retry-failed`/
`admin.backup`/`admin.restore` — not 1). Adding the top-level `--version`
flag (not a subcommand) gives **10 distinct `command` envelope values
overall**, matching the 10 rows in the table above. §7's flag matrix has 10
rows for the same reason (9 subcommand rows + 1 `(top level)` row for
global-only flags).

---

## 6. Gap-by-Gap Migration Plan

### 6.1 Gap 1 — Standard JSON response envelope

**mk-cli requires**: `{ok, command, summary, data, next_actions}` on success
("Minimal Successful Output").
**Today**: `WriteResult(w, v, format)` in `output.go:153-168` writes either
`FormatJSON(v)` (a bare `map[string]interface{}` per command, e.g.
`store.go:82-90`'s `res := map[string]interface{}{"memory_id": id, "status":
"stored"}`) or the human text formatter.
**Target**: under `--json`, every `runXxx` builds a `SuccessEnvelope{OK:
true, Command: "<table in §5.4>", Summary: "<one sentence>", Data:
<today's existing map/struct, unchanged>, NextActions: []NextAction{...}}`
and calls `WriteSuccessEnvelope(os.Stdout, env)`. **The existing `Data`
payloads are reused as-is** (e.g. `store`'s `{"memory_id": id, "status":
"stored"}` becomes the `data` field verbatim) — this gap is about wrapping,
not redesigning, the existing result shapes.
**Files**: `envelope.go` (new), `store.go`, `search.go` (both functions),
`explore.go`, `status.go`, `admin.go` (all three `runAdminXxx`),
`import_quorum.go`.

### 6.2 Gap 2 — Actionable error schema + stable codes

**mk-cli requires**: `{ok:false, command, error:{code, message, field,
received}, retryable, suggested_fix:{command}}`.
**Today**: `WriteError(w, err error, code int, format string)` in
`output.go:170-181` writes `{"error": err.Error(), "code": <int exit
code>}` — the `code` field is the **process exit code** (1 or 2), not a
semantic error code.
**Target**: replace every failure branch — both `WriteError(os.Stderr, err,
exitUsage|exitRuntime, outputFormat); os.Exit(...)` call sites (some already
pass `exitUsage`, e.g. `admin.go:111`, `import_quorum.go:44,57` — not only
`exitRuntime`) and the non-`WriteError` branches that print directly and
`os.Exit` without ever calling `WriteError` (`fmt.Fprintln(os.Stderr, ...)` +
`fs.Usage()` at `store.go:40-44`; `fmt.Println(...)` — to **stdout**, not
stderr — at `store.go:47-51`/`:59-62`; `fmt.Fprintln(os.Stderr, ...)` at
`search.go:26-29`/`:66-69` and `explore.go:28-31`; `fmt.Fprintln`/
`Fprintf(os.Stderr, ...)` at `admin.go:19-22`/`:34-37`) — with an explicit
`NewErrorEnvelope(...)` + the writer selected per §5.1 (stdout under
`--json` via `WriteErrorEnvelope`, stderr otherwise via `WriteHumanError`) +
`os.Exit(<code>)`.

`exitUsage=1`/`exitRuntime=2` are **unchanged in value** — mk-cli doesn't
mandate specific exit code values, only that they're stable ("Recommended
contract": "exit code `0`: success. non-zero exit code: error.",
`mk-cli-EN.md:217-218` — no specific non-zero values required). What changes
is that the mapping from error **code** to exit **code** becomes one
explicit, uniform rule instead of whichever exit constant a given call site
happened to already hardcode:

- **`exitUsage` (1)** — usage/validation-shaped codes: `MISSING_REQUIRED_FLAG`,
  `INVALID_ENUM`, `INVALID_ARGUMENT`, `VALIDATION_FAILED`.
- **`exitRuntime` (2)** — runtime/IO/network-shaped codes: `FILE_NOT_FOUND`,
  `CONFLICT`, `TIMEOUT`, `NETWORK_ERROR`, `PERMISSION_DENIED`,
  `INTERNAL_ERROR`.

This rule is consistent with every current call site's exit code (verified
2026-08-11 against the table below) — e.g. `store.go:40-44`'s
`MISSING_REQUIRED_FLAG` already exits `exitUsage`, `store.go:64-68`'s
`INTERNAL_ERROR` already exits `exitRuntime` — so **no call site's exit code
value actually changes**; only the JSON body shape and destination stream
change (§5.1).

Per-site error code mapping (exhaustive — every existing failure branch in
`semantic/cmd/cli/*.go` mapped to one code; `*` marks a site that does not
call `WriteError` today, `†` marks a site that calls `WriteError` today but
with `exitUsage`, not `exitRuntime` — see the rule above for how both
collapse onto the same `exitUsage`/`exitRuntime` split):

| Site | Condition | Code | Field | Retryable | Exit |
|---|---|---|---|---|---|
| `store.go:40-44` * | `--source-type` empty | `MISSING_REQUIRED_FLAG` | `source-type` | `true` | `exitUsage` |
| `store.go:47-51` * | stdin is a TTY (no piped input) | `VALIDATION_FAILED` | `stdin` | `true` | `exitUsage` |
| `store.go:59-62` * | stdin content is empty | `VALIDATION_FAILED` | `stdin` | `true` | `exitUsage` |
| `store.go:64-68` | `bootstrap.OpenWithEmbedder` fails | `INTERNAL_ERROR` | — | `false` | `exitRuntime` |
| `store.go:76-80` | `indexer.StoreContext` returns an error whose message has the prefix `DUPLICATE_CONTENT:` (decided sentinel: `strings.HasPrefix(err.Error(), "DUPLICATE_CONTENT:")`, exact string verified at `semantic/src/core/indexer/ingest.go:43`). This only happens when `--force-reingest` **was** set but `--supersedes` is absent or does not match the existing memory's ID — when `--force-reingest` is unset and the hash matches, `StoreContext` silently returns the existing ID with **no error** (a success, not a failure branch, so it needs no row here) | `CONFLICT` | `supersedes` | `true`; `suggested_fix` is `hsme-cli store ... --supersedes <existing_memory_id>` | `exitRuntime` |
| `store.go:76-80` | any other `indexer.StoreContext` error (message prefix does not match `DUPLICATE_CONTENT:`) | `INTERNAL_ERROR` | — | `false` | `exitRuntime` |
| `search.go:26-29` (`runSearchFuzzy`) * / `:66-69` (`runSearchExact`) * | missing positional query/keyword | `MISSING_REQUIRED_FLAG` | `query` | `true` | `exitUsage` |
| `search.go:32-36` | `bootstrap.OpenWithEmbedder`/`OpenDB` fails | `INTERNAL_ERROR` | — | `false` | `exitRuntime` |
| `search.go:39-43` (`FuzzySearch` only) | `search.FuzzySearch` returns a non-nil error — verified (`semantic/src/core/search/fuzzy.go:174-201`) this can **only** come from its internal `LexicalSearch` call or the batched chunk-lookup SQL query; the Ollama embedder call (`GenerateVector`, `fuzzy.go:183`) has its error swallowed internally (logged to stderr, `fuzzy.go:192`) and the search degrades to lexical-only, so no Ollama/network error is ever propagated to the CLI through this path | `INTERNAL_ERROR` | — | `false` | `exitRuntime` |
| `explore.go:28-31` * | missing positional `entity_name` | `MISSING_REQUIRED_FLAG` | `entity-name` | `true` | `exitUsage` |
| `explore.go` (new check, added by this migration — see §6.3) | `--direction` not one of `upstream\|downstream\|both` | `INVALID_ENUM` | `direction` | `false` | `exitUsage` |
| `explore.go:34-38` | `bootstrap.OpenDB` fails | `INTERNAL_ERROR` | — | `false` | `exitRuntime` |
| `status.go:30-34` | `bootstrap.OpenDB` fails | `INTERNAL_ERROR` | — | `false` | `exitRuntime` |
| `status.go:38-42` | `getStatus` query error | `INTERNAL_ERROR` | — | `false` | `exitRuntime` |
| `admin.go:19-22` (`runAdmin`) * | missing admin action | `MISSING_REQUIRED_FLAG` | `action` | `true` | `exitUsage` |
| `admin.go:34-37` (`runAdmin`) * | unknown admin action | `INVALID_ENUM` | `action` | `false` | `exitUsage` |
| `admin.go:45-49`/`52-56` (`runAdminRetryFailed`) | DB open / `RetryFailedTasks` fails | `INTERNAL_ERROR` | — | `false` | `exitRuntime` |
| `admin.go:78-82` (`runAdminBackup`) | `os.MkdirAll("backups", ...)` fails (check `os.IsPermission(err)`) | `PERMISSION_DENIED` if permission error, else `INTERNAL_ERROR` | `dest` | `false` | `exitRuntime` |
| `admin.go:83-87` (`runAdminBackup`) | `admin.Backup` fails | `INTERNAL_ERROR` | — | `false` | `exitRuntime` |
| `admin.go:110-113` (`runAdminRestore`) † | not exactly one of `--from`/`--latest` | `VALIDATION_FAILED` | `from`/`latest` | `true` | `exitUsage` |
| `admin.go:116-123` (`runAdminRestore`) | `--latest` with no backups found | `FILE_NOT_FOUND` | `latest` | `false` | `exitRuntime` |
| `admin.go` (new check, added by this migration) | `--from <path>` does not exist (`os.Stat` before calling `admin.Restore`) | `FILE_NOT_FOUND` | `from` | `false` | `exitRuntime` |
| `admin.go:125-129` (`runAdminRestore`) | `admin.Restore` fails for any other reason | `INTERNAL_ERROR` | — | `false` | `exitRuntime` |
| `import_quorum.go:43-46` † | `--project` empty | `MISSING_REQUIRED_FLAG` | `project` | `true` | `exitUsage` |
| `import_quorum.go:56-59` † | `--source` not `curated\|capsules\|all` | `INVALID_ENUM` | `source` | `false` | `exitUsage` |
| `import_quorum.go:61-65` | HSME `bootstrap.OpenDB` fails | `INTERNAL_ERROR` | — | `false` | `exitRuntime` |
| `import_quorum.go:71-75` | `--quorum-db` path does not exist (`os.Stat` before `quorumdelta.OpenReadOnly`) | `FILE_NOT_FOUND` | `quorum-db` | `false` | `exitRuntime` |
| `import_quorum.go:71-75` | `quorumdelta.OpenReadOnly`/`Import` fails for any other reason | `INTERNAL_ERROR` | — | `false` | `exitRuntime` |
| `import_quorum.go:86-92` | `capsule.Import` fails | `INTERNAL_ERROR` | — | `false` | `exitRuntime` |

**`NETWORK_ERROR` classification rule** (for any future HTTP/Ollama/OpenRouter
client-layer call that does propagate an error to the CLI — none does
today, per the `search.go:39-43` row above): an error is
`NETWORK_ERROR`/`retryable:true` only if it originates from an HTTP
round-trip failure inside `semantic/src/core/inference/{ollama,openrouter}/*.go`
(dial/connection-refused/timeout/non-2xx from `http.Client.Do`, e.g.
`ollama/embedder.go:48-58`). Every other error — database (`database/sql`),
filesystem (`os`), or programming errors — is `INTERNAL_ERROR`/
`retryable:false`. No current `cmd/cli/*.go` call site meets the
`NETWORK_ERROR` condition; this rule exists so any future subcommand that
calls the embedder/extractor directly (bypassing `FuzzySearch`'s internal
fallback) classifies correctly without re-litigating the decision.

**Do not introduce a new failure mode for per-item `Errored` counts inside
`quorumdelta.ImportResult`/`capsule.ImportResult`** — `import-quorum` already
tolerates individual item failures by design (ADR 0013 §4, cited in
`help.go:129`); keep `ok: true` with `Errored > 0` surfaced inside `data`
exactly as today, unless the *whole* import call returns a Go `error` (the
cases in the table above).

**Files**: `envelope.go` (new), every failure branch (both `WriteError` call
sites and the non-`WriteError` raw-print branches marked `*`/`†` above)
across `store.go`, `search.go`, `explore.go`, `status.go`, `admin.go`,
`import_quorum.go`, `main.go` (unknown/missing subcommand — see §6.3.1).

### 6.3 Gap 3 — Universal agent flags

Already fully specified in §5.2. Summary of the delta:

- **Removed**: `--format text|json`, the `outputFormat` package var.
- **Added** (registered for all 7 subcommands via `RegisterAgentFlags`):
  `--json`, `--no-input`, `--quiet`, `--verbose`, `--output <path>`,
  `--timeout <seconds>`, `--schema`.
- **Added** (top-level only): `--version`.
- **Unchanged**: `--db`, `--ollama-host`, `--embedding-model`, `--no-color`.

#### 6.3.1 `main.go` top-level error handling

Two paths in `main.go` currently write raw text to stderr and exit, bypassing
`WriteError` entirely:

- No subcommand given (`main.go:36-42`): if the top-level `--json` flag was
  set, emit `ErrorEnvelope{Command: "cli", Error: {Code:
  "MISSING_REQUIRED_FLAG", Message: "missing subcommand", Field:
  "subcommand"}, Retryable: true, SuggestedFix: {"hsme-cli --help"}}` to
  stdout, exit `exitUsage`. Without `--json`, keep the existing
  `printTopLevelHelp()` + `exitUsage` behavior unchanged (this is the
  human/no-flags path, not an agent-facing regression).
- Unknown subcommand (`main.go:69-72`): same pattern with `Code:
  "INVALID_ARGUMENT"`, `Field: "subcommand"`, `Received: subcommand`,
  `Retryable: false`.

#### 6.3.2 `--no-input` semantics

`store.go:47-51` already refuses to read from an interactive TTY
automatically (mk-cli: "The CLI should not require interactive prompts. If
`stdin` is not interactive, the program should skip prompts"). `--no-input`
does not need to change this detection logic — it is accepted and parsed on
every subcommand for **contract symmetry with `quorum fleet run`** (so
callers can pass it uniformly per the AGENTS.md template in §6.7) but has no
additional runtime effect beyond what TTY-sniffing already does, since no
subcommand in `hsme-cli` has an interactive prompt today or gains one in
this migration. Do not wire it to anything beyond accepting and ignoring it
(explicitly not a dead-code smell — it exists so `hsme-cli ... --no-input`
never errors with "unknown flag" when a caller follows the universal-flag
convention).

### 6.4 Gap 4 — `--schema` introspection

Every subcommand gets a `--schema` bool flag (via `RegisterAgentFlags`),
checked **first**, before required-flag validation (§5.3 step 2), printing
(via `WriteSchemaEnvelope`, analogous to `fleetEmit.schema` —
`json.NewEncoder(stdout).SetIndent("", "  ")`) a JSON object shaped exactly
like `fleetRunSchema` (`fleet_run.go:223-241`):

```json
{
  "command": "search-fuzzy",
  "description": "Perform a semantic search using embeddings.",
  "input": {
    "required": ["query"],
    "properties": {
      "query": {"type": "string", "description": "positional search text"},
      "limit": {"type": "integer", "default": 10},
      "project": {"type": "string", "description": "filter results by project"},
      "fields": {"type": "string", "description": "comma-separated subset of result fields"},
      "output": {"type": "string", "description": "write full result to this file; returns data.result_file"}
    }
  },
  "output": {"type": "object", "required": ["ok", "command", "summary", "data"]},
  "errors": ["MISSING_REQUIRED_FLAG", "INTERNAL_ERROR", "NETWORK_ERROR"]
}
```

Build one such schema function per subcommand (`storeSchema()`,
`searchFuzzySchema()`, `searchExactSchema()`, `exploreSchema()`,
`statusSchema()`, `adminRetryFailedSchema()`, `adminBackupSchema()`,
`adminRestoreSchema()`, `importQuorumSchema()`), each returning
`map[string]any`, listing only the codes that subcommand can actually emit
(per the §6.2 table). No DB open, no network call, no stdin read may occur
before `--schema` is handled — this must be checked immediately after flag
parsing.

### 6.5 Gap 5 — Read/plan/write separation (`--dry-run`)

Per the human decision, exactly three subcommands get `--dry-run`: `store`,
`admin restore`, `import-quorum`. **`admin backup` does NOT get `--dry-run`
in this migration** even though it also writes a file — it is out of the
decided scope; do not add it (avoid scope creep beyond the human decision).

- `store --dry-run`: after reading + validating stdin (so the dry-run report
  can include real content length / detected `--source-type`), skip the
  `bootstrap.OpenWithEmbedder` + `indexer.StoreContext` call entirely. Return
  `data: {"dry_run": true, "source_type": sourceType, "project": project,
  "content_bytes": len(contentBytes), "would_supersede":
  hasSupersedes}`.
- `admin restore --dry-run`: after resolving `src` (via `--from` or
  `--latest`/`findLatestBackup()`) and confirming it exists, skip
  `admin.Restore(...)`. Return `data: {"dry_run": true, "restore_from":
  src}`.
- `import-quorum --dry-run`: after validating `--project`/`--source` and
  opening the HSME DB (read access only, to prove connectivity) but before
  calling `quorumdelta.Import`/`capsule.Import`, skip both. Return `data:
  {"dry_run": true, "source": source, "quorum_db": quorumDB, "project":
  project}`.

### 6.6 Gap 6 — Pagination / output controls (only what mk-cli actually mandates)

mk-cli's "Output Size Control to Save Tokens" section lists `--limit
--fields --summary --output --cursor` for "every command that may return
significant content." Applying this literally, not inventing beyond it:

- **`--limit`**: already present on `search-fuzzy`/`search-exact` (default
  10). `explore` already bounds size via `--max-depth`/`--max-nodes` (not
  `--limit` — leave as-is, these are the domain-appropriate equivalent).
  `store`, `status`, `admin *`, `import-quorum` don't return lists — no
  `--limit` needed.
- **`--fields <a,b,c>`**: add to `search-fuzzy`, `search-exact`, `explore`
  only — the three commands whose `data` contains a homogeneous array
  (`results`/`nodes`+`edges`). Comma-separated field-name projection applied
  to each result item before marshaling (drop keys not listed; if `--fields`
  is absent, return the full item as today). `store`, `status`, `admin *`,
  `import-quorum` return small fixed-shape objects, not lists — no
  `--fields`.
- **`--summary`**: this requirement is already satisfied structurally by the
  envelope's mandatory `summary` string field (§6.1) — no separate flag is
  needed or should be added.
- **`--output <path>`**: registered globally (§5.2) for all 7 subcommands
  for contract symmetry; meaningfully used on `search-fuzzy`, `search-exact`,
  `explore`, `import-quorum` (writes the full `data` payload as JSON to the
  file, and the stdout envelope's `data` becomes `{"result_file": path}`
  with a short `summary` like `"results written to <path>"` — mirrors
  `fleet_run.go:204-213`'s pattern). `store`/`status`/`admin *` accept
  `--output` but their `data` is already small — writing to a file is
  allowed but not expected to be exercised; do not special-case it away,
  just let the same generic `--output` handling apply uniformly.
- **`--cursor`**: **investigated and decided: explicitly OUT OF SCOPE, not
  silently dropped.** Verified 2026-08-11 by reading the signatures of
  `search.FuzzySearch`, `search.ExactSearch` (both in
  `semantic/src/core/search/fuzzy.go:174,448`), and
  `search.TraceDependencies` (`semantic/src/core/search/graph.go:51`): **none
  accepts an offset/cursor parameter today.** Implementing true cursor
  pagination would require changing the underlying query layer, which is out
  of scope for a CLI contract migration. Decision: `--limit`, `--fields`,
  and `--output` are implemented (this section); `--cursor` is **not**
  implemented. Rationale: result sets are already bounded — `--limit`
  defaults to 10, `--max-nodes` defaults to 100 — so mk-cli's rule 9 ("Do not
  return huge lists by default") is satisfied without it. Each affected
  `--schema` output's top-level `description` (or an `"unsupported"` note
  under `errors`/`input`) must say cursor pagination is not implemented.
  This is a documented, deliberate limitation — not an oversight — and does
  not block `HSME-006`'s Definition of Done.

### 6.7 Gap 7 — Agent documentation

Two deliverables, both required by AC-11 (§12):

1. **`AGENTS.md` (root) entry**, appended as a new subsection near the
   existing HSME material (after the "HSME subordination in this repo (ADR
   0008)" section), following mk-cli's exact template shape
   ("Documentation for Agents" → "AGENTS.md"):

   ```md
   ## hsme-cli for agents

   Use `hsme-cli` for HSME memory operations instead of querying its SQLite
   database directly.

   Default agent flags:
   - Always pass `--json`.
   - Always pass `--no-input`.
   - Use `--dry-run` before `store`, `admin restore`, `import-quorum`.
   - Prefer `--output <file>` for large `search-fuzzy`/`search-exact`/`explore` results.
   - Do not parse human-text output when `--json` is available.

   Common commands:
   - `hsme-cli search-fuzzy "<query>" --project <proj> --limit 10 --json --no-input`
   - `hsme-cli search-exact "<keyword>" --project <proj> --limit 10 --json --no-input`
   - `hsme-cli status --json --no-input`
   - `hsme-cli store --source-type note --project <proj> --json --no-input < notes.md`
   ```

   This is additive prose; it does not alter any of the existing ADR 0008
   subordination language already in `AGENTS.md` (§ "HSME subordination in
   this repo").

2. **New progressive-disclosure skill doc** at
   `.agents/skills/hsme-cli-usage/SKILL.md` (new directory), following
   mk-cli's "Skill" template exactly — frontmatter with strong trigger
   words near the start of `description`, then only: when to use / when not
   to use / required agent flags / 5-10 common commands / the JSON success +
   error contract shapes (quoted, not the full `--help` text) / error-code
   handling rules. Do **not** paste `help.go`'s full per-subcommand help
   text into this skill file — mk-cli explicitly warns against that
   ("Inside `SKILL.md`, do not paste the full CLI help").

### 6.8 Cross-cutting: existing already-compliant surfaces to leave alone

- `help.go`'s layered-help *mechanism* (`printTopLevelHelp`/`runHelp`
  dispatch) is unchanged. Only the doc-string *text* inside each `case` of
  `runHelp` is edited: remove `--format` references, add the new flags
  (`--json`, `--no-input`, `--quiet`, `--verbose`, `--output`, `--timeout`,
  `--schema`, and `--dry-run` where applicable) to each subcommand's `Flags:`
  block.
- `main.go`'s dispatcher `switch` and `ScanTrailingFlags`/`isBoolFlag`
  mechanism are unchanged.
- The human-text `Format*Result` functions in `output.go` are unchanged —
  only their call sites' surrounding `if outputFormat == "json"` branches
  are replaced with `if agentFlags.JSON`.

---

## 7. Per-Subcommand Flag Matrix (final state)

| Subcommand | Existing flags kept | New universal flags | New command-specific flags |
|---|---|---|---|
| `store` | `--source-type`, `--project`, `--supersedes`, `--force-reingest` | `--json --no-input --quiet --verbose --output --timeout --schema` | `--dry-run` |
| `search-fuzzy` | `--limit`, `--project` | same | `--fields` |
| `search-exact` | `--limit`, `--project` | same | `--fields` |
| `explore` | `--direction` (now validated), `--max-depth`, `--max-nodes` | same | `--fields` |
| `status` | `--watch`, `--interval` | same | — |
| `admin retry-failed` | (none) | same | — |
| `admin backup` | `--dest` | same | — |
| `admin restore` | `--from`, `--latest` | same | `--dry-run` |
| `import-quorum` | `--project`, `--quorum-project`, `--quorum-db`, `--tasks-root`, `--source` | same | `--dry-run` |
| (top level) | `--db`, `--ollama-host`, `--embedding-model`, `--no-color` | `--version` | — |

---

## 8. Caller Inventory & Required Edits (verbatim checklist)

Every item below was verified by direct `grep`/`Read` on 2026-08-11. Update
all of them in the same change as the CLI rewrite (§1 constraint).

### 8.1 `.agents/skills/q-memory/SKILL.md`

Lines with `hsme-cli`: **138, 143, 150, 172, 186**.

- Line 143: `SQLITE_DB_PATH="<hsme-db-path>" timeout 20 hsme-cli search-fuzzy
  "<proposed_title_and_content>" --project quorum --limit 10` → append
  `--json --no-input`.
- Line 150: same for `search-exact "<proposed_title>"`.
- Prose at lines 138, 172, 186 mentioning `hsme-cli` and "the result's
  highlighted text" (line ~154 context, "Every result carries provenance")
  must be updated to say results are read from the JSON envelope's
  `data.results[].memory_id` and `data.results[].highlights[].text` (the
  underlying `search.MemorySearchResult`/highlight shape is unchanged by
  this migration — only the wrapping envelope is new).
- Line 172's "Graceful Degradation" text ("If `hsme-cli` is missing, times
  out (>20s), errors, or returns no results...") is unaffected logically;
  confirm it doesn't reference `--format` (it doesn't) and leave as-is
  beyond the flag-string edits above.

### 8.2 `.agents/skills/q-session/SKILL.md`

Lines with `hsme-cli`: **113, 118, 125, 147, 172**. Same edits as §8.1
(this file has a near-identical Pre-Save Duplicate Advisor section): add
`--json --no-input` to the `search-fuzzy` (line 118) and `search-exact`
(line 125) invocations; update the provenance prose the same way.

### 8.3 `.agents/skills/q-brief/SKILL.md`

Lines with `hsme-cli`: **44, 49, 52, 56, 67**.

- Line 49: `hsme-cli search-fuzzy "<interview_topic>" --project quorum
  --limit 10` → append `--json --no-input`.
- Line 56: `hsme-cli search-exact "<interview_topic>" --project quorum
  --limit 10` → append `--json --no-input`.
- Lines 44, 52, 67 are prose referencing `hsme-cli` generically (invocation
  description, fallback description, graceful-degradation description) —
  no flag strings to edit there, but confirm none silently assume
  text-format output; if they do, adjust the wording to reference JSON
  fields as in §8.1.

### 8.4 `.agents/skills/q-blueprint/SKILL.md`

Lines with `hsme-cli`: **43, 48, 51, 55, 67**. Same pattern as §8.3 (this
skill's HSME advisor hook is near-identical to `q-brief`'s): add `--json
--no-input` to lines 48 and 55's commands; adjust surrounding prose.

### 8.5 `semantic/justfile`

Lines with `hsme-cli`/`bin/hsme-cli`: **25, 50, 96, 100, 104, 108** (build,
install, `retry-failed`, `backup`, `restore`, `clean` recipes).

**None of these recipes pass `--format`** (verified: `grep -n "hsme-cli"
semantic/justfile` shows only bare invocations like `./bin/hsme-cli admin
retry-failed`). **No line edits are structurally required** — these are
human-facing `just` recipes (Spanish `@echo` messages, no JSON parsing) that
invoke the compiled binary by name only, and the binary name (`hsme-cli`)
and build path (`./cmd/cli`) are unchanged by this migration. Action for
this task: re-run `just retry-failed`/`just backup`/`just restore` once the
new binary is built, to confirm they still exit 0 against the new error/flag
surface (they will, since none of their invoked subcommands pass a flag this
migration removes). Do not add `--json` to these — they are human console
output, not agent input.

### 8.6 `README.md` (root)

Line with `hsme-cli`: **505** — a directory-tree listing (`├──
cmd/{hsme,worker,ops,cli}  # binarios: hsme, hsme-worker, hsme-ops,
hsme-cli`), not a usage example. **No flag-string edit required.** Re-grep
`README.md` for `hsme-cli` after the CLI changes are drafted, in case a
usage example was added elsewhere in the interim; as of 2026-08-11 there is
exactly one hit and it needs no change.

### 8.7 `AGENTS.md` (root)

Line with `hsme-cli`: **331** — prose inside the "HSME subordination in
this repo (ADR 0008)" section ("Operational rules for every HSME call made
from this repo (MCP tools or `hsme-cli`): always pass `project`..."). No
flag string to edit (it's already flag-agnostic prose). **This is where the
new §6.7.1 "hsme-cli for agents" subsection is added** (append after this
paragraph, not inside it).

### 8.8 `semantic/README.md`

Lines with `hsme-cli`: **171, 173, 174, 175, 179**. This file is written
**entirely in Spanish** — keep all new content in Spanish for consistency;
do not switch to English.

- Existing examples (`hsme-cli status`, `watch -n 2 -c "hsme-cli status"`,
  `hsme-cli admin retry-failed`) pass no `--format` flag, so they remain
  syntactically valid as human commands with no edit required.
- Add one short paragraph after line 179 (inside "Comandos de Utilidad" or
  as a new subsection) documenting the agent contract in Spanish, e.g.:
  `Para uso por agentes, hsme-cli expone --json (salida estructurada),
  --schema (contrato de entrada/salida de cada subcomando) y --dry-run en
  store/admin restore/import-quorum (simulación sin escritura). Ver
  AGENTS.md y .agents/skills/hsme-cli-usage/SKILL.md.`

### 8.9 `semantic/tests/modules/cli_test.go`, `cli_import_quorum_test.go`, `semantic/cmd/cli/*_test.go`

Full rewrite scope already detailed in §2.3. Concretely:

- `cli_test.go:79`: `runCLI("--format", "json", "search-fuzzy", "test")` →
  `runCLI("search-fuzzy", "test", "--json")`; assertion on `"results"` at
  top level → assert on `"\"data\""` and `"\"results\""` nested under
  `data`, plus `"\"ok\":true"` and `"\"command\":\"search-fuzzy\""`.
- `cli_import_quorum_test.go` lines 56, 85, 106, 125, 179: same `--format
  json` → `--json` substitution; assertions against `FormatImportQuorumResult`
  bare-map JSON must instead assert against the new envelope's `data.curated`
  / `data.capsules` nesting.
- `flags_test.go`: `TestRegisterDBFlags` must drop the `-format json
  -no-color` parse assertions tied to `outputFormat`, and either move a
  `TestRegisterAgentFlags` test into a new `flags_test.go` block (or a new
  `envelope_test.go`) asserting `--json`/`--no-input`/`--quiet`/`--verbose`/
  `--output`/`--timeout`/`--schema` all parse into an `AgentFlags` struct
  correctly. Keep the `--no-color`/`--db`/`--ollama-host`/`--embedding-model`
  assertions (unchanged flags).
- `output_test.go`: `TestWriteResult`/`TestWriteError` are testing function
  signatures being deleted (`WriteResult(w, v, format string)`, `WriteError(w,
  err error, code int, format string)`). Replace with tests against
  `WriteSuccessEnvelope`/`WriteErrorEnvelope` asserting the exact JSON key
  set (`ok`, `command`, `summary`, `data`, `next_actions` / `ok`, `command`,
  `error.code`, `error.message`, `retryable`). `TestFormatJSON` and
  `TestColorFunctions` are untouched (those functions aren't removed).
- `help_test.go`: structurally fine; extend `TestRunHelp`'s per-subcommand
  cases to also assert the new flags appear in the printed usage text if
  `help.go`'s doc-strings are updated per §6.8 (optional strengthening, not
  required for AC — the existing substring assertions still pass either
  way since they check for presence of `"hsme-cli store"` etc., not
  absence of new text).

### 8.10 Explicitly NOT a caller (verified, no action)

`~/.claude/skills/` — grepped, zero references to `hsme-cli`. No action.

---

## 9. Out of Scope

- `semantic/kitty-specs/**` — frozen historical archives. **Do not touch,
  do not grep-and-replace inside them even if they happen to contain
  `--format` examples.** Verify with `git diff --stat` after the change
  shows zero lines under this path.
- The `hsme-worker`/`hsme-ops`/`hsme` (MCP server) binaries and their
  `cmd/worker`, `cmd/ops`, `cmd/hsme` sources — untouched. This migration is
  scoped to `cmd/cli` only.
- `~/.config/hsme/worker.env` — the systemd worker's environment file,
  unrelated to this CLI migration. **Do not print its contents** (it holds
  an API key) at any point in this task's execution, including in logs,
  commit messages, or artifact fields.
- No new subcommands. The subcommand set stays exactly `store`,
  `search-fuzzy`, `search-exact`, `explore`, `status`, `admin
  {retry-failed,backup,restore}`, `import-quorum`.
- No DB schema changes. This is a CLI contract migration only; no
  `semantic/src/core/*` query logic, table, or migration changes beyond the
  read-only inspection needed to classify errors (§6.2) and check for
  `--cursor` feasibility (§6.6).
- `--plain`, `--debug`, `--cwd`, `--config`, `--yes`, `--cursor` — considered
  and explicitly excluded per §5.2/§6.6.
- `admin backup --dry-run` — considered and explicitly excluded per §6.5.
- No changes to `SQLITE_DB_PATH` env var resolution behavior
  (`semantic/src/bootstrap/`) — flag/env precedence for `--db` is unchanged.
- No cross-module import in either direction (ADR 0008, restated from §4).

---

## 10. Environment & Build/Test Context

- **Golden rule: never build or test `semantic/` from the repo root.** Always
  `cd /home/gary/dev/quorum/semantic` first.
- Build/install: `cd semantic && just install` — builds with `CGO_ENABLED=1`
  and tags `sqlite_fts5 sqlite_vec` (see `justfile:4-5`). This produces
  `bin/hsme-cli` (via the `cli-build`/`cli-install` recipes, `justfile:22-25,
  47-50`) plus `hsme`, `hsme-worker`, `hsme-ops`.
- Test: `cd semantic && just test` → `go test -v -tags "sqlite_fts5
  sqlite_vec" ./...` (`justfile:36-37`).
- Binary name is fixed as `hsme-cli` — do not rename it. `hsme` is already
  taken by the MCP server binary (`cmd/hsme`).
- `SQLITE_DB_PATH` env var selects the HSME database (default resolution in
  `semantic/src/bootstrap/`, referenced by all the `.agents/skills/q-*`
  advisor hooks in §8.1-8.4). This migration does not change its resolution
  logic.
- `semantic/CLAUDE.md`'s **entire content** is the "HSME Protocol" section
  governing how an AI agent should call the HSME **MCP tools**
  (`recall_recent_session` for recency queries, `search_fuzzy`/`search_exact`
  for semantic/topic queries, `explore_knowledge_graph` for tracing). It
  contains **no Go code-style conventions** for editing files under
  `semantic/` — do not invent style rules attributed to that file; there
  are none beyond this MCP-tool-usage protocol.
- Effort estimate for the full `HSME-006` parent: **M (2-3 days)**. The
  envelope+error-schema rewrite across all 7 subcommands (§6.1, §6.2) is the
  hardest gap — it touches every `runXxx` function and both test suites.

---

## 11. Definition of Done

- [ ] `--format text|json` and the `outputFormat` package var no longer
      exist anywhere in `semantic/cmd/cli/*.go` (`grep -rn 'outputFormat\|--format'
      semantic/cmd/cli` returns nothing outside comments referencing the old
      behavior for historical context, if any are left — prefer zero hits).
- [ ] All 7 subcommands (`store`, `search-fuzzy`, `search-exact`, `explore`,
      `status`, `admin {retry-failed,backup,restore}`, `import-quorum`)
      answer `--json` with the `{ok, command, summary, data, next_actions}`
      envelope on success and the `{ok:false, command, error, retryable,
      suggested_fix}` envelope on failure.
- [ ] All 7 subcommands answer `--schema` without touching the DB or network.
- [ ] `store`, `admin restore`, `import-quorum` answer `--dry-run` with no
      side effects.
- [ ] Every caller in §8.1-§8.9 is updated exactly as specified; §8.10's
      non-caller is reconfirmed with a fresh grep.
- [ ] `semantic/kitty-specs/**` shows zero diff.
- [ ] `cd semantic && just test` exits 0.
- [ ] `cd /home/gary/dev/quorum && go test ./...` (root module) still passes
      with `CGO_ENABLED=0` and no cross-module import introduced (ADR 0008
      acid test unaffected — this migration never touches the root module).
- [ ] `AGENTS.md` (root) has the new "hsme-cli for agents" subsection;
      `.agents/skills/hsme-cli-usage/SKILL.md` exists and follows the
      progressive-disclosure template (no pasted full help text).
- [ ] `semantic/README.md` has the new Spanish agent-contract paragraph.

---

## 12. `/q-brief` Input for `HSME-006`

### Objective

Migrate `hsme-cli` (`semantic/cmd/cli/*.go`) to strictly follow the mk-cli
agent-CLI conventions (`/home/gary/Documents/make-clis/mk-cli-EN.md`),
replacing its ad-hoc `--format text|json` / bare-map JSON / untyped-error
surface with the `{ok, command, summary, data, next_actions}` /
`{ok:false, command, error:{code,message,field,received}, retryable,
suggested_fix}` contract, `--schema` introspection, and `--dry-run` on
mutating commands — mirroring the pattern already proven in this repo by
`quorum fleet run` (`cmd/fleet_run.go`, `cmd/fleet_agentio.go`) without any
cross-module import (ADR 0008). This is a breaking change: the old flag
surface is removed, and every verified caller (4 `.agents/skills/q-*`
files, test suites, `AGENTS.md`, `semantic/README.md`) is updated in the
same change.

### Declared risk

**Medium** — breaking change to a CLI surface, but with a fully bounded and
pre-verified caller inventory (§8) and no data/schema migration involved.

### Acceptance Criteria

1. **AC-1**: All 7 subcommands (9 distinct `command` envelope values per
   §5.4 — `admin` contributes 3), under `--json`, emit exactly one JSON
   object on stdout matching `{ok, command, summary, data, next_actions}` on
   success (§6.1).
2. **AC-2**: All 7 subcommands (9 distinct `command` envelope values per
   §5.4), under `--json`, emit `{ok:false, command, error:{code, message,
   field, received}, retryable, suggested_fix}` on failure, using only the
   codes, `retryable` values, and exit codes (`exitUsage`/`exitRuntime` per
   §6.2's exit-code rule) in the §6.2 table.
3. **AC-3**: both greps return zero hits: (a) `grep -rn 'outputFormat'
   semantic/cmd/cli` (the removed package var — code only, no exclusions,
   since the identifier no longer exists once removed); (b)
   `grep -rln -- '--format' semantic/cmd/cli/ README.md AGENTS.md
   semantic/README.md semantic/justfile .agents/skills/` (the removed flag
   string across every caller inventoried in §8; `semantic/kitty-specs/` is
   excluded by not being listed, per AC-12).
4. **AC-4**: All 7 subcommands (9 distinct `--schema` outputs per §5.4, one
   per `command` value) answer `--schema` (exits 0, prints the `{command,
   description, input, output, errors}` shape from §6.4) without opening the
   DB or making a network call.
5. **AC-5**: `store`, `admin restore`, `import-quorum` accept `--dry-run`
   and perform no DB write / file write / restore when set, returning
   `data.dry_run: true`.
6. **AC-6**: `--no-input`, `--quiet`, `--verbose`, `--output`, `--timeout`,
   `--schema` are accepted by all 7 subcommands (all 9 `command` values
   under them, per §5.4); `--version` is accepted at the top level.
   `--output` on `search-fuzzy`/`search-exact`/`explore`/`import-quorum`
   writes the full result to the given file and the stdout envelope's
   `data` contains `result_file`.
7. **AC-7**: `search-fuzzy`, `search-exact`, `explore` accept `--fields
   <a,b,c>` and project result items to the requested field subset when set.
8. **AC-8**: `explore --direction` rejects any value other than
   `upstream|downstream|both` with `INVALID_ENUM` before calling
   `search.TraceDependencies`.
9. **AC-9**: Each of `.agents/skills/q-memory/SKILL.md`,
   `.agents/skills/q-session/SKILL.md`, `.agents/skills/q-brief/SKILL.md`,
   `.agents/skills/q-blueprint/SKILL.md` (§8.1-§8.4) satisfies, mechanically:
   (a) every line matching `search-fuzzy|search-exact` also contains both
   substrings `--json` and `--no-input`; (b) the exact substring
   `data.results[].memory_id` appears at least once; (c) the exact substring
   `data.results[].highlights[].text` appears at least once.
10. **AC-10**: `semantic/tests/modules/cli_test.go`,
    `cli_import_quorum_test.go`, and `semantic/cmd/cli/*_test.go` are
    rewritten per §8.9 to assert the new envelope/flag surface; `cd
    semantic && just test` exits 0.
11. **AC-11**: `AGENTS.md` (root) has the new "hsme-cli for agents"
    subsection (§6.7 item 1) and `.agents/skills/hsme-cli-usage/SKILL.md`
    exists per the mk-cli "Skill" template (§6.7 item 2).
12. **AC-12**: `git diff --stat` shows zero changes under
    `semantic/kitty-specs/`.
13. **AC-13**: No file is modified outside `semantic/` other than
    `.agents/skills/{q-memory,q-session,q-brief,q-blueprint}/SKILL.md`,
    root `README.md` (if a usage example is found on re-grep per §8.6),
    root `AGENTS.md`, and the new `.agents/skills/hsme-cli-usage/SKILL.md`;
    `cd /home/gary/dev/quorum && go test ./...` (root module, `CGO_ENABLED=0`,
    no C compiler) still passes untouched.

### Proposed Decomposition

Three sequential children (`HSME-006-a` → `HSME-006-b` → `HSME-006-c`, each
declaring `depends_on` its predecessor per `.agents/schemas/spec.schema.json`'s
`decomposition[].depends_on` field — this is **not** a parallel 3-way fan-out:
`-b` and `-c` structurally require `envelope.go`/`AgentFlags` to already
exist, since they call the same shared functions `-a` introduces).

#### `HSME-006-a` — Core envelope + universal flags + highest-traffic read commands (band M)

Scope: gaps 1-4 infrastructure, plus `store`, `search-fuzzy`, `search-exact`,
`explore` (the 4 commands the `.agents/skills/q-*` advisor hooks call, plus
`store` as the other primary ingestion path).

Touch list:
- `semantic/cmd/cli/envelope.go` (new)
- `semantic/cmd/cli/flags.go`
- `semantic/cmd/cli/main.go`
- `semantic/cmd/cli/output.go`
- `semantic/cmd/cli/store.go`
- `semantic/cmd/cli/search.go`
- `semantic/cmd/cli/explore.go`
- `semantic/cmd/cli/help.go` (doc-string updates for `store`, `search-fuzzy`,
  `search-exact`, `explore` only)
- `semantic/cmd/cli/flags_test.go`
- `semantic/cmd/cli/output_test.go`

Verify commands:
```
cd semantic && go build -tags "sqlite_fts5 sqlite_vec" ./cmd/cli/...
cd semantic && go test -tags "sqlite_fts5 sqlite_vec" ./cmd/cli/...
```

#### `HSME-006-b` — Remaining commands: status, admin, import-quorum (band S)

`depends_on: ["HSME-006-a"]`. Scope: gaps 1-6 applied to the remaining 5
subcommand entry points (`status`, `admin retry-failed`, `admin backup`,
`admin restore`, `import-quorum`), including `--dry-run` on `admin restore`
and `import-quorum` (gap 5). The `--cursor` OUT-OF-SCOPE decision (gap 6,
§6.6) is already final and verified against
`search.FuzzySearch`/`search.ExactSearch`/`search.TraceDependencies` —
`import-quorum` requires no further cursor-related action since it is not
list-shaped.

Touch list:
- `semantic/cmd/cli/status.go`
- `semantic/cmd/cli/admin.go`
- `semantic/cmd/cli/import_quorum.go`
- `semantic/cmd/cli/help.go` (doc-string updates for `status`, `admin *`,
  `import-quorum` only — rebase on `-a`'s merged `main` first to avoid
  conflicting edits to the same file)
- `semantic/tests/modules/cli_test.go`
- `semantic/tests/modules/cli_import_quorum_test.go`

Verify commands:
```
cd semantic && go build -tags "sqlite_fts5 sqlite_vec" ./cmd/cli/...
cd semantic && just test
```

#### `HSME-006-c` — Caller updates + agent documentation (band S)

`depends_on: ["HSME-006-a", "HSME-006-b"]`. Scope: gap 7, plus the full
caller inventory from §8 (this child cannot start meaningfully before the
final flag surface exists, since it embeds exact example commands and JSON
shapes into skills/docs).

Touch list:
- `.agents/skills/q-memory/SKILL.md`
- `.agents/skills/q-session/SKILL.md`
- `.agents/skills/q-brief/SKILL.md`
- `.agents/skills/q-blueprint/SKILL.md`
- `.agents/skills/hsme-cli-usage/SKILL.md` (new)
- `README.md` (root, only if re-grep per §8.6 finds a usage example to
  update; otherwise no edit)
- `AGENTS.md` (root)
- `semantic/README.md`
- `semantic/justfile` (no functional edit expected per §8.5; touch only if
  the re-verification run surfaces an actual failure)

Verify commands:
```
grep -rln -- '--format' .agents/skills semantic/justfile README.md AGENTS.md semantic/README.md | grep -v kitty-specs
# must return no output
cd semantic && just test
cd /home/gary/dev/quorum && go test ./...
```
