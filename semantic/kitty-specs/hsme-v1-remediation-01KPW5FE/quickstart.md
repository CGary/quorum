# Quickstart: HSME V1 Remediation

1. **Verify Broken Tests**: The `tests/modules/worker_test.go` file currently fails to compile because the implementation is missing. This is the starting point for remediation.
2. **Build the server**: Compile with CGO enabled and the `sqlite_load_extension` build tag once the `cmd/server/main.go` logic is built.
   ```bash
   go build -tags="sqlite_load_extension" -o hsme ./cmd/server
   ```
3. **Run Tests**:
   ```bash
   go test -tags="sqlite_load_extension" ./tests/modules/...
   ```
