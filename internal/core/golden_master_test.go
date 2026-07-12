package core

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Black-box golden-master harness for the compiled quorum binary.
//
// This is the Go port of the former tests/golden_master.py + tests/test_golden_master.py.
// It deliberately treats the binary as an opaque CLI contract: it builds ./quorum, invokes
// it via os/exec, and inspects only the observable surface (stdout, stderr, exit code). It
// shares no implementation code with the binary, which is the whole point of a black-box
// oracle — it cannot inherit the same bug as the code it verifies.
//
// The core assertion is determinism: running the same scenarios twice against the same state
// must produce byte-identical normalized output. This catches Go-specific non-determinism
// (e.g. randomized map iteration order leaking into serialized output).

var (
	reGMTimestamp = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})?`)
	reGMGitSHA    = regexp.MustCompile(`\b[0-9a-f]{40}\b`)
	reGMShortSHA  = regexp.MustCompile(`\b[0-9a-f]{7}\b`)
	reGMUUID      = regexp.MustCompile(`\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
)

// normalizeGolden strips volatile fields so two runs can be compared byte-for-byte.
// Order matters: 40-char SHAs are normalized before 7-char SHAs so a full SHA is not
// partially consumed by the short-SHA pattern.
func normalizeGolden(text, repoRoot string) string {
	if text == "" {
		return text
	}
	text = strings.ReplaceAll(text, repoRoot, "<REPO_ROOT>")
	text = reGMTimestamp.ReplaceAllString(text, "<TIMESTAMP>")
	text = reGMGitSHA.ReplaceAllString(text, "<GIT_SHA>")
	text = reGMShortSHA.ReplaceAllString(text, "<GIT_SHORT_SHA>")
	text = reGMUUID.ReplaceAllString(text, "<UUID>")
	return text
}

type goldenCapture struct {
	command  string
	exitCode int
	stdout   string
	stderr   string
}

// captureGolden runs the binary from repoRoot with deterministic terminal env and returns
// the normalized observable surface. A non-zero exit is a valid capture, not a test failure;
// only a failure to launch the process aborts the test.
func captureGolden(t *testing.T, repoRoot, bin string, args ...string) goldenCapture {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "TERM=dumb", "NO_COLOR=1")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	exitCode := 0
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to launch %q: %v", strings.Join(args, " "), err)
		}
	}

	return goldenCapture{
		command:  strings.Join(args, " "),
		exitCode: exitCode,
		stdout:   normalizeGolden(stdout.String(), repoRoot),
		stderr:   normalizeGolden(stderr.String(), repoRoot),
	}
}

func captureGoldenWithStdin(t *testing.T, repoRoot, bin, stdin string, args ...string) goldenCapture {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "TERM=dumb", "NO_COLOR=1")
	cmd.Stdin = strings.NewReader(stdin)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	exitCode := 0
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to launch %q: %v", strings.Join(args, " "), err)
		}
	}

	return goldenCapture{
		command:  strings.Join(args, " "),
		exitCode: exitCode,
		stdout:   normalizeGolden(stdout.String(), repoRoot),
		stderr:   normalizeGolden(stderr.String(), repoRoot),
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// generateGoldenCorpus exercises the binary's CLI contract across the same three scenarios
// the original Python harness covered. Each scenario provisions its own task dirs and removes
// them afterward, so the corpus can be regenerated against an identical starting state.
func generateGoldenCorpus(t *testing.T, repoRoot, bin string) map[string]goldenCapture {
	t.Helper()
	inbox := filepath.Join(repoRoot, ".ai", "tasks", "inbox")
	active := filepath.Join(repoRoot, ".ai", "tasks", "active")
	corpus := map[string]goldenCapture{}

	// 1. Normal task list.
	corpus["task_list"] = captureGolden(t, repoRoot, bin, "task", "list")

	// 2. Invalid spec triggers a validation error (field=$.path; reason=...).
	brokenDir := filepath.Join(inbox, "BROKEN-999")
	writeFile(t, filepath.Join(brokenDir, "00-spec.yaml"), "task_id: BROKEN-999\nsummary: broken\n")
	corpus["validation_error"] = captureGolden(t, repoRoot, bin, "task", "blueprint", "BROKEN-999")
	_ = os.RemoveAll(brokenDir)
	_ = os.RemoveAll(filepath.Join(active, "BROKEN-999"))

	// 3. Task ID resolution with a sibling whose name shares the prefix.
	ambig1 := filepath.Join(inbox, "AMBIG-100")
	ambig2 := filepath.Join(inbox, "AMBIG-1000")
	writeFile(t, filepath.Join(ambig1, "00-spec.yaml"), "task_id: AMBIG-100\n")
	writeFile(t, filepath.Join(ambig2, "00-spec.yaml"), "task_id: AMBIG-1000\n")
	corpus["ambiguous_resolution"] = captureGolden(t, repoRoot, bin, "task", "status", "AMBIG-100")
	_ = os.RemoveAll(ambig1)
	_ = os.RemoveAll(ambig2)

	// 4. Analyze commands reject positional arguments.
	for _, cmd := range []string{"acceptance-coverage", "blueprint-context", "decomposition-coverage", "decomposition-render", "failure-lookup", "feedback-partition", "risk-score", "complexity-score", "contract-check", "fleet-preflight"} {
		corpus["analyze_no_args_"+cmd] = captureGolden(t, repoRoot, bin, "analyze", cmd, "spurious-arg")
	}

	return corpus
}

func TestAnalyzeCommandsRejectPositionalArgsBeforeStdin(t *testing.T) {
	bin := buildQuorumCLI(t)
	root := initGitRepo(t)

	for _, analyzeCmd := range []string{
		"acceptance-coverage",
		"blueprint-context",
		"decomposition-coverage",
		"decomposition-render",
		"failure-lookup",
		"feedback-partition",
		"risk-score",
		"complexity-score",
		"contract-check",
		"fleet-preflight",
	} {
		t.Run(analyzeCmd, func(t *testing.T) {
			got := captureGolden(t, root, bin, "analyze", analyzeCmd, "spurious-arg")
			if got.exitCode == 0 {
				t.Fatalf("%s accepted a positional argument", got.command)
			}
			if !strings.Contains(got.stderr, "accepts 0 arg(s), received 1") {
				t.Fatalf("%s did not report the no-args contract\nstderr:\n%s", got.command, got.stderr)
			}
			if strings.Contains(got.stderr, "empty stdin") {
				t.Fatalf("%s read stdin before rejecting positional args\nstderr:\n%s", got.command, got.stderr)
			}
		})
	}
}

func TestAnalyzeValidStdinInvocationStillWorks(t *testing.T) {
	bin := buildQuorumCLI(t)
	root := initGitRepo(t)

	got := captureGoldenWithStdin(
		t,
		root,
		bin,
		`{"decomposition":[{"child_id":"FEAT-001-a","depends_on":[]}]}`,
		"analyze",
		"decomposition-render",
	)
	if got.exitCode != 0 {
		t.Fatalf("%s failed with valid stdin\nstderr:\n%s", got.command, got.stderr)
	}
	if !strings.Contains(got.stdout, "Decomposition DAG:") {
		t.Fatalf("%s did not render expected DAG\nstdout:\n%s", got.command, got.stdout)
	}
}

// TestGoldenCorpusStable asserts the binary's observable output is deterministic: two
// independent generations of the same corpus must be byte-identical after normalization.
func TestGoldenCorpusStable(t *testing.T) {
	useSchemas(t)
	bin := buildQuorumCLI(t)
	root := initGitRepo(t)
	ensureTaskDirs(t, root)

	first := generateGoldenCorpus(t, root, bin)
	second := generateGoldenCorpus(t, root, bin)

	if len(first) != len(second) {
		t.Fatalf("corpus structure is not deterministic: %d scenarios vs %d", len(first), len(second))
	}

	for name, a := range first {
		b, ok := second[name]
		if !ok {
			t.Fatalf("scenario %q present in first run but missing in second", name)
		}
		if a.exitCode != b.exitCode {
			t.Errorf("scenario %q: exit code not deterministic: %d vs %d", name, a.exitCode, b.exitCode)
		}
		if a.stdout != b.stdout {
			t.Errorf("scenario %q: stdout not deterministic\n--- first ---\n%s\n--- second ---\n%s", name, a.stdout, b.stdout)
		}
		if a.stderr != b.stderr {
			t.Errorf("scenario %q: stderr not deterministic\n--- first ---\n%s\n--- second ---\n%s", name, a.stderr, b.stderr)
		}
	}
}
