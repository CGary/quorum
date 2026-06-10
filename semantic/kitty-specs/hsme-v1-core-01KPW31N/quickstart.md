# Quickstart: HSME V1 Core

1. **Build the server**: Compile with CGO enabled and the `sqlite_load_extension` build tag.
   ```bash
   go build -tags="sqlite_load_extension" -o hsme ./cmd/server
   ```
2. **Dependencies**: Ensure `vec0.so` is available in your system path.
3. **Run**:
   ```bash
   export SQLITE_DB_PATH=/app/data/engram.db
   export SQLITE_VEC_PATH=/usr/local/lib/vec0.so
   ./hsme
   ```
4. **Test strategy**: Create business block/module tests evaluating the deduplication logic, RRF scoring, and worker leasing behavior before implementing the underlying Go code. Tests reside in `tests/modules/`.
