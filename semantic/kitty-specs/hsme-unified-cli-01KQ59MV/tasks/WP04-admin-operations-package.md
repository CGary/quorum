---
work_package_id: WP04
title: Admin Operations Package
dependencies:
- WP01
requirement_refs:
- FR-030
- FR-031
- FR-032
- FR-033
- FR-034
- FR-035
- FR-036
planning_base_branch: main
merge_target_branch: main
branch_strategy: Planning artifacts for this feature were generated on main. During /spec-kitty.implement this WP may branch from a dependency-specific base, but completed changes must merge back into main unless the human explicitly redirects the landing branch.
subtasks:
- T020
- T021
- T022
- T023
agent: "gemini:o3:reviewer:reviewer"
shell_pid: "2109447"
history:
- date: '2026-04-26T16:47:42Z'
  action: tasks generated
  actor: tasks skill
agent_profile: ''
authoritative_surface: src/core/admin/
execution_mode: code_change
model: ''
owned_files:
- src/core/admin/retry.go
- src/core/admin/backup.go
- src/core/admin/restore.go
- src/core/admin/admin_test.go
role: ''
---
---

## ⚡ Do This First: Load Agent Profile

Before reading anything else, load the implementer agent profile:

```
/ad-hoc-profile-load implementer
```

This injects your role identity, skill directives, and execution context. All other instructions in this prompt are subordinate to the profile load.

---

## Objective

Create `src/core/admin/` package with `RetryFailed`, `Backup`, and `Restore` logic, plus integration tests. `cmd/cli/admin.go` stays thin (parsing + dispatch only).

---

## Context

### Feature directory

`/home/gary/dev/hsme/kitty-specs/hsme-unified-cli-01KQ59MV`

### Dependencies

- **WP01 (bootstrap)** must be complete — `src/bootstrap/` is imported by the admin package.
- Research decisions: R2 (backup API), R3 (atomic restore strategy) — read `research.md` before implementing.

### What this WP produces

```
src/core/admin/
├── retry.go         # RetryFailed function
├── backup.go        # Backup function using SQLite Online Backup API
├── restore.go       # Restore function with integrity check + WAL/SHM cleanup
└── admin_test.go    # Integration tests (backup/restore round-trip, integrity failure, missing backup)

cmd/cli/admin.go     # thin dispatcher (parsing + invocation) — WP02 already created this, only modify if needed
```

---

## Guidance per Subtask

### T020 — Create `src/core/admin/retry.go`

**Purpose**: Re-queue failed and exhausted async tasks.

**File**: `src/core/admin/retry.go` (new file)

**Signature**:
```go
func RetryFailed(ctx context.Context, db *sql.DB) (*RetryFailedResult, error)
```

**Where**:
```go
type RetryFailedResult struct {
    Requeued int64 `json:"requeued"`
    Pending  int64 `json:"pending"`
    Failed   int64 `json:"failed"`
}
```

**SQL** (ported from justfile `retry-failed` target — find the exact SQL):
```sql
UPDATE async_tasks
SET state = 'pending',
    attempt_count = 0,
    last_error = NULL
WHERE state IN ('failed', 'exhausted');
```

**Logic**:
1. Execute the UPDATE, capture `RowsAffected` → `Requeued`
2. Query post-state counts: `SELECT COUNT(*) FROM async_tasks WHERE state='pending'` and `WHERE state='failed'`
3. Return result struct

**Error handling**: Wrap errors with `fmt.Errorf("admin retry: %w", err)`.

**No new deps** — uses `database/sql` only.

---

### T021 — Create `src/core/admin/backup.go`

**Purpose**: WAL-safe atomic backup using SQLite Online Backup API (R2 from research.md).

**File**: `src/core/admin/backup.go` (new file)

**Signature**:
```go
func Backup(ctx context.Context, db *sql.DB, destPath string) (*BackupResult, error)
```

**Where**:
```go
type BackupResult struct {
    BackupPath string `json:"backup_path"`
    SizeBytes  int64  `json:"size_bytes"`
}
```

**SQLite Online Backup API** (R2 — verified available in `mattn/go-sqlite3 v1.14.42`):

```go
import "github.com/mattn/go-sqlite3"

// Get *sqlite3.SQLiteConn from *sql.DB
var srcConn *sqlite3.SQLiteConn
err := db.Conn(ctx).Raw(func(driverConn interface{}) error {
    srcConn = driverConn.(*sqlite3.SQLiteConn)
    return nil
})

// Open dest file
destDB, err := sqlite3.Open(destPath)
defer destDB.Close()

var destConn *sqlite3.SQLiteConn
err = destDB.Conn(ctx).Raw(func(driverConn interface{}) error {
    destConn = driverConn.(*sqlite3.SQLiteConn)
    return nil
})

// Run backup
backup, err := srcConn.Backup("main", destConn, "main")
defer backup.Finish()
for {
    finished, err := backup.Step(false)  // false = not done
    if err != nil {
        return nil, fmt.Errorf("backup step: %w", err)
    }
    if finished {
        break
    }
}
err = backup.Finish()
if err != nil {
    return nil, fmt.Errorf("backup finish: %w", err)
}
```

**Output**: Return `BackupPath` (the `destPath` argument) and `SizeBytes` from `os.Stat(destPath).Size()`.

**Error if dest exists**: The backup operation must NOT overwrite an existing file. Use `os.OpenFile(destPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)` — if the file already exists, this fails with `os.ErrExist`. Catch that and return a clear error.

**Failure cleanup**: If `backup.Step` fails mid-copy, the partial dest file must be removed. Use `defer func() { if err != nil { os.Remove(destPath) } }()`.

**Build tags**: `//go:build sqlite_fts5 sqlite_vec` — requires SQLite driver with backup API.

---

### T022 — Create `src/core/admin/restore.go`

**Purpose**: Atomic restore with integrity check and WAL/SHM cleanup (R3 from research.md).

**File**: `src/core/admin/restore.go` (new file)

**Signature**:
```go
func Restore(ctx context.Context, db *sql.DB, srcPath string) (*RestoreResult, error)
```

**Where**:
```go
type RestoreResult struct {
    From        string `json:"from"`
    IntegrityOK bool   `json:"integrity_ok"`
    DBPath      string `json:"db_path"`
}
```

**Restore lifecycle** (from data-model.md section 6.2):

1. **Resolve source**: `srcPath` is already resolved by CLI (`--from` or `--latest`). Verify it exists.

2. **Verify integrity**:
   ```go
   // Open src DB read-only
   tmpDB, err := sql.Open("sqlite3", srcPath+"?mode=ro")
   defer tmpDB.Close()
   var ok string
   err = tmpDB.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&ok)
   if err != nil || ok != "ok" {
       return nil, fmt.Errorf("restore: integrity check failed for %s", srcPath)
   }
   ```
   If integrity fails, exit with clear error — do NOT proceed.

3. **Copy to temp file in target directory**:
   ```go
   targetPath := cfg.DBPath  // from bootstrap config
   tmpPath := targetPath + ".restore.tmp"
   srcFile, err := os.Open(srcPath)
   defer srcFile.Close()
   dstFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
   defer dstFile.Close()
   _, err = io.Copy(dstFile, srcFile)
   if err != nil {
       os.Remove(tmpPath)
       return nil, fmt.Errorf("restore: copy failed: %w", err)
   }
   dstFile.Close()
   ```

4. **Clean WAL/SHM sidecars of target**:
   ```go
   os.Remove(targetPath + "-wal")  // ignore os.ErrNotExist
   os.Remove(targetPath + "-shm")
   os.Remove(targetPath + "-journal")  // also clean journal
   ```

5. **Atomic rename**:
   ```go
   err = os.Rename(tmpPath, targetPath)
   if err != nil {
       os.Remove(tmpPath)
       return nil, fmt.Errorf("restore: rename failed: %w", err)
   }
   ```

6. **Return result**: `From: srcPath, IntegrityOK: true, DBPath: targetPath`

**Error paths** (FR-036):
- No backup found (srcPath doesn't exist): `fmt.Errorf("restore: backup not found: %s", srcPath)` → exit 2
- Integrity check fails: `fmt.Errorf("restore: integrity check failed for %s", srcPath)` → exit 2
- Rename fails (cross-device): propagate error with context

**Build tags**: `//go:build sqlite_fts5 sqlite_vec`

---

### T023 — Write `src/core/admin/admin_test.go`

**Purpose**: Integration tests for backup/restore round-trip, integrity failure, and missing backup.

**File**: `src/core/admin/admin_test.go` (new file)

**Pattern**: Integration tests against temporary DBs (same pattern as `tests/modules/`).

**Build tags**: `//go:build sqlite_fts5 sqlite_vec` (skips in CI when extensions unavailable — match existing repo convention).

**Tests to write**:

1. **Backup creates file**:
   ```go
   result, err := Backup(ctx, db, tempDir+"/test.db")
   assert.NoError(t, err)
   assert.FileExists(t, result.BackupPath)
   assert.Greater(t, result.SizeBytes, int64(0))
   ```

2. **Restore round-trip preserves data**:
   - Insert known data into DB
   - Run `Backup` to create backup
   - Modify original DB (add more data)
   - Run `Restore` from backup
   - Verify restored DB has original data only (not the additional data)

3. **Integrity failure**:
   - Create a corrupt `.db` file (write garbage)
   - Call `Restore` with corrupt path
   - Verify error is returned, live DB is untouched

4. **Missing backup**:
   - Call `Restore` with non-existent path
   - Verify error returned with clear message

5. **Backup refuses overwrite**:
   - Create a file at dest path
   - Call `Backup` with same dest path
   - Verify error (file not overwritten)

---

## Branch Strategy

- **Planning branch**: `main`
- **Final merge target**: `main`
- **Execution worktrees**: Allocated per computed lane from `lanes.json`.

---

## Definition of Done

- [ ] `src/core/admin/retry.go` — `RetryFailed` works, returns correct counts
- [ ] `src/core/admin/backup.go` — `Backup` creates file, uses Online Backup API, refuses overwrite
- [ ] `src/core/admin/restore.go` — `Restore` verifies integrity, cleans sidecars, atomic rename, refuses corrupt backup
- [ ] `src/core/admin/admin_test.go` — all 5 test cases pass
- [ ] `go test ./src/core/admin/... -tags sqlite_fts5 sqlite_vec` passes

---

## Risks & Reviewer Guidance

**Risk — Backup API surface**: Verify the exact `*sqlite3.SQLiteConn` acquisition pattern. The `db.Conn(ctx).Raw(func(driverConn interface{}) error {...})` pattern must be used to get the raw `*sqlite3.SQLiteConn`. Test that backup actually works with a live DB.

**Risk — Restore atomicity**: If the `os.Rename` fails after the temp file is created, the temp file must be cleaned up. The `defer os.Remove(tmpPath)` must be active from creation through rename.

**Risk — Cross-device rename**: On POSIX, `os.Rename` is atomic within the same filesystem. Since we create the temp in the same dir as the target, this is guaranteed atomic. But if somehow the rename fails, the error message should be clear.

**Reviewer**: After WP04, run `go test ./src/core/admin/... -v` and verify each test case. Check that corrupt backup is rejected (exit 2) and missing backup is rejected (exit 2).

## Activity Log

- 2026-04-26T18:13:16Z – gemini:o3:implementer:implementer – shell_pid=2104662 – Started implementation via action command
- 2026-04-26T18:16:59Z – gemini:o3:implementer:implementer – shell_pid=2104662 – Implemented admin operations package with retry, backup, and restore.
- 2026-04-26T18:17:04Z – gemini:o3:reviewer:reviewer – shell_pid=2109447 – Started review via action command
- 2026-04-26T18:17:13Z – gemini:o3:reviewer:reviewer – shell_pid=2109447 – Review passed: Admin operations (retry, backup, restore) are correctly implemented and verified with tests.
