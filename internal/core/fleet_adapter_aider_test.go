package core

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRenderAiderArgvExpandsFilesAndVars(t *testing.T) {
	tmpl := []string{
		"--message-file", "{prompt_file}",
		"--no-auto-commits", "--no-attribute-co-authored-by", "--yes-always",
		"--model", "{model_arg}",
		"{files}",
	}
	vars := map[string]string{"prompt_file": "/tmp/msg.txt", "model_arg": "openrouter/openrouter/free"}
	files := []string{"internal/core/fleet_adapter_aider.go", "cmd/fleet_dispatch.go"}
	got := RenderAiderArgv(tmpl, vars, files)
	want := []string{
		"--message-file", "/tmp/msg.txt",
		"--no-auto-commits", "--no-attribute-co-authored-by", "--yes-always",
		"--model", "openrouter/openrouter/free",
		"internal/core/fleet_adapter_aider.go", "cmd/fleet_dispatch.go",
	}
	if len(got) != len(want) {
		t.Fatalf("argv length mismatch: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("argv[%d] = %q, want %q (full got=%v)", i, got[i], want[i], got)
		}
	}
}

func TestRenderAiderArgvSingleFile(t *testing.T) {
	tmpl := []string{"--message-file", "{prompt_file}", "{files}"}
	got := RenderAiderArgv(tmpl, map[string]string{"prompt_file": "m.txt"}, []string{"only.go"})
	want := []string{"--message-file", "m.txt", "only.go"}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestValidateAiderArgvAcceptsCompleteArgv(t *testing.T) {
	argv := []string{
		"--message-file", "m.txt",
		"--no-auto-commits", "--no-attribute-co-authored-by", "--yes-always",
		"--model", "openrouter/openrouter/free",
		"file.go",
	}
	if err := ValidateAiderArgv(argv); err != nil {
		t.Fatalf("want nil error for complete argv, got %v", err)
	}
}

func TestValidateAiderArgvRejectsMissingMessageFile(t *testing.T) {
	argv := []string{
		"--no-auto-commits", "--no-attribute-co-authored-by", "--yes-always",
		"--model", "openrouter/openrouter/free",
		"file.go",
	}
	err := ValidateAiderArgv(argv)
	if err == nil {
		t.Fatal("want error for argv missing --message-file")
	}
	if !strings.Contains(err.Error(), "--message-file") {
		t.Fatalf("error should name the missing flag, got %v", err)
	}
}

func TestValidateAiderArgvRejectsMissingYesAlways(t *testing.T) {
	argv := []string{
		"--message-file", "m.txt",
		"--no-auto-commits", "--no-attribute-co-authored-by",
		"--model", "openrouter/openrouter/free",
		"file.go",
	}
	if err := ValidateAiderArgv(argv); err == nil {
		t.Fatal("want error for argv missing --yes-always")
	}
}

func TestAiderUsageIsAlwaysSourceNone(t *testing.T) {
	u := AiderUsage()
	if u.Source != "none" {
		t.Fatalf("want Source=none, got %q", u.Source)
	}
}

func TestAiderRequiredEnvVarKnownProvider(t *testing.T) {
	envVar, ok := AiderRequiredEnvVar("openrouter/openrouter/free")
	if !ok || envVar != "OPENROUTER_API_KEY" {
		t.Fatalf("got (%q, %v), want (OPENROUTER_API_KEY, true)", envVar, ok)
	}
}

func TestAiderRequiredEnvVarUnknownProvider(t *testing.T) {
	_, ok := AiderRequiredEnvVar("some-unknown-provider/model-x")
	if ok {
		t.Fatal("want ok=false for an unrecognized provider prefix")
	}
}

func TestCheckAiderPreflightFailsWhenKeyAbsent(t *testing.T) {
	os.Unsetenv("OPENROUTER_API_KEY")
	err := CheckAiderPreflight("openrouter/openrouter/free")
	if err == nil {
		t.Fatal("want noisy failure when OPENROUTER_API_KEY is absent")
	}
	if !strings.Contains(err.Error(), "OPENROUTER_API_KEY") {
		t.Fatalf("error should name the missing env var, got %v", err)
	}
}

func TestCheckAiderPreflightPassesWhenKeyPresent(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test-not-real")
	if err := CheckAiderPreflight("openrouter/openrouter/free"); err != nil {
		t.Fatalf("want nil error when key present, got %v", err)
	}
}

func TestCheckAiderPreflightNeverLeaksValueInError(t *testing.T) {
	os.Unsetenv("OPENROUTER_API_KEY")
	err := CheckAiderPreflight("openrouter/openrouter/free")
	if err == nil {
		t.Fatal("expected error")
	}
	// The absent-key error path can never contain a key value (there is none
	// to leak here); this test documents the invariant that CheckAiderPreflight
	// only ever checks presence and the error message names the var, not a value.
	if strings.Contains(err.Error(), "sk-") {
		t.Fatalf("error must never contain a key-shaped value: %v", err)
	}
}

func TestParseAiderReportedCostParsesSessionFigure(t *testing.T) {
	out := "Tokens: 2.3k sent, 456 received.\nCost: $0.0123 message, $0.0456 session.\n"
	cost, ok := ParseAiderReportedCost(out)
	if !ok {
		t.Fatal("want ok=true for a well-formed cost report")
	}
	if cost != 0.0456 {
		t.Fatalf("got cost=%v, want 0.0456", cost)
	}
}

func TestParseAiderReportedCostNoMatch(t *testing.T) {
	_, ok := ParseAiderReportedCost("no cost info here")
	if ok {
		t.Fatal("want ok=false when no session cost figure is present")
	}
}

func TestCostExceedsCeiling(t *testing.T) {
	if !CostExceedsCeiling(0.6, 0.5) {
		t.Fatal("0.6 should exceed a 0.5 ceiling")
	}
	if CostExceedsCeiling(0.5, 0.5) {
		t.Fatal("cost equal to ceiling must not be classified as exceeding it")
	}
	if CostExceedsCeiling(0.1, 0.5) {
		t.Fatal("0.1 should not exceed a 0.5 ceiling")
	}
}

// TestAiderAgentsYamlNeverLeaksAnApiKey is a static AC-8 guard: it reads the
// repo's own .agents/fleet/agents.yaml and asserts no OpenRouter-key-shaped
// value or literal env-value assignment ever appears in it. Keys must come
// only from the process environment, never from a versioned artifact.
func TestAiderAgentsYamlNeverLeaksAnApiKey(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	raw, err := os.ReadFile(filepath.Join(repoRoot, ".agents", "fleet", "agents.yaml"))
	if err != nil {
		t.Fatalf("read agents.yaml: %v", err)
	}
	content := string(raw)
	for _, banned := range []string{"sk-or-", "OPENROUTER_API_KEY:", "OPENROUTER_API_KEY="} {
		if strings.Contains(content, banned) {
			t.Fatalf("agents.yaml must never contain %q (secret leak)", banned)
		}
	}
}
