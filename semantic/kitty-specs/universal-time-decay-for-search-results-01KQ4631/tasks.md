# Work Packages: Universal Time-Decay for Search Results

**Mission**: `universal-time-decay-for-search-results-01KQ4631`
**Branch**: `main`

## Subtask Index
| ID | Description | WP | Parallel |
|---|---|---|---|
| T001 | Implement the shared decay math primitive and age helper in `src/core/search/decay.go` | WP01 | | [D] |
| T002 | Load and validate `RRF_TIME_DECAY` / `RRF_HALF_LIFE_DAYS` at server startup | WP01 | | [D] |
| T003 | Add focused unit tests for decay math, future timestamp clamping, and invalid config values | WP01 | | [D] |
| T004 | Extend fuzzy-search chunk hydration to fetch `memories.created_at` and preserve the decay-off fast path | WP02 | | [D] |
| T005 | Apply the decay factor inside fuzzy chunk scoring before per-memory aggregation | WP02 | | [D] |
| T006 | Add decay-aware exact-search ordering for both FTS5 BM25 rows and substring fallback rows | WP02 | | [D] |
| T007 | Add integration tests and golden fixtures proving byte-equivalence when decay is off and expected reordering when decay is on | WP02 | | [D] |
| T008 | Build `cmd/bench-decay` to run paired OFF/ON evaluations against the frozen corpus | WP03 | | [D] |
| T009 | Emit JSON and Markdown benchmark reports under `data/benchmarks/<run_id>/` and include at least 5 `search_exact` samples | WP03 | | [D] |
| T010 | Add harness smoke coverage for CLI/report generation and read-only DB access | WP03 | | [D] |
| T011 | Document runtime usage, env vars, benchmark invocation, and rollback flow in `README.md` | WP04 | | [D] |
| T012 | Produce and retain at least one benchmark audit run showing mission acceptance metrics | WP04 | | [D] |

## WP01: Decay Primitive & Startup Config
**Goal**: Create the shared time-decay primitive, validate the env-driven config, and wire server startup so invalid values fail loudly before any MCP request is served.
**Prompt**: `tasks/WP01-decay-primitive-startup-config.md` (~260 lines)
**Dependencies**: None
**Included Subtasks**:
- [x] T001 Implement the shared decay math primitive and age helper in `src/core/search/decay.go` (WP01)
- [x] T002 Load and validate `RRF_TIME_DECAY` / `RRF_HALF_LIFE_DAYS` at server startup (WP01)
- [x] T003 Add focused unit tests for decay math, future timestamp clamping, and invalid config values (WP01)

## WP02: Search Ranking Integration
**Goal**: Apply the shared decay primitive to both search surfaces without breaking the byte-identical decay-off path.
**Prompt**: `tasks/WP02-search-ranking-integration.md` (~360 lines)
**Dependencies**: WP01
**Included Subtasks**:
- [x] T004 Extend fuzzy-search chunk hydration to fetch `memories.created_at` and preserve the decay-off fast path (WP02)
- [x] T005 Apply the decay factor inside fuzzy chunk scoring before per-memory aggregation (WP02)
- [x] T006 Add decay-aware exact-search ordering for both FTS5 BM25 rows and substring fallback rows (WP02)
- [x] T007 Add integration tests and golden fixtures proving byte-equivalence when decay is off and expected reordering when decay is on (WP02)

## WP03: Benchmark Harness & Reports
**Goal**: Add the standalone benchmark binary that compares decay OFF vs ON and emits auditable reports for both search tools.
**Prompt**: `tasks/WP03-benchmark-harness-reports.md` (~320 lines)
**Dependencies**: WP01, WP02
**Included Subtasks**:
- [x] T008 Build `cmd/bench-decay` to run paired OFF/ON evaluations against the frozen corpus (WP03)
- [x] T009 Emit JSON and Markdown benchmark reports under `data/benchmarks/<run_id>/` and include at least 5 `search_exact` samples (WP03)
- [x] T010 Add harness smoke coverage for CLI/report generation and read-only DB access (WP03)

## WP04: Documentation & Acceptance Evidence
**Goal**: Document the operator workflow and preserve the benchmark evidence needed to accept or roll back the feature safely.
**Prompt**: `tasks/WP04-documentation-acceptance-evidence.md` (~220 lines)
**Dependencies**: WP03
**Included Subtasks**:
- [x] T011 Document runtime usage, env vars, benchmark invocation, and rollback flow in `README.md` (WP04)
- [x] T012 Produce and retain at least one benchmark audit run showing mission acceptance metrics (WP04)
