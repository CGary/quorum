# ADR 0012: Optional Per-Class Diff Limits in Contract Checking

## Status
Accepted

## Context

`02-contract.yaml.limits.max_diff_lines` (and `contract-check`'s enforcement
of it, `internal/core/contract_check.go`) is a single global cap over the
aggregate insertion+deletion count of a task's diff. A blind global cap does
not distinguish file categories that legitimately grow at very different
rates. FLEET-021 is the concrete case that motivated this task: a 13-line
production fix paired with 216 lines of table-driven tests. Under a single
120-line global cap, that diff either fails a contract that is actually
sound (tests correctly outweigh the fix many times over, per this
project's own historical ratios -- see the sizing guidance added to
`.agents/skills/q-blueprint/SKILL.md`), or the contract author is forced to
set a cap loose enough to also let a runaway production-code diff through
unchecked. Neither is acceptable: the manifesto (`quorum.md`) treats risk
and sizing as signal-based, author-estimated per contract -- never a
framework-fixed constant -- and a single global number cannot encode "tests
can run 4-6x the fix size, production code should not."

Constraints verified against the real schema and code (not assumed):

- `contract.schema.json`'s `limits` object has `additionalProperties: false`
  and only `max_files_changed`/`max_diff_lines`/`max_cost_usd` are declared
  today; any new key must be added explicitly, and `max_diff_lines` stays
  required as the backstop.
- `internal/core/contract_check.go`'s `CheckContract` is a pure function
  called with exactly `(contract, changedFiles, diffStat)` from
  `cmd/analyze_contract_check.go` and from `internal/core/contract_check_test.go`.
  `contract-check` is dormant (no live skill caller yet, per
  `01-blueprint.yaml`'s non_goals), but its CLI/stdin shape is still
  exercised by the golden-master harness, so any signature change must stay
  additive.
- `internal/core/risk.go`'s `safeGlobMatch` is the only glob matcher in the
  package and is documented (in its own comments) to have a one-directory-
  level limitation for `**`: a leading `**/X` matches `X` at any path
  depth, but a trailing `dir/**` or a mid-pattern `**` only crosses a
  single directory level, not arbitrary depth. This task reuses
  `safeGlobMatch` unmodified rather than introducing a second matcher (see
  Non-Goals in `00-spec.yaml`).

## Decision

### 1. `limits.per_class`: an optional, ordered list of budgets

`ContractLimits` gains an optional `PerClass []PerClassLimit` field
(`per_class` in YAML/JSON, `omitempty`), where each entry is
`{glob, max_diff_lines}`. `max_diff_lines` remains required at the top
level and is the fallback for any file that is not claimed by a
`per_class` entry. `contract.schema.json` declares `per_class` explicitly
under `properties.limits.properties` (an array of objects, each requiring
`glob` and `max_diff_lines`, `additionalProperties: false`) without adding
it to `limits.required`, so every contract written before this task
validates unchanged (AC-5).

Per-class evaluation is **first-match-wins in declaration order**: for each
file with per-file diff data, `CheckContract` walks `per_class` in the
order the contract author wrote it and applies the first glob (via
`safeGlobMatch`) that matches that file's path. A matched file's own line
count is checked against that class's `max_diff_lines` instead of being
folded into the aggregate check; an unmatched file is left entirely to the
pre-existing aggregate `limits.max_diff_lines` check (no new per-file
global check is introduced). A per-class violation
(`type: "max_diff_lines_per_class"`, `rule: "limits.per_class"`) reports
the matched glob plus the measured and budgeted line counts in `detail`,
additive to (never replacing) the existing `max_diff_lines` violation type.

### 2. The request shape evolves additively

`cmd/analyze_contract_check.go`'s `contractCheckRequest` gains an optional
`file_diffs []core.FileDiff` (`{path, insertions, deletions}`,
`omitempty`). `core.CheckContract` gains this data as a trailing variadic
`fileDiffs ...FileDiff` parameter specifically so every existing
three-argument call site -- the CLI shim itself, and every test in
`contract_check_test.go` -- keeps compiling and behaving identically
without being touched. A stdin request that omits `file_diffs` (the
pre-existing shape) produces a nil slice, and per-class evaluation is
skipped outright: `CheckContract` degrades to global-limit-only checking
with no error (AC-4). Per-class evaluation only ever runs when both
`file_diffs` is non-empty **and** `contract.Limits.PerClass` is non-empty,
which also guarantees AC-5: a legacy contract with no `per_class` produces
byte-identical output to before this task, regardless of what the caller
passes in `file_diffs`.

### 3. The `safeGlobMatch` depth limitation applies to `per_class` globs too

Because `per_class` reuses `safeGlobMatch` unmodified, its documented
one-directory-level `**` behavior carries over directly: a trailing
`dir/**` or a mid-pattern `**` matches only one path segment beneath it,
not arbitrary depth. Contract authors who need "match this file anywhere
in the tree" must use a leading `**/X` form (e.g. `**/auth/**`, which
`safeGlobMatch` special-cases as "does any path component equal `auth`"),
or a bare basename pattern such as `*_test.go`, which `safeGlobMatch`
matches against `filepath.Base(path)` regardless of directory depth. This
task documents the limitation in two places rather than fixing the
matcher: here, and in the sizing-guidance section added to
`.agents/skills/q-blueprint/SKILL.md`, so contract authors see it at the
point they are writing `per_class` globs.

### 4. The pre-existing schema/checker discrepancy is noted, not fixed

`contract.schema.json` already requires `limits.max_diff_lines` at the
schema level, but `CheckContract` treats `ContractLimits.MaxDiffLines` as a
nilable pointer and silently skips the check when it is absent -- a
discrepancy that predates this task. `per_class` does not touch or resolve
it: `PerClassLimit.MaxDiffLines` is a plain (non-pointer) `int`, required
by both the schema and the Go struct, so this task introduces no new
instance of the discrepancy, but it also does not fix the existing one.
Fixing it is out of scope (`00-spec.yaml` non_goals) and left for a future
task if the divergence ever causes a real incident.

## Consequences

- **Positive**: contract authors can now express "tests may run several
  times larger than the fix, production code should stay tight" as data in
  `02-contract.yaml`, instead of picking one blind global number that is
  either too loose for code or too tight for tests -- directly addressing
  the FLEET-021 shape of diff. This is consistent with `quorum.md`'s
  signal-based risk governance: budgets are per-contract author estimates,
  never framework-fixed constants.
- **Positive**: the request and function-signature evolution is fully
  additive (variadic `fileDiffs`, `omitempty` JSON field, non-required
  schema key), so every pre-existing caller of `CheckContract` and every
  pre-existing `02-contract.yaml` keeps working without modification.
- **Negative**: `per_class` globs inherit `safeGlobMatch`'s one-directory-
  level `**` limitation; an author who reaches for `dir/**` expecting
  any-depth matching will get a contract that silently fails to budget
  files it was meant to cover. This is mitigated by explicit documentation
  in this ADR and in the sizing-guidance text, not by changing the
  matcher (changing `safeGlobMatch` was explicitly out of scope, per
  `00-spec.yaml` non_goals, since it also backs `internal/core/risk.go`'s
  `sensitive_paths` matching).
- **Neutral**: `contract-check` remains dormant -- this task does not wire
  it into any skill's live invocation path (`00-spec.yaml`/`01-blueprint.yaml`
  non_goals) -- so the risk surface of this change is contained to CLI and
  schema correctness, not live task execution, until a future task adopts
  it as a gate.
