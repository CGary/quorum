package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupFleetRunProject builds a minimal project root with a fake agy-like
// transport and an executable fake binary. It creates NO git repo, NO task
// dir, and NO worktree: 'fleet run' is non-lifecycle, so its dependencies are
// only the transport map and the delegate binary.
func setupFleetRunProject(t *testing.T) (root, marker string) {
	t.Helper()
	// Isolate from any QUORUM_FLEET_AGENTS exported in the invoking shell.
	t.Setenv("QUORUM_FLEET_AGENTS", "")
	root = t.TempDir()
	// A marker path the fake binary touches so a test can prove the delegate
	// (a) ran and (b) ran with cwd = --cwd.
	marker = "ran.txt"
	script := filepath.Join(root, "fake-agent.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'delegate ok\\n'\nprintf 'x' > "+marker+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	agentsDir := filepath.Join(root, ".agents", "fleet")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	agents := "transports:\n" +
		"  fake:\n" +
		"    binary: " + script + "\n" +
		"    argv_template:\n" +
		"      - --model\n" +
		"      - \"{model_arg}\"\n" +
		"      - \"{prompt}\"\n" +
		"    output_format: text\n" +
		"    timeouts:\n" +
		"      default_s: 30\n" +
		"    failure_signatures: []\n" +
		"    active: true\n" +
		"    models:\n" +
		"      anthropic/claude-sonnet-4-6:\n" +
		"        model_arg: Claude Sonnet 4.6 (Thinking)\n"
	if err := os.WriteFile(filepath.Join(agentsDir, "agents.yaml"), []byte(agents), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, marker
}

func writePromptFile(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "prompt.txt")
	if err := os.WriteFile(p, []byte("say hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func decodeEnvelope(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, b)
	}
	return m
}

func TestFleetRunHappyPathInCwd(t *testing.T) {
	root, marker := setupFleetRunProject(t)
	cwd := t.TempDir()
	var out, errW bytes.Buffer
	code := runFleetRun(fleetRunParams{
		Agent: "fake", Model: "anthropic/claude-sonnet-4-6", Cwd: cwd,
		Input: writePromptFile(t, root), JSON: true, ProjectRoot: root,
	}, &out, &errW)
	if code != 0 {
		t.Fatalf("want exit 0, got %d\nstdout=%s\nstderr=%s", code, out.String(), errW.String())
	}
	env := decodeEnvelope(t, out.Bytes())
	if env["ok"] != true || env["command"] != "fleet.run" {
		t.Fatalf("bad success envelope: %v", env)
	}
	if _, err := os.Stat(filepath.Join(cwd, marker)); err != nil {
		t.Fatalf("delegate did not run in --cwd: %v", err)
	}
	// Non-lifecycle: no lifecycle side effects in the project root.
	for _, p := range []string{".ai", "worktrees", ".git"} {
		if _, err := os.Stat(filepath.Join(root, p)); err == nil {
			t.Fatalf("fleet run created lifecycle artifact %s", p)
		}
	}
}

func TestFleetRunUnknownModelInvalidEnum(t *testing.T) {
	root, marker := setupFleetRunProject(t)
	cwd := t.TempDir()
	var out, errW bytes.Buffer
	code := runFleetRun(fleetRunParams{
		Agent: "fake", Model: "anthropic/nope", Cwd: cwd,
		Input: writePromptFile(t, root), JSON: true, ProjectRoot: root,
	}, &out, &errW)
	if code == 0 {
		t.Fatal("unknown model must exit non-zero")
	}
	env := decodeEnvelope(t, out.Bytes())
	e, _ := env["error"].(map[string]any)
	if env["ok"] != false || e["code"] != "INVALID_ENUM" || e["field"] != "model" || e["received"] != "anthropic/nope" {
		t.Fatalf("bad INVALID_ENUM envelope: %v", env)
	}
	if !strings.Contains(e["message"].(string), "anthropic/claude-sonnet-4-6") {
		t.Fatalf("message must list valid models: %v", e["message"])
	}
	if _, err := os.Stat(filepath.Join(cwd, marker)); err == nil {
		t.Fatal("no delegate must run on INVALID_ENUM")
	}
}

func TestFleetRunMissingRequiredFlag(t *testing.T) {
	root, _ := setupFleetRunProject(t)
	var out, errW bytes.Buffer
	code := runFleetRun(fleetRunParams{
		Agent: "fake", Cwd: t.TempDir(), Input: writePromptFile(t, root), JSON: true, ProjectRoot: root,
	}, &out, &errW)
	if code == 0 {
		t.Fatal("missing --model must exit non-zero")
	}
	e, _ := decodeEnvelope(t, out.Bytes())["error"].(map[string]any)
	if e["code"] != "MISSING_REQUIRED_FLAG" || e["field"] != "model" {
		t.Fatalf("bad MISSING_REQUIRED_FLAG envelope: %v", e)
	}
}

func TestFleetRunFileNotFound(t *testing.T) {
	root, _ := setupFleetRunProject(t)
	var out, errW bytes.Buffer
	code := runFleetRun(fleetRunParams{
		Agent: "fake", Model: "anthropic/claude-sonnet-4-6",
		Cwd: filepath.Join(root, "no-such-dir"), Input: writePromptFile(t, root), JSON: true, ProjectRoot: root,
	}, &out, &errW)
	if code == 0 {
		t.Fatal("missing --cwd path must exit non-zero")
	}
	e, _ := decodeEnvelope(t, out.Bytes())["error"].(map[string]any)
	if e["code"] != "FILE_NOT_FOUND" || e["field"] != "cwd" {
		t.Fatalf("bad FILE_NOT_FOUND envelope: %v", e)
	}
}

func TestFleetRunSchema(t *testing.T) {
	root, marker := setupFleetRunProject(t)
	cwd := t.TempDir()
	var out, errW bytes.Buffer
	code := runFleetRun(fleetRunParams{
		Agent: "fake", Schema: true, Cwd: cwd, ProjectRoot: root,
	}, &out, &errW)
	if code != 0 {
		t.Fatalf("--schema must exit 0, got %d", code)
	}
	if !strings.Contains(out.String(), "anthropic/claude-sonnet-4-6") || !strings.Contains(out.String(), "\"input\"") {
		t.Fatalf("--schema must print the model enum and input contract:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(cwd, marker)); err == nil {
		t.Fatal("--schema must not run a delegate")
	}
}

func TestFleetRunTimeout(t *testing.T) {
	t.Setenv("QUORUM_FLEET_AGENTS", "")
	root := t.TempDir()
	script := filepath.Join(root, "slow.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	agentsDir := filepath.Join(root, ".agents", "fleet")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	agents := "transports:\n  fake:\n    binary: " + script + "\n    argv_template: []\n    output_format: text\n    timeouts:\n      default_s: 30\n    failure_signatures: []\n    active: true\n    models:\n      anthropic/claude-sonnet-4-6:\n        model_arg: x\n"
	if err := os.WriteFile(filepath.Join(agentsDir, "agents.yaml"), []byte(agents), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errW bytes.Buffer
	code := runFleetRun(fleetRunParams{
		Agent: "fake", Model: "anthropic/claude-sonnet-4-6", Cwd: t.TempDir(),
		Input: writePromptFile(t, root), TimeoutS: 1, JSON: true, ProjectRoot: root,
	}, &out, &errW)
	if code == 0 {
		t.Fatal("timed-out delegate must exit non-zero")
	}
	e, _ := decodeEnvelope(t, out.Bytes())["error"].(map[string]any)
	if e["code"] != "TIMEOUT" {
		t.Fatalf("bad TIMEOUT envelope: %v", e)
	}
}

// TestFleetRunPrintTimeout is FLEET-019 AC-4 (run half): --dry-run's
// returned argv must carry "--print-timeout" set to the effective timeout
// (--timeout if set, else the transport default), same formatting as dispatch.
func TestFleetRunPrintTimeout(t *testing.T) {
	t.Setenv("QUORUM_FLEET_AGENTS", "")
	root := t.TempDir()
	agentsDir := filepath.Join(root, ".agents", "fleet")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	agents := "transports:\n  agy-fake:\n    binary: /bin/true\n    argv_template:\n      - --model\n      - \"{model_arg}\"\n      - --print-timeout\n      - \"{print_timeout}\"\n      - --print\n      - \"{prompt}\"\n    output_format: text\n    timeouts:\n      default_s: 300\n    failure_signatures: []\n    active: true\n    models:\n      anthropic/claude-sonnet-4-6:\n        model_arg: Claude Sonnet 4.6 (Thinking)\n"
	if err := os.WriteFile(filepath.Join(agentsDir, "agents.yaml"), []byte(agents), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name     string
		timeoutS int
		want     string
	}{
		{"timeout-flag-900s", 900, "15m0s"},
		{"default-300s", 0, "5m0s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errW bytes.Buffer
			code := runFleetRun(fleetRunParams{
				Agent: "agy-fake", Model: "anthropic/claude-sonnet-4-6", Cwd: t.TempDir(),
				Input: writePromptFile(t, root), TimeoutS: tc.timeoutS, DryRun: true, JSON: true, ProjectRoot: root,
			}, &out, &errW)
			if code != 0 {
				t.Fatalf("--dry-run must exit 0, got %d\nstdout=%s\nstderr=%s", code, out.String(), errW.String())
			}
			data, _ := decodeEnvelope(t, out.Bytes())["data"].(map[string]any)
			rawArgv, _ := data["argv"].([]any)
			argv := make([]string, len(rawArgv))
			for i, v := range rawArgv {
				argv[i], _ = v.(string)
			}
			if got := argAfter(argv, "--print-timeout"); got != tc.want {
				t.Fatalf("want --print-timeout %s in rendered argv, got argv=%v", tc.want, argv)
			}
		})
	}
}

// setupFleetRunOpencodeProject builds an opencode-shaped fake transport for
// 'fleet run': prompt travels as a trailing argv token, {cwd} resolves to
// p.Cwd (FLEET-020), env carries a marker var, and stdin_empty is true. The
// fake binary records its argv, stdin, and the marker env var to files under
// its cwd so tests can assert all three.
func setupFleetRunOpencodeProject(t *testing.T) string {
	t.Helper()
	// Isolate from any QUORUM_FLEET_AGENTS exported in the invoking shell.
	t.Setenv("QUORUM_FLEET_AGENTS", "")
	root := t.TempDir()
	script := filepath.Join(root, "fake-opencode.sh")
	body := "#!/bin/sh\nprintf '%s\\n' \"$@\" > args.txt\ncat > stdin.txt\nprintf '%s' \"$FLEET_TEST_ENV_MARKER\" > env.txt\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	agentsDir := filepath.Join(root, ".agents", "fleet")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	agents := "transports:\n" +
		"  opencode-fake:\n" +
		"    binary: " + script + "\n" +
		"    env:\n" +
		"      FLEET_TEST_ENV_MARKER: marker-value\n" +
		"    argv_template:\n" +
		"      - run\n" +
		"      - \"{prompt}\"\n" +
		"      - -m\n" +
		"      - \"{model_arg}\"\n" +
		"      - --dir\n" +
		"      - \"{cwd}\"\n" +
		"    input_channel: prompt_arg\n" +
		"    output_format: json\n" +
		"    stdin_empty: true\n" +
		"    timeouts:\n" +
		"      default_s: 30\n" +
		"    failure_signatures: []\n" +
		"    active: true\n" +
		"    models:\n" +
		"      openrouter/free:\n" +
		"        model_arg: openrouter/openrouter/free\n"
	if err := os.WriteFile(filepath.Join(agentsDir, "agents.yaml"), []byte(agents), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestFleetRunOpencodeCwdEnvAndStdinEmpty is FLEET-020 AC-2: the {cwd}
// placeholder substitutes to --cwd on the standalone 'fleet run' path too
// (not just dispatch/smoke), transport.Env is applied before exec, and
// stdin_empty forces an empty stdin even though the prompt still arrives via
// argv.
func TestFleetRunOpencodeCwdEnvAndStdinEmpty(t *testing.T) {
	os.Unsetenv("FLEET_TEST_ENV_MARKER")
	t.Cleanup(func() { os.Unsetenv("FLEET_TEST_ENV_MARKER") })
	root := setupFleetRunOpencodeProject(t)
	cwd := t.TempDir()
	var out, errW bytes.Buffer
	code := runFleetRun(fleetRunParams{
		Agent: "opencode-fake", Model: "openrouter/free", Cwd: cwd,
		Input: writePromptFile(t, root), JSON: true, ProjectRoot: root,
	}, &out, &errW)
	if code != 0 {
		t.Fatalf("want exit 0, got %d\nstdout=%s\nstderr=%s", code, out.String(), errW.String())
	}
	argvRaw, err := os.ReadFile(filepath.Join(cwd, "args.txt"))
	if err != nil {
		t.Fatalf("read args.txt: %v", err)
	}
	argv := strings.Split(strings.TrimRight(string(argvRaw), "\n"), "\n")
	if got := argAfter(argv, "--dir"); got != cwd {
		t.Fatalf("want --dir %s (cwd substituted), got argv=%v", cwd, argv)
	}
	envRaw, err := os.ReadFile(filepath.Join(cwd, "env.txt"))
	if err != nil {
		t.Fatalf("read env.txt: %v", err)
	}
	if string(envRaw) != "marker-value" {
		t.Fatalf("want transport.Env applied before exec, got env.txt=%q", envRaw)
	}
	stdinRaw, err := os.ReadFile(filepath.Join(cwd, "stdin.txt"))
	if err != nil {
		t.Fatalf("read stdin.txt: %v", err)
	}
	if len(stdinRaw) != 0 {
		t.Fatalf("want empty stdin for stdin_empty:true transport, got stdin.txt=%q", stdinRaw)
	}
}

// setupFleetRunAgyPromptPointerProject builds an agy-shaped fake transport
// declaring input_channel: prompt_pointer (FLEET-024). The fake binary
// records its full argv (one token per line) and its stdin content, so tests
// can assert the final --print token is a small pointer instruction, never
// raw prompt content, and that stdin is empty.
func setupFleetRunAgyPromptPointerProject(t *testing.T) string {
	t.Helper()
	t.Setenv("QUORUM_FLEET_AGENTS", "")
	root := t.TempDir()
	script := filepath.Join(root, "fake-agy-pointer.sh")
	body := "#!/bin/sh\nprintf '%s\\n' \"$@\" > args.txt\ncat > stdin.txt\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	agentsDir := filepath.Join(root, ".agents", "fleet")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	agents := "transports:\n" +
		"  agy:\n" +
		"    binary: " + script + "\n" +
		"    argv_template:\n" +
		"      - --model\n" +
		"      - \"{model_arg}\"\n" +
		"      - --print-timeout\n" +
		"      - \"{print_timeout}\"\n" +
		"      - --print\n" +
		"      - \"{prompt}\"\n" +
		"    input_channel: prompt_pointer\n" +
		"    output_format: text\n" +
		"    timeouts:\n" +
		"      default_s: 30\n" +
		"    failure_signatures: []\n" +
		"    active: true\n" +
		"    models:\n" +
		"      test/agy-model-a:\n" +
		"        model_arg: Test Agy Model A\n"
	if err := os.WriteFile(filepath.Join(agentsDir, "agents.yaml"), []byte(agents), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestFleetRunAgyPromptPointerLargeInputFileAvoidsE2BIG is FLEET-024 AC-4:
// 'quorum fleet run' on the prompt_pointer channel renders a small pointer
// --print token targeting the absolute path of a large --input file and runs
// the delegate successfully (no fork/exec E2BIG).
func TestFleetRunAgyPromptPointerLargeInputFileAvoidsE2BIG(t *testing.T) {
	root := setupFleetRunAgyPromptPointerProject(t)
	cwd := t.TempDir()
	promptPath := filepath.Join(root, "large-prompt.txt")
	large := strings.Repeat("A", 200*1024) // > 128 KiB MAX_ARG_STRLEN
	if err := os.WriteFile(promptPath, []byte(large), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errW bytes.Buffer
	code := runFleetRun(fleetRunParams{
		Agent: "agy", Model: "test/agy-model-a", Cwd: cwd,
		Input: promptPath, JSON: true, ProjectRoot: root,
	}, &out, &errW)
	if code != 0 {
		t.Fatalf("want exit 0 (no E2BIG), got %d\nstdout=%s\nstderr=%s", code, out.String(), errW.String())
	}
	argvRaw, err := os.ReadFile(filepath.Join(cwd, "args.txt"))
	if err != nil {
		t.Fatalf("read args.txt: %v", err)
	}
	argv := strings.Split(strings.TrimRight(string(argvRaw), "\n"), "\n")
	got := argv[len(argv)-1]
	if len(got) > 1024 {
		t.Fatalf("want a small pointer instruction, got %d bytes", len(got))
	}
	absPrompt, aerr := filepath.Abs(promptPath)
	if aerr != nil {
		t.Fatal(aerr)
	}
	if !strings.Contains(got, absPrompt) {
		t.Fatalf("want the pointer instruction to reference %s, got %q", absPrompt, got)
	}
	stdinRaw, err := os.ReadFile(filepath.Join(cwd, "stdin.txt"))
	if err != nil {
		t.Fatalf("read stdin.txt: %v", err)
	}
	if len(stdinRaw) != 0 {
		t.Fatalf("want empty stdin for prompt_pointer channel, got %d bytes", len(stdinRaw))
	}
}

// TestFleetRunAgyPromptPointerStdinInputMaterializesTempFile is FLEET-024
// AC-4: with --input - (stdin), the large prompt has no on-disk path yet, so
// fleet run must materialize it to a temp file inside --cwd, point the
// delegate at that file's absolute path, and remove it after the delegate
// runs.
func TestFleetRunAgyPromptPointerStdinInputMaterializesTempFile(t *testing.T) {
	root := setupFleetRunAgyPromptPointerProject(t)
	cwd := t.TempDir()
	large := strings.Repeat("A", 200*1024) // > 128 KiB MAX_ARG_STRLEN
	var out, errW bytes.Buffer
	code := runFleetRun(fleetRunParams{
		Agent: "agy", Model: "test/agy-model-a", Cwd: cwd,
		Input: "-", Stdin: strings.NewReader(large), JSON: true, ProjectRoot: root,
	}, &out, &errW)
	if code != 0 {
		t.Fatalf("want exit 0 (no E2BIG), got %d\nstdout=%s\nstderr=%s", code, out.String(), errW.String())
	}
	argvRaw, err := os.ReadFile(filepath.Join(cwd, "args.txt"))
	if err != nil {
		t.Fatalf("read args.txt: %v", err)
	}
	argv := strings.Split(strings.TrimRight(string(argvRaw), "\n"), "\n")
	got := argv[len(argv)-1]
	if len(got) > 1024 {
		t.Fatalf("want a small pointer instruction, got %d bytes", len(got))
	}
	if !strings.Contains(got, cwd) {
		t.Fatalf("want the materialized temp prompt file to live inside --cwd %s, got %q", cwd, got)
	}
	var materialized string
	for _, f := range strings.Fields(got) {
		if strings.HasPrefix(f, cwd) {
			materialized = f
			break
		}
	}
	if materialized == "" {
		t.Fatalf("could not find a materialized path token in %q", got)
	}
	if _, err := os.Stat(materialized); err == nil {
		t.Fatalf("want the temp prompt file removed after the delegate ran, still present: %s", materialized)
	}
}

// setupFleetRunOpencodeStdinProject mirrors the real opencode transport
// shape (FLEET-024): argv has no {prompt} token and input_channel is stdin,
// so the prompt must flow to stdin on the standalone 'fleet run' path too.
func setupFleetRunOpencodeStdinProject(t *testing.T) string {
	t.Helper()
	t.Setenv("QUORUM_FLEET_AGENTS", "")
	root := t.TempDir()
	script := filepath.Join(root, "fake-opencode-stdin.sh")
	body := "#!/bin/sh\nprintf '%s\\n' \"$@\" > args.txt\ncat > stdin.txt\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	agentsDir := filepath.Join(root, ".agents", "fleet")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	agents := "transports:\n" +
		"  opencode-stdin-fake:\n" +
		"    binary: " + script + "\n" +
		"    argv_template:\n" +
		"      - run\n" +
		"      - -m\n" +
		"      - \"{model_arg}\"\n" +
		"      - --dir\n" +
		"      - \"{cwd}\"\n" +
		"    input_channel: stdin\n" +
		"    output_format: json\n" +
		"    timeouts:\n" +
		"      default_s: 30\n" +
		"    failure_signatures: []\n" +
		"    active: true\n" +
		"    models:\n" +
		"      openrouter/free:\n" +
		"        model_arg: openrouter/openrouter/free\n"
	if err := os.WriteFile(filepath.Join(agentsDir, "agents.yaml"), []byte(agents), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestFleetRunOpencodeStdinDelivery is FLEET-024 AC-4: 'quorum fleet run' on
// the stdin channel delivers the prompt via stdin with no inline argv token
// and returns a normal success envelope.
func TestFleetRunOpencodeStdinDelivery(t *testing.T) {
	root := setupFleetRunOpencodeStdinProject(t)
	cwd := t.TempDir()
	var out, errW bytes.Buffer
	code := runFleetRun(fleetRunParams{
		Agent: "opencode-stdin-fake", Model: "openrouter/free", Cwd: cwd,
		Input: writePromptFile(t, root), JSON: true, ProjectRoot: root,
	}, &out, &errW)
	if code != 0 {
		t.Fatalf("want exit 0, got %d\nstdout=%s\nstderr=%s", code, out.String(), errW.String())
	}
	env := decodeEnvelope(t, out.Bytes())
	if env["ok"] != true {
		t.Fatalf("want ok:true, got %v", env)
	}
	stdinRaw, err := os.ReadFile(filepath.Join(cwd, "stdin.txt"))
	if err != nil {
		t.Fatalf("read stdin.txt: %v", err)
	}
	if string(stdinRaw) != "say hi" {
		t.Fatalf("want the prompt delivered via stdin, got stdin.txt=%q", stdinRaw)
	}
	argvRaw, err := os.ReadFile(filepath.Join(cwd, "args.txt"))
	if err != nil {
		t.Fatalf("read args.txt: %v", err)
	}
	argv := strings.Split(strings.TrimRight(string(argvRaw), "\n"), "\n")
	for _, tok := range argv {
		if tok == "say hi" {
			t.Fatal("want no argv token equal to the prompt content")
		}
	}
}

// TestFleetRunPromptWithLiteralBracesPasses is the regression test for the
// false-positive placeholder rejection: a prompt_arg transport (agy-like)
// whose prompt contains literal '{'/'}' (e.g. Go code) must not be rejected
// as an unresolved dispatch-only placeholder, because the residual check now
// scans the raw argv_template before {prompt} substitution, not the argv
// after the user's prompt has been substituted in.
func TestFleetRunPromptWithLiteralBracesPasses(t *testing.T) {
	root, _ := setupFleetRunProject(t)
	cwd := t.TempDir()
	promptPath := filepath.Join(root, "code-prompt.txt")
	codePrompt := `func main() { fmt.Println("{}") }`
	if err := os.WriteFile(promptPath, []byte(codePrompt), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errW bytes.Buffer
	code := runFleetRun(fleetRunParams{
		Agent: "fake", Model: "anthropic/claude-sonnet-4-6", Cwd: cwd,
		Input: promptPath, DryRun: true, JSON: true, ProjectRoot: root,
	}, &out, &errW)
	if code != 0 {
		t.Fatalf("prompt with literal braces must pass validation under --dry-run, got exit %d\nstdout=%s\nstderr=%s", code, out.String(), errW.String())
	}
	env := decodeEnvelope(t, out.Bytes())
	if env["ok"] != true {
		t.Fatalf("want ok:true, got %v", env)
	}
	data, _ := env["data"].(map[string]any)
	rawArgv, _ := data["argv"].([]any)
	found := false
	for _, v := range rawArgv {
		if s, _ := v.(string); s == codePrompt {
			found = true
		}
	}
	if !found {
		t.Fatalf("want the code prompt substituted verbatim into argv, got argv=%v", rawArgv)
	}
}

// TestFleetRunDispatchOnlyPlaceholderWorktreeRejected asserts a template
// referencing the dispatch-only {worktree} placeholder is still rejected by
// 'fleet run' (it is not in the vars map fleet run builds), naming the
// placeholder itself in the error message.
func TestFleetRunDispatchOnlyPlaceholderWorktreeRejected(t *testing.T) {
	root := t.TempDir()
	agentsDir := filepath.Join(root, ".agents", "fleet")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	agents := "transports:\n" +
		"  worktree-only:\n" +
		"    binary: /bin/true\n" +
		"    argv_template:\n" +
		"      - --model\n" +
		"      - \"{model_arg}\"\n" +
		"      - --worktree\n" +
		"      - \"{worktree}\"\n" +
		"      - \"{prompt}\"\n" +
		"    output_format: text\n" +
		"    timeouts:\n" +
		"      default_s: 30\n" +
		"    failure_signatures: []\n" +
		"    active: true\n" +
		"    models:\n" +
		"      anthropic/claude-sonnet-4-6:\n" +
		"        model_arg: x\n"
	if err := os.WriteFile(filepath.Join(agentsDir, "agents.yaml"), []byte(agents), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errW bytes.Buffer
	code := runFleetRun(fleetRunParams{
		Agent: "worktree-only", Model: "anthropic/claude-sonnet-4-6", Cwd: t.TempDir(),
		Input: writePromptFile(t, root), JSON: true, ProjectRoot: root,
	}, &out, &errW)
	if code == 0 {
		t.Fatal("template referencing {worktree} must be rejected by fleet run")
	}
	e, _ := decodeEnvelope(t, out.Bytes())["error"].(map[string]any)
	if e["code"] != "INVALID_ARGUMENT" {
		t.Fatalf("bad error code: %v", e)
	}
	if msg, _ := e["message"].(string); !strings.Contains(msg, "worktree") {
		t.Fatalf("message must name the offending placeholder %q, got %v", "worktree", e["message"])
	}
}

// TestFleetRunUnknownPlaceholderRejected asserts an unrecognized {bogus}
// placeholder in the template is rejected exactly like a known
// dispatch-only one.
func TestFleetRunUnknownPlaceholderRejected(t *testing.T) {
	root := t.TempDir()
	agentsDir := filepath.Join(root, ".agents", "fleet")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	agents := "transports:\n" +
		"  bogus-only:\n" +
		"    binary: /bin/true\n" +
		"    argv_template:\n" +
		"      - --model\n" +
		"      - \"{model_arg}\"\n" +
		"      - --bogus\n" +
		"      - \"{bogus}\"\n" +
		"      - \"{prompt}\"\n" +
		"    output_format: text\n" +
		"    timeouts:\n" +
		"      default_s: 30\n" +
		"    failure_signatures: []\n" +
		"    active: true\n" +
		"    models:\n" +
		"      anthropic/claude-sonnet-4-6:\n" +
		"        model_arg: x\n"
	if err := os.WriteFile(filepath.Join(agentsDir, "agents.yaml"), []byte(agents), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errW bytes.Buffer
	code := runFleetRun(fleetRunParams{
		Agent: "bogus-only", Model: "anthropic/claude-sonnet-4-6", Cwd: t.TempDir(),
		Input: writePromptFile(t, root), JSON: true, ProjectRoot: root,
	}, &out, &errW)
	if code == 0 {
		t.Fatal("template referencing an unknown {bogus} placeholder must be rejected by fleet run")
	}
	e, _ := decodeEnvelope(t, out.Bytes())["error"].(map[string]any)
	if e["code"] != "INVALID_ARGUMENT" {
		t.Fatalf("bad error code: %v", e)
	}
	if msg, _ := e["message"].(string); !strings.Contains(msg, "bogus") {
		t.Fatalf("message must name the offending placeholder %q, got %v", "bogus", e["message"])
	}
}

func TestFleetRunDryRun(t *testing.T) {
	root, marker := setupFleetRunProject(t)
	cwd := t.TempDir()
	var out, errW bytes.Buffer
	code := runFleetRun(fleetRunParams{
		Agent: "fake", Model: "anthropic/claude-sonnet-4-6", Cwd: cwd,
		Input: writePromptFile(t, root), DryRun: true, JSON: true, ProjectRoot: root,
	}, &out, &errW)
	if code != 0 {
		t.Fatalf("--dry-run must exit 0, got %d", code)
	}
	if _, err := os.Stat(filepath.Join(cwd, marker)); err == nil {
		t.Fatal("--dry-run must not run a delegate")
	}
}
