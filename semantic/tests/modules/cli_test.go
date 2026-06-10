package modules

import (
	"bytes"
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
	// We need to find the root of the project to run go build
	cmd := exec.Command("go", "build", "-tags", "sqlite_fts5 sqlite_vec", "-o", cliPath, "./cmd/cli")
	// The test is running in tests/modules, so we need to go up two levels
	cmd.Dir = "../.." 
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build hsme-cli: %v\nOutput: %s", err, out)
	}

	runCLI := func(args ...string) (string, string, error) {
		// Prepend --db flag
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
		c := exec.Command(cliPath, "--db", dbPath, "store", "--source-type", "note")
		c.Stdin = strings.NewReader(input)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("hsme-cli store failed: %v\nOutput: %s", err, out)
		}
	})

	// 4. Status
	t.Run("Status", func(t *testing.T) {
		stdout, stderr, err := runCLI("status")
		if err != nil {
			t.Fatalf("hsme-cli status failed: %v\nStderr: %s", err, stderr)
		}
		if !strings.Contains(stdout, "Worker Status") {
			t.Errorf("status output missing 'Worker Status', got: %s", stdout)
		}
	})

	// 5. Search Exact
	t.Run("SearchExact", func(t *testing.T) {
		stdout, stderr, err := runCLI("search-exact", "integration")
		if err != nil {
			t.Fatalf("hsme-cli search-exact failed: %v\nStderr: %s", err, stderr)
		}
		if !strings.Contains(stdout, "integration") {
			t.Errorf("search-exact output missing 'integration', got: %s", stdout)
		}
	})

	// 6. Search Fuzzy (JSON)
	t.Run("SearchFuzzyJSON", func(t *testing.T) {
		// Note: Fuzzy search requires vectorization. Since we don't have a worker running,
		// it might return 0 semantic results, but it should still work (lexical part).
		stdout, stderr, err := runCLI("--format", "json", "search-fuzzy", "test")
		if err != nil {
			t.Fatalf("hsme-cli search-fuzzy failed: %v\nStderr: %s", err, stderr)
		}
		if !strings.Contains(stdout, `"results"`) {
			t.Errorf("search-fuzzy JSON output missing 'results', got: %s", stdout)
		}
	})
    
    // 7. Help
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
