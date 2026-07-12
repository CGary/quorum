package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalyzeFeedbackPartition(t *testing.T) {
	bin := buildAnalyzeCLI(t)
	dir := t.TempDir()

	t.Run("ValidStdin", func(t *testing.T) {
		stdin := `{"findings": [{"category": "mechanical", "id": 1}, {"category": "semantic", "id": 2}]}`
		out, err := runAnalyzeCmdErr(t, dir, bin, stdin, "analyze", "feedback-partition")
		if err != nil {
			t.Fatalf("expected success, got err=%v out=%q", err, out)
		}

		var result struct {
			Mechanical []map[string]any `json:"mechanical"`
			Semantic   []map[string]any `json:"semantic"`
		}
		if err := json.Unmarshal([]byte(out), &result); err != nil {
			t.Fatalf("failed to unmarshal stdout: %v, out=%q", err, out)
		}

		if len(result.Mechanical) != 1 || result.Mechanical[0]["id"] != float64(1) {
			t.Errorf("unexpected mechanical partition: %v", result.Mechanical)
		}
		if len(result.Semantic) != 1 || result.Semantic[0]["id"] != float64(2) {
			t.Errorf("unexpected semantic partition: %v", result.Semantic)
		}
	})

	t.Run("EmptyStdin", func(t *testing.T) {
		out, err := runAnalyzeCmdErr(t, dir, bin, "", "analyze", "feedback-partition")
		if err == nil {
			t.Fatal("expected error for empty stdin, got success")
		}
		if !strings.Contains(out, "empty stdin") {
			t.Errorf("expected 'empty stdin' in output, got %q", out)
		}
	})

	t.Run("MalformedJSON", func(t *testing.T) {
		out, err := runAnalyzeCmdErr(t, dir, bin, `{"findings": [`, "analyze", "feedback-partition")
		if err == nil {
			t.Fatal("expected error for malformed JSON, got success")
		}
		if !strings.Contains(out, "unmarshal") {
			t.Errorf("expected unmarshal error in output, got %q", out)
		}
	})
}

func buildAnalyzeCLI(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(cwd)
	bin := filepath.Join(t.TempDir(), "quorum")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}
	return bin
}

func runAnalyzeCmdErr(t *testing.T, dir, bin, stdin string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	// Always set Stdin so empty string translates to an empty reader (EOF immediately),
	// rather than blocking or reading from the test runner's os.Stdin.
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
