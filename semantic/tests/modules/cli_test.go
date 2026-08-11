package modules

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLI_EndToEnd(t *testing.T) {
	// 1. Setup
	tmpDir, err := os.MkdirTemp("", "hsme-cli-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	cliPath := filepath.Join(tmpDir, "hsme-cli")

	// 2. Build the CLI binary
	cmd := exec.Command("go", "build", "-tags", "sqlite_fts5 sqlite_vec", "-o", cliPath, "./cmd/cli")
	cmd.Dir = "../.."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build hsme-cli: %v\nOutput: %s", err, out)
	}

	runCLI := func(args ...string) (string, string, error) {
		fullArgs := append([]string{"--db", dbPath}, args...)
		c := exec.Command(cliPath, fullArgs...)
		var stdout, stderr bytes.Buffer
		c.Stdout = &stdout
		c.Stderr = &stderr
		err := c.Run()
		return stdout.String(), stderr.String(), err
	}

	// 3. Store data
	t.Run("Store", func(t *testing.T) {
		input := "This is a test memory for the CLI integration test."
		c := exec.Command(cliPath, "--db", dbPath, "store", "--source-type", "note", "--json")
		c.Stdin = strings.NewReader(input)
		var stdout, stderr bytes.Buffer
		c.Stdout = &stdout
		c.Stderr = &stderr
		if err := c.Run(); err != nil {
			t.Fatalf("hsme-cli store failed: %v\nStderr: %s\nStdout: %s", err, stderr.String(), stdout.String())
		}

		var res map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
			t.Fatalf("failed to parse store JSON: %v\nOutput: %s", err, stdout.String())
		}
		if res["ok"] != true {
			t.Errorf("expected ok: true, got %v", res["ok"])
		}
		if res["command"] != "store" {
			t.Errorf("expected command 'store', got %v", res["command"])
		}
		data, ok := res["data"].(map[string]any)
		if !ok || data["status"] != "stored" {
			t.Errorf("unexpected data payload: %v", res["data"])
		}
	})

	// 4. Store Dry-Run
	t.Run("StoreDryRun", func(t *testing.T) {
		input := "Dry run test content."
		c := exec.Command(cliPath, "--db", dbPath, "store", "--source-type", "note", "--dry-run", "--json")
		c.Stdin = strings.NewReader(input)
		var stdout, stderr bytes.Buffer
		c.Stdout = &stdout
		c.Stderr = &stderr
		if err := c.Run(); err != nil {
			t.Fatalf("hsme-cli store --dry-run failed: %v\nStderr: %s\nStdout: %s", err, stderr.String(), stdout.String())
		}

		var res map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
			t.Fatalf("failed to parse store dry-run JSON: %v", err)
		}
		if res["ok"] != true {
			t.Errorf("expected ok: true, got %v", res["ok"])
		}
		data := res["data"].(map[string]any)
		if data["dry_run"] != true {
			t.Errorf("expected data.dry_run: true, got %v", data["dry_run"])
		}
		if data["content_bytes"].(float64) != float64(len(input)) {
			t.Errorf("expected content_bytes %d, got %v", len(input), data["content_bytes"])
		}
	})

	// 5. Store Missing Source Type Error
	t.Run("StoreMissingSourceTypeError", func(t *testing.T) {
		c := exec.Command(cliPath, "--db", dbPath, "store", "--json")
		c.Stdin = strings.NewReader("some content")
		var stdout, stderr bytes.Buffer
		c.Stdout = &stdout
		c.Stderr = &stderr
		err := c.Run()
		if err == nil {
			t.Errorf("expected error when --source-type is missing")
		}
		var res map[string]any
		if jsonErr := json.Unmarshal(stdout.Bytes(), &res); jsonErr != nil {
			t.Fatalf("failed to parse JSON error: %v, stdout: %s", jsonErr, stdout.String())
		}
		if res["ok"] != false {
			t.Errorf("expected ok: false, got %v", res["ok"])
		}
		errObj := res["error"].(map[string]any)
		if errObj["code"] != "MISSING_REQUIRED_FLAG" {
			t.Errorf("expected MISSING_REQUIRED_FLAG, got %v", errObj["code"])
		}
	})

	// 6. Status
	t.Run("Status", func(t *testing.T) {
		stdout, stderr, err := runCLI("status", "--json")
		if err != nil {
			t.Fatalf("hsme-cli status failed: %v\nStderr: %s", err, stderr)
		}
		var res map[string]any
		if err := json.Unmarshal([]byte(stdout), &res); err != nil {
			t.Fatalf("failed to parse status JSON: %v\nOutput: %s", err, stdout)
		}
		if res["ok"] != true {
			t.Errorf("expected ok: true, got %v", res["ok"])
		}
		if res["command"] != "status" {
			t.Errorf("expected command 'status', got %v", res["command"])
		}
		data := res["data"].(map[string]any)
		if data["memories"].(float64) < 1 {
			t.Errorf("expected memories >= 1, got %v", data["memories"])
		}
	})

	t.Run("StatusHuman", func(t *testing.T) {
		stdout, stderr, err := runCLI("status")
		if err != nil {
			t.Fatalf("hsme-cli status failed: %v\nStderr: %s", err, stderr)
		}
		if !strings.Contains(stdout, "Worker Status") {
			t.Errorf("status output missing 'Worker Status', got: %s", stdout)
		}
	})

	// 7. Search Exact
	t.Run("SearchExact", func(t *testing.T) {
		stdout, stderr, err := runCLI("search-exact", "integration", "--json")
		if err != nil {
			t.Fatalf("hsme-cli search-exact failed: %v\nStderr: %s", err, stderr)
		}
		var res map[string]any
		if err := json.Unmarshal([]byte(stdout), &res); err != nil {
			t.Fatalf("failed to parse search-exact JSON: %v\nOutput: %s", err, stdout)
		}
		if res["ok"] != true {
			t.Errorf("expected ok: true, got %v", res["ok"])
		}
		if res["command"] != "search-exact" {
			t.Errorf("expected command 'search-exact', got %v", res["command"])
		}
		data := res["data"].(map[string]any)
		results := data["results"].([]any)
		if len(results) == 0 {
			t.Errorf("expected search results > 0")
		}
	})

	// 8. Search Exact with --fields
	t.Run("SearchExactFields", func(t *testing.T) {
		stdout, stderr, err := runCLI("search-exact", "integration", "--fields", "memory_id", "--json")
		if err != nil {
			t.Fatalf("hsme-cli search-exact with fields failed: %v\nStderr: %s", err, stderr)
		}
		var res map[string]any
		if err := json.Unmarshal([]byte(stdout), &res); err != nil {
			t.Fatalf("failed to parse JSON: %v", err)
		}
		data := res["data"].(map[string]any)
		results := data["results"].([]any)
		if len(results) == 0 {
			t.Fatalf("expected results > 0")
		}
		first := results[0].(map[string]any)
		if _, ok := first["memory_id"]; !ok {
			t.Errorf("expected memory_id field in projected result")
		}
		if _, ok := first["chunk_id"]; ok {
			t.Errorf("did not expect chunk_id field in projected result")
		}
	})

	// 9. Search Fuzzy (JSON)
	t.Run("SearchFuzzyJSON", func(t *testing.T) {
		stdout, stderr, err := runCLI("search-fuzzy", "test", "--json")
		if err != nil {
			t.Fatalf("hsme-cli search-fuzzy failed: %v\nStderr: %s", err, stderr)
		}
		var res map[string]any
		if err := json.Unmarshal([]byte(stdout), &res); err != nil {
			t.Fatalf("failed to parse search-fuzzy JSON: %v\nOutput: %s", err, stdout)
		}
		if res["ok"] != true {
			t.Errorf("expected ok: true, got %v", res["ok"])
		}
		if res["command"] != "search-fuzzy" {
			t.Errorf("expected command 'search-fuzzy', got %v", res["command"])
		}
	})

	// 10. Explore
	t.Run("Explore", func(t *testing.T) {
		stdout, stderr, err := runCLI("explore", "integration", "--json")
		if err != nil {
			t.Fatalf("hsme-cli explore failed: %v\nStderr: %s", err, stderr)
		}
		var res map[string]any
		if err := json.Unmarshal([]byte(stdout), &res); err != nil {
			t.Fatalf("failed to parse explore JSON: %v\nOutput: %s", err, stdout)
		}
		if res["ok"] != true {
			t.Errorf("expected ok: true, got %v", res["ok"])
		}
		if res["command"] != "explore" {
			t.Errorf("expected command 'explore', got %v", res["command"])
		}
	})

	// 11. Explore Invalid Direction Enum Error
	t.Run("ExploreInvalidDirectionEnum", func(t *testing.T) {
		stdout, _, err := runCLI("explore", "integration", "--direction", "sideways", "--json")
		if err == nil {
			t.Errorf("expected non-zero exit for invalid direction")
		}
		var res map[string]any
		if jsonErr := json.Unmarshal([]byte(stdout), &res); jsonErr != nil {
			t.Fatalf("failed to parse error JSON: %v, stdout: %s", jsonErr, stdout)
		}
		if res["ok"] != false {
			t.Errorf("expected ok: false, got %v", res["ok"])
		}
		errObj := res["error"].(map[string]any)
		if errObj["code"] != "INVALID_ENUM" {
			t.Errorf("expected code INVALID_ENUM, got %v", errObj["code"])
		}
		if errObj["field"] != "direction" {
			t.Errorf("expected field direction, got %v", errObj["field"])
		}
	})

	// 12. Admin Subcommands
	t.Run("AdminRetryFailed", func(t *testing.T) {
		stdout, stderr, err := runCLI("admin", "retry-failed", "--json")
		if err != nil {
			t.Fatalf("admin retry-failed failed: %v\nStderr: %s", err, stderr)
		}
		var res map[string]any
		if err := json.Unmarshal([]byte(stdout), &res); err != nil {
			t.Fatalf("failed to parse admin retry-failed JSON: %v", err)
		}
		if res["ok"] != true {
			t.Errorf("expected ok: true, got %v", res["ok"])
		}
		if res["command"] != "admin.retry-failed" {
			t.Errorf("expected command 'admin.retry-failed', got %v", res["command"])
		}
	})

	backupPath := filepath.Join(tmpDir, "backup.db")
	t.Run("AdminBackupAndRestoreDryRun", func(t *testing.T) {
		stdout, stderr, err := runCLI("admin", "backup", "--dest", backupPath, "--json")
		if err != nil {
			t.Fatalf("admin backup failed: %v\nStderr: %s", err, stderr)
		}
		var res map[string]any
		if err := json.Unmarshal([]byte(stdout), &res); err != nil {
			t.Fatalf("failed to parse backup JSON: %v", err)
		}
		if res["ok"] != true {
			t.Errorf("expected ok: true, got %v", res["ok"])
		}
		if res["command"] != "admin.backup" {
			t.Errorf("expected command 'admin.backup', got %v", res["command"])
		}

		// Restore dry-run
		rStdout, rStderr, rErr := runCLI("admin", "restore", "--from", backupPath, "--dry-run", "--json")
		if rErr != nil {
			t.Fatalf("admin restore --dry-run failed: %v\nStderr: %s", rErr, rStderr)
		}
		var rRes map[string]any
		if err := json.Unmarshal([]byte(rStdout), &rRes); err != nil {
			t.Fatalf("failed to parse restore dry-run JSON: %v", err)
		}
		if rRes["ok"] != true {
			t.Errorf("expected ok: true, got %v", rRes["ok"])
		}
		if rRes["command"] != "admin.restore" {
			t.Errorf("expected command 'admin.restore', got %v", rRes["command"])
		}
		data := rRes["data"].(map[string]any)
		if data["dry_run"] != true {
			t.Errorf("expected data.dry_run: true, got %v", data["dry_run"])
		}
	})

	// 13. Output redirection flag
	t.Run("OutputFlag", func(t *testing.T) {
		outFile := filepath.Join(tmpDir, "search_results.json")
		stdout, stderr, err := runCLI("search-exact", "integration", "--output", outFile, "--json")
		if err != nil {
			t.Fatalf("search-exact with --output failed: %v\nStderr: %s", err, stderr)
		}
		var res map[string]any
		if err := json.Unmarshal([]byte(stdout), &res); err != nil {
			t.Fatalf("failed to parse output envelope: %v", err)
		}
		data := res["data"].(map[string]any)
		if data["result_file"] != outFile {
			t.Errorf("got result_file %v, want %v", data["result_file"], outFile)
		}
		if _, err := os.Stat(outFile); err != nil {
			t.Errorf("output file was not created: %v", err)
		}
	})

	// 14. Version flag
	t.Run("VersionFlag", func(t *testing.T) {
		c := exec.Command(cliPath, "--version")
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("--version failed: %v", err)
		}
		if !strings.Contains(string(out), "hsme-cli 2.0.0") {
			t.Errorf("expected version output, got: %s", out)
		}

		cJSON := exec.Command(cliPath, "--version", "--json")
		outJSON, err := cJSON.CombinedOutput()
		if err != nil {
			t.Fatalf("--version --json failed: %v", err)
		}
		var res map[string]any
		if err := json.Unmarshal(outJSON, &res); err != nil {
			t.Fatalf("failed to parse version JSON: %v", err)
		}
		if res["ok"] != true || res["command"] != "version" {
			t.Errorf("unexpected version JSON envelope: %v", res)
		}
	})

	// 15. Schema flags
	t.Run("SchemaIntrospection", func(t *testing.T) {
		subcommands := []string{"store", "search-fuzzy", "search-exact", "explore", "status"}
		for _, sub := range subcommands {
			stdout, _, err := runCLI(sub, "--schema")
			if err != nil {
				t.Fatalf("%s --schema failed: %v", sub, err)
			}
			var s map[string]any
			if err := json.Unmarshal([]byte(stdout), &s); err != nil {
				t.Fatalf("failed to parse schema JSON for %s: %v", sub, err)
			}
			if s["command"] != sub {
				t.Errorf("got schema command %v, want %v", s["command"], sub)
			}
		}
	})

	// 16. Help
	t.Run("Help", func(t *testing.T) {
		stdout, _, err := runCLI("help")
		if err != nil {
			t.Fatalf("hsme-cli help failed: %v", err)
		}
		if !strings.Contains(stdout, "Subcommands:") {
			t.Errorf("help output missing 'Subcommands:', got: %s", stdout)
		}
	})
}
