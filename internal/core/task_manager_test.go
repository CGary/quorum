package core

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func sourceRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func useSchemas(t *testing.T) {
	t.Helper()
	t.Setenv("QUORUM_SCHEMAS_DIR", filepath.Join(sourceRoot(t), ".agents", "schemas"))
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

func run(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return string(out)
}

func runErr(t *testing.T, dir, name string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	run(t, root, "git", "init", "-q", "-b", "main", ".")
	run(t, root, "git", "config", "user.email", "test@example.com")
	run(t, root, "git", "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(root, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, root, "git", "add", "seed.txt")
	run(t, root, "git", "commit", "-q", "-m", "init")
	return root
}

func ensureTaskDirs(t *testing.T, root string) {
	t.Helper()
	for _, loc := range []string{"inbox", "active", "done", "failed"} {
		if err := os.MkdirAll(filepath.Join(root, ".ai", "tasks", loc), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "worktrees"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func mkSpec(t *testing.T, root, loc, dir, id string) string {
	t.Helper()
	p := filepath.Join(root, ".ai", "tasks", loc, dir)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "00-spec.yaml"), []byte("task_id: "+id+"\nsummary: test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func mkFullSpec(t *testing.T, dir, taskID string) {
	t.Helper()
	raw := "task_id: " + taskID + "\n" +
		"summary: Test task\n" +
		"goal: Exercise native Go task manager behavior.\n" +
		"invariants:\n  - Preserve task artifacts.\n" +
		"acceptance:\n  - Behavior is covered by this test.\n" +
		"risk: medium\n" +
		"non_goals: []\n" +
		"constraints: []\n"
	if err := os.WriteFile(filepath.Join(dir, "00-spec.yaml"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mkContract(t *testing.T, dir, taskID string) {
	t.Helper()
	raw := "task_id: " + taskID + "\n" +
		"summary: Valid test contract\n" +
		"goal: Exercise task start contract validation.\n" +
		"read:\n  - internal/core/task_manager.go\n" +
		"touch:\n  - internal/core/task_manager_test.go\n" +
		"forbid:\n  files: []\n  behaviors: []\n" +
		"verify:\n  commands:\n    - go test ./internal/core\n" +
		"acceptance:\n  human_gate: true\n" +
		"limits:\n  max_files_changed: 1\n  max_diff_lines: 200\n" +
		"execution:\n  mode: worktree_edit\n" +
		"retry_policy:\n  max_attempts: 2\n"
	if err := os.WriteFile(filepath.Join(dir, "02-contract.yaml"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mkActiveTask(t *testing.T, root, taskID string) string {
	t.Helper()
	ensureTaskDirs(t, root)
	dir := filepath.Join(root, ".ai", "tasks", "active", taskID+"-new-spec")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mkFullSpec(t, dir, taskID)
	return dir
}

func mkActiveTaskWithWorktree(t *testing.T, taskID string) (root, taskDir, worktree string) {
	t.Helper()
	root = initGitRepo(t)
	chdir(t, root)
	taskDir = mkActiveTask(t, root, taskID)
	worktree = filepath.Join(root, "worktrees", taskID)
	run(t, root, "git", "worktree", "add", "-q", "-b", "ai/"+taskID, worktree)
	return root, taskDir, worktree
}

func buildQuorumCLI(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "quorum-test")
	run(t, sourceRoot(t), "go", "build", "-o", bin, ".")
	return bin
}

func TestFindTaskDirParity(t *testing.T) {
	root := t.TempDir()
	mkSpec(t, root, "active", "LEGACY-999-slug", "FEAT-001")
	mkSpec(t, root, "active", "FEAT-001", "OTHER-001")
	m, err := FindTaskDirIn(root, "FEAT-001", []string{"active"})
	if err != nil || m == nil || filepath.Base(m.Path) != "LEGACY-999-slug" || m.Location != "active" {
		t.Fatalf("yaml tier = %#v, %v", m, err)
	}

	mkSpec(t, root, "active", "FEAT-002", "OTHER-002")
	m, err = FindTaskDirIn(root, "FEAT-002", []string{"active"})
	if err != nil || m == nil || filepath.Base(m.Path) != "FEAT-002" {
		t.Fatalf("exact tier = %#v, %v", m, err)
	}

	mkSpec(t, root, "inbox", "FEAT-003-a-new-spec", "FEAT-003-a")
	m, err = FindTaskDirIn(root, "FEAT-003", []string{"inbox"})
	if err != nil || m != nil {
		t.Fatalf("parent resolved to child = %#v, %v", m, err)
	}
	m, err = FindTaskDirIn(root, "FEAT-003-a", []string{"inbox"})
	if err != nil || m == nil || filepath.Base(m.Path) != "FEAT-003-a-new-spec" {
		t.Fatalf("child tier = %#v, %v", m, err)
	}

	mkSpec(t, root, "done", "FEAT-004-alpha", "OTHER-004")
	mkSpec(t, root, "done", "FEAT-004-beta", "OTHER-005")
	_, err = FindTaskDirIn(root, "FEAT-004", []string{"done"})
	if err == nil || !strings.Contains(err.Error(), "AMBIGUITY ERROR") || !strings.Contains(err.Error(), "done/FEAT-004-alpha") {
		t.Fatalf("ambiguity = %v", err)
	}
}

func TestValidateArtifactErrorFormatAndTraceAppendOnly(t *testing.T) {
	useSchemas(t)
	cases := []struct {
		path    string
		payload map[string]any
		want    string
	}{
		{"01-blueprint.yaml", map[string]any{"task_id": "FEAT-001", "summary": "valid blueprint", "affected_files": []any{"src/a.py"}, "symbols": []any{}, "dependencies": []any{}}, "artifact=01-blueprint.yaml; field=$; reason='test_scenarios' is a required property"},
		{"05-validation.json", map[string]any{"task_id": "FEAT-001", "summary": "invalid", "executed_at": "2026-05-03T00:00:00Z", "commands": []any{}, "overall_result": "passed"}, "artifact=05-validation.json; field=$.commands; reason=[] should be non-empty"},
		{"05-validation.json", map[string]any{"task_id": "FEAT-001", "summary": "invalid", "executed_at": "2026-05-03T00:00:00Z", "commands": []any{map[string]any{"command": "pytest", "exit_code": 0, "duration_s": 1.0, "output_excerpt": "", "extra": true}}, "overall_result": "passed"}, "artifact=05-validation.json; field=$.commands[0]; reason=Additional properties are not allowed ('extra' was unexpected)"},
	}
	for _, tc := range cases {
		if err := ValidateArtifact(tc.path, tc.payload); err == nil || err.Error() != tc.want {
			t.Fatalf("%s error = %v, want %s", tc.path, err, tc.want)
		}
	}

	existing := map[string]any{"attempts": []any{map[string]any{"phase": "blueprint", "result": "passed", "duration_s": 1.0}}}
	appended := map[string]any{"attempts": []any{map[string]any{"phase": "blueprint", "result": "passed", "duration_s": 1.0}, map[string]any{"phase": "execute", "result": "passed", "duration_s": 2.0}}}
	if err := EnsureTraceAppendOnly("07-trace.json", existing, appended); err != nil {
		t.Fatal(err)
	}
	want := "artifact=07-trace.json; field=$.attempts; reason=append-only trace cannot remove existing attempts"
	if err := EnsureTraceAppendOnly("07-trace.json", existing, map[string]any{"attempts": []any{}}); err == nil || err.Error() != want {
		t.Fatalf("shorten = %v", err)
	}
	want = "artifact=07-trace.json; field=$.attempts; reason=append-only trace cannot reorder or mutate existing attempts"
	mutated := map[string]any{"attempts": []any{map[string]any{"phase": "blueprint", "result": "failed", "duration_s": 1.0}}}
	if err := EnsureTraceAppendOnly("07-trace.json", existing, mutated); err == nil || err.Error() != want {
		t.Fatalf("mutate = %v", err)
	}
}

func TestSaveArtifactValidatesBeforeOverwrite(t *testing.T) {
	useSchemas(t)
	root := t.TempDir()
	blueprintPath := filepath.Join(root, "01-blueprint.yaml")
	valid := map[string]any{"task_id": "FEAT-001", "summary": "valid blueprint", "affected_files": []any{"src/a.py"}, "symbols": []any{}, "dependencies": []any{}, "test_scenarios": []any{"works"}}
	if _, err := SaveArtifact(blueprintPath, valid); err != nil {
		t.Fatal(err)
	}
	invalid := map[string]any{"task_id": "FEAT-001", "summary": "broken", "affected_files": []any{}}
	if _, err := SaveArtifact(blueprintPath, invalid); err == nil {
		t.Fatal("expected invalid blueprint to be rejected")
	}
	persisted, err := LoadArtifactPayload(blueprintPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(mustJSON(t, persisted)) != string(mustJSON(t, valid)) {
		t.Fatalf("invalid save overwrote existing artifact: %#v", persisted)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestTaskLifecycleSpecifyBlueprintStartBackAndSplit(t *testing.T) {
	useSchemas(t)
	root := initGitRepo(t)
	chdir(t, root)
	ensureTaskDirs(t, root)

	dir, err := InitializeSpecify("FEAT-010")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(dir) != "FEAT-010-new-spec" {
		t.Fatalf("spec dir = %s", filepath.Base(dir))
	}
	active, err := PrepareBlueprint("FEAT-010")
	if err != nil {
		t.Fatal(err)
	}
	mkContract(t, active, "FEAT-010")

	StartTask("FEAT-010")
	worktree := filepath.Join(root, "worktrees", "FEAT-010")
	if st, err := os.Stat(worktree); err != nil || !st.IsDir() {
		t.Fatalf("worktree was not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(active, "04-implementation-log.yaml")); err != nil {
		t.Fatalf("implementation log missing: %v", err)
	}
	trace, err := LoadArtifactPayload(filepath.Join(active, "07-trace.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := trace.(map[string]any)["task_id"]; got != "FEAT-010" {
		t.Fatalf("trace task_id = %v", got)
	}

	BackTask("FEAT-010")
	if _, err := os.Stat(worktree); !os.IsNotExist(err) {
		t.Fatalf("worktree still exists after back: %v", err)
	}
	if _, err := os.Stat(active); err != nil {
		t.Fatalf("active task should remain after reversing start: %v", err)
	}

	parentDir := filepath.Join(root, ".ai", "tasks", "active", "FEAT-020-parent")
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	parentSpec := "task_id: FEAT-020\nsummary: Parent task\ngoal: Coordinate split children.\ninvariants:\n  - Keep child order.\nacceptance:\n  - Children are materialized.\nrisk: medium\nnon_goals: []\nconstraints: []\ndecomposition:\n  - child_id: FEAT-020-a\n    summary: First child\n  - child_id: FEAT-020-b\n    summary: Second child\n"
	if err := os.WriteFile(filepath.Join(parentDir, "00-spec.yaml"), []byte(parentSpec), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SplitTask("FEAT-020"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".ai", "tasks", "inbox", "FEAT-020-a", "00-spec.yaml")); err != nil {
		t.Fatalf("child a missing: %v", err)
	}

	out := captureStdout(t, func() { CleanTask("FEAT-020", false, false) })
	if !strings.Contains(out, "unfinished children") {
		t.Fatalf("parent clean did not report unfinished children: %q", out)
	}
	for _, child := range []string{"FEAT-020-a", "FEAT-020-b"} {
		from := filepath.Join(root, ".ai", "tasks", "inbox", child)
		to := filepath.Join(root, ".ai", "tasks", "done", child)
		if err := os.Rename(from, to); err != nil {
			t.Fatal(err)
		}
	}
	CleanTask("FEAT-020", false, false)
	if _, err := os.Stat(filepath.Join(root, ".ai", "tasks", "done", "FEAT-020-parent")); err != nil {
		t.Fatalf("completed parent was not archived: %v", err)
	}
}

func TestCleanTaskDirtyWorktreeModes(t *testing.T) {
	cases := []struct {
		name         string
		dirty        bool
		force        bool
		save         bool
		wantWorktree bool
		wantDone     bool
		wantOut      []string
		wantStash    bool
		wantNoStash  bool
	}{
		{name: "clean archives without flags", wantDone: true, wantNoStash: true},
		{name: "dirty without flags aborts", dirty: true, wantWorktree: true, wantOut: []string{"uncommitted changes", "wip.txt", "--force", "--stash"}},
		{name: "dirty force discards and archives", dirty: true, force: true, wantDone: true, wantNoStash: true},
		{name: "dirty stash saves patch and archives", dirty: true, save: true, wantDone: true, wantStash: true},
		{name: "force and stash abort", dirty: true, force: true, save: true, wantWorktree: true, wantOut: []string{"mutually exclusive"}},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			taskID := "FEAT-1" + string(rune('0'+i))
			root, taskDir, worktree := mkActiveTaskWithWorktree(t, taskID)
			if tc.dirty {
				if err := os.WriteFile(filepath.Join(worktree, "wip.txt"), []byte("uncommitted\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			out := captureStdout(t, func() { CleanTask(taskID, tc.force, tc.save) })
			for _, want := range tc.wantOut {
				if !strings.Contains(out, want) {
					t.Fatalf("output %q missing %q", out, want)
				}
			}
			if _, err := os.Stat(worktree); (err == nil) != tc.wantWorktree {
				t.Fatalf("worktree exists = %v, want %v", err == nil, tc.wantWorktree)
			}
			if _, err := os.Stat(filepath.Join(root, ".ai", "tasks", "done", filepath.Base(taskDir))); (err == nil) != tc.wantDone {
				t.Fatalf("done task exists = %v, want %v", err == nil, tc.wantDone)
			}
			patches, _ := filepath.Glob(filepath.Join(root, "worktrees", ".stash", taskID+"-*.patch"))
			if tc.wantStash && len(patches) != 1 {
				t.Fatalf("stash patch count = %d, want 1", len(patches))
			}
			if tc.wantNoStash && len(patches) != 0 {
				t.Fatalf("unexpected stash patches: %#v", patches)
			}
		})
	}
}

func TestBackTaskDirtyWorktreeModes(t *testing.T) {
	cases := []struct {
		name, dirtyKind                       string
		force, stash, wantWorktree, wantPatch bool
		wantOut                               []string
	}{
		{name: "modified without flags aborts", dirtyKind: "modified", wantWorktree: true, wantOut: []string{"uncommitted changes", "seed.txt", "--force", "--stash"}},
		{name: "untracked force removes", dirtyKind: "untracked", force: true},
		{name: "mixed stash saves patch and removes", dirtyKind: "mixed", stash: true, wantPatch: true, wantOut: []string{"Saved worktree patch"}},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			taskID := "FEAT-2" + string(rune('0'+i))
			root, _, worktree := mkActiveTaskWithWorktree(t, taskID)
			if tc.dirtyKind == "modified" || tc.dirtyKind == "mixed" {
				if err := os.WriteFile(filepath.Join(worktree, "seed.txt"), []byte("changed\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if tc.dirtyKind == "untracked" || tc.dirtyKind == "mixed" {
				if err := os.WriteFile(filepath.Join(worktree, "new.txt"), []byte("new\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			out := captureStdout(t, func() { BackTask(taskID, tc.force, tc.stash) })
			for _, want := range tc.wantOut {
				if !strings.Contains(out, want) {
					t.Fatalf("output %q missing %q", out, want)
				}
			}
			if _, err := os.Stat(worktree); (err == nil) != tc.wantWorktree {
				t.Fatalf("worktree exists = %v, want %v", err == nil, tc.wantWorktree)
			}
			patches, _ := filepath.Glob(filepath.Join(root, "worktrees", ".stash", taskID+"-*.patch"))
			if tc.wantPatch && len(patches) != 1 {
				t.Fatalf("stash patch count = %d, want 1", len(patches))
			}
		})
	}
}

func TestTaskCLIArtifactSaveFeedbackConsumeAndRunRemoval(t *testing.T) {
	useSchemas(t)
	bin := buildQuorumCLI(t)
	root := initGitRepo(t)
	chdir(t, root)
	taskDir := mkActiveTask(t, root, "FEAT-001")

	out, _ := runErr(t, root, bin, "task", "run", "FEAT-001")
	if strings.Contains(out, "\n  run") {
		t.Fatalf("task run unexpectedly appears as a registered subcommand: %q", out)
	}
	entries, err := os.ReadDir(taskDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "00-spec.yaml" {
		t.Fatalf("task run created side effects: %#v", entries)
	}

	feedback := map[string]any{
		"task_id":      "FEAT-001",
		"summary":      "q-analyze found reusable feedback.",
		"produced_by":  "q-analyze",
		"generated_at": "2026-05-22T20:00:00Z",
		"findings": []any{map[string]any{
			"severity":      "low",
			"category":      "mechanical",
			"artifact":      "00-spec.yaml",
			"path":          "$.summary",
			"issue":         "Typo in summary.",
			"suggested_fix": "Fix the typo.",
		}},
	}
	raw := string(mustJSON(t, feedback))
	cmd := exec.Command(bin, "task", "artifact-save", "FEAT-001", "feedback.json")
	cmd.Dir = root
	cmd.Stdin = strings.NewReader(raw)
	outBytes, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("artifact-save failed: %v\n%s", err, outBytes)
	}
	if !strings.Contains(string(outBytes), "Saved artifact") {
		t.Fatalf("artifact-save output = %q", outBytes)
	}
	persisted, err := LoadArtifactPayload(filepath.Join(taskDir, "feedback.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(mustJSON(t, persisted)) != string(mustJSON(t, feedback)) {
		t.Fatalf("feedback mismatch: %#v", persisted)
	}

	out = run(t, root, bin, "task", "feedback-consume", "FEAT-001")
	if !strings.Contains(out, "Consumed feedback") {
		t.Fatalf("feedback-consume output = %q", out)
	}
	if _, err := os.Stat(filepath.Join(taskDir, "feedback.json")); !os.IsNotExist(err) {
		t.Fatalf("feedback.json still exists: %v", err)
	}
	if consumed, err := ConsumeFeedback(taskDir); err != nil || consumed {
		t.Fatalf("ConsumeFeedback idempotency = %v, %v", consumed, err)
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s content = %q, want %q", path, got, want)
	}
}

func assertNonEmptyFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("missing %s: %v", path, err)
	}
	if info.IsDir() || info.Size() == 0 {
		t.Fatalf("%s must be a non-empty file, size=%d dir=%v", path, info.Size(), info.IsDir())
	}
}

func TestCopyScaffoldSelfCopyIsNoop(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "skills", "q-brief", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("skill body\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := CopyFile(file, file); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, file, "skill body\n")

	if err := CopyDir(filepath.Join(root, "skills"), filepath.Join(root, "skills")); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, file, "skill body\n")
}

func TestInitializeProjectFromMovedBinaryCopiesNonEmptyResources(t *testing.T) {
	bin := buildQuorumCLI(t)
	root := initGitRepo(t)

	cmd := exec.Command(bin, "init", "--project-id", "moved-binary", "--project-name", "Moved Binary")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "QUORUM_MEMORY_DB="+filepath.Join(t.TempDir(), "memory.db"))
	outBytes, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s init failed: %v\n%s", bin, err, outBytes)
	}
	out := string(outBytes)
	if !strings.Contains(out, "Quorum initialized successfully") {
		t.Fatalf("init output = %q", out)
	}

	for _, path := range []string{
		".ai/tasks/_template/00-spec.yaml",
		".ai/tasks/_template/01-blueprint.yaml",
		".ai/tasks/_template/02-contract.yaml",
		".agents/config.yaml",
		".agents/schemas/spec.schema.json",
		".agents/schemas/blueprint.schema.json",
		".agents/policies/risk.yaml",
		".agents/policies/routing.yaml",
		".agents/prompts/architect/default.md",
	} {
		assertNonEmptyFile(t, filepath.Join(root, filepath.FromSlash(path)))
	}

	sourceSkills := filepath.Join(sourceRoot(t), ".agents", "skills")
	entries, err := os.ReadDir(sourceSkills)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "q-") {
			continue
		}
		count++
		assertNonEmptyFile(t, filepath.Join(root, ".agents", "skills", entry.Name(), "SKILL.md"))
	}
	if count != 10 {
		t.Fatalf("expected 10 source q-* skills, found %d", count)
	}

	link := filepath.Join(root, ".claude", "skills")
	if target, err := filepath.EvalSymlinks(link); err != nil || target != filepath.Join(root, ".agents", "skills") {
		t.Fatalf("skills symlink target = %s, %v", target, err)
	}
}

func TestInitializeProjectScaffoldingAndClaudeSkillsGuards(t *testing.T) {
	root := initGitRepo(t)
	chdir(t, root)
	resourceAgents := filepath.Join(root, ".agents")
	for _, path := range []string{
		filepath.Join(resourceAgents, "templates"),
		filepath.Join(resourceAgents, "skills", "q-brief"),
		filepath.Join(resourceAgents, "schemas"),
		filepath.Join(resourceAgents, "policies"),
		filepath.Join(resourceAgents, "prompts"),
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{
		filepath.Join(resourceAgents, "templates", "00-spec.yaml"),
		filepath.Join(resourceAgents, "templates", "01-blueprint.yaml"),
		filepath.Join(resourceAgents, "templates", "02-contract.yaml"),
		filepath.Join(resourceAgents, "skills", "q-brief", "SKILL.md"),
		filepath.Join(resourceAgents, "schemas", "spec.schema.json"),
		filepath.Join(resourceAgents, "schemas", "implementation-log.schema.json"),
		filepath.Join(resourceAgents, "policies", "risk.yaml"),
		filepath.Join(resourceAgents, "prompts", "brief.md"),
		filepath.Join(resourceAgents, "config.yaml"),
	} {
		if err := os.WriteFile(file, []byte("seed\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	t.Setenv("QUORUM_MEMORY_DB", filepath.Join(t.TempDir(), "memory.db"))
	if err := InitializeProjectWithOptions(InitOptions{ProjectID: "scaffold-test", ProjectName: "Scaffold Test", NonInteractive: true}); err != nil {
		t.Fatalf("InitializeProjectWithOptions failed: %v", err)
	}
	for _, file := range []string{
		filepath.Join(resourceAgents, "skills", "q-brief", "SKILL.md"),
		filepath.Join(resourceAgents, "schemas", "spec.schema.json"),
		filepath.Join(resourceAgents, "policies", "risk.yaml"),
		filepath.Join(resourceAgents, "prompts", "brief.md"),
		filepath.Join(resourceAgents, "config.yaml"),
	} {
		assertNonEmptyFile(t, file)
	}
	for _, path := range []string{
		".ai/tasks/inbox",
		".ai/tasks/active",
		"worktrees",
		".ai/tasks/_template/00-spec.yaml",
		".ai/tasks/_template/01-blueprint.yaml",
		".ai/tasks/_template/02-contract.yaml",
		".agents/schemas/spec.schema.json",
		".agents/schemas/implementation-log.schema.json",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil {
			t.Fatalf("missing %s: %v", path, err)
		}
	}
	gitignore, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gitignore), "worktrees/") || !strings.Contains(string(gitignore), ".ai/tasks/active/*") {
		t.Fatalf("gitignore missing quorum entries: %s", gitignore)
	}
	if _, err := os.Stat(filepath.Join(root, "memory")); !os.IsNotExist(err) {
		t.Fatalf("legacy memory scaffold should not be created, stat err=%v", err)
	}
	link := filepath.Join(root, ".claude", "skills")
	if target, err := filepath.EvalSymlinks(link); err != nil || target != filepath.Join(resourceAgents, "skills") {
		t.Fatalf("skills symlink target = %s, %v", target, err)
	}

	legacyRoot := t.TempDir()
	legacyResource := filepath.Join(legacyRoot, "quorum", ".agents")
	if err := os.MkdirAll(filepath.Join(legacyResource, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, ".claude", "skills")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(legacyResource, "skills"), filepath.Join(root, ".claude", "skills")); err != nil {
		t.Fatal(err)
	}
	if err := ensureClaudeSkillsSymlink(root, resourceAgents); err != nil {
		t.Fatalf("legacy symlink should be repaired: %v", err)
	}
	if target, err := filepath.EvalSymlinks(filepath.Join(root, ".claude", "skills")); err != nil || target != filepath.Join(resourceAgents, "skills") {
		t.Fatalf("repaired skills symlink target = %s, %v", target, err)
	}

	guardRoot := t.TempDir()
	guardResource := filepath.Join(t.TempDir(), "resources")
	if err := os.MkdirAll(filepath.Join(guardResource, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	blockingDir := filepath.Join(guardRoot, ".claude", "skills")
	if err := os.MkdirAll(blockingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(blockingDir, "user-content.txt")
	if err := os.WriteFile(marker, []byte("preserve me"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureClaudeSkillsSymlink(guardRoot, guardResource); err == nil || !strings.Contains(err.Error(), ".claude/skills") {
		t.Fatalf("expected directory guard error, got %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("guard overwrote existing directory content: %v", err)
	}
}

func TestSkillsMentionContextPrefixInCommunicationProtocol(t *testing.T) {
	skillsDir := filepath.Join(sourceRoot(t), ".agents", "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "q-") {
			continue
		}
		count++
		content, err := os.ReadFile(filepath.Join(skillsDir, entry.Name(), "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		beforeHandoff := strings.SplitN(string(content), "## 🛑 Handoff", 2)[0]
		for _, want := range []string{"Communication Protocol", "Prefijo de contexto", "[root]", "[worktree:"} {
			if !strings.Contains(beforeHandoff, want) {
				t.Fatalf("%s missing %q in communication protocol", entry.Name(), want)
			}
		}
	}
	if count != 10 {
		t.Fatalf("expected 10 q-* skills, found %d", count)
	}
}

func TestDeriveParentState(t *testing.T) {
	tmp := t.TempDir()
	cwd, _ := os.Getwd()
	os.Chdir(tmp)
	defer os.Chdir(cwd)
	os.Mkdir(".git", 0755)

	specActive := map[string]any{
		"decomposition": []any{
			map[string]any{"child_id": "PARENT-001-a"},
		},
	}
	os.MkdirAll(".ai/tasks/active/PARENT-001-a", 0755)
	if s := DeriveParentState(specActive); s != "active" {
		t.Fatalf("expected active, got %s", s)
	}

	specPartial := map[string]any{
		"decomposition": []any{
			map[string]any{"child_id": "PARENT-001-a"},
			map[string]any{"child_id": "PARENT-001-b"},
		},
	}
	os.MkdirAll(".ai/tasks/failed", 0755)
	os.Rename(".ai/tasks/active/PARENT-001-a", ".ai/tasks/failed/PARENT-001-a")
	os.MkdirAll(".ai/tasks/done/PARENT-001-b", 0755)
	if s := DeriveParentState(specPartial); s != "partial" {
		t.Fatalf("expected partial, got %s", s)
	}

	specCompleted := map[string]any{
		"decomposition": []any{
			map[string]any{"child_id": "PARENT-001-a"},
			map[string]any{"child_id": "PARENT-001-b"},
		},
	}
	os.Rename(".ai/tasks/failed/PARENT-001-a", ".ai/tasks/done/PARENT-001-a")
	if s := DeriveParentState(specCompleted); s != "completed" {
		t.Fatalf("expected completed, got %s", s)
	}

	if s := DeriveParentState(map[string]any{}); s != "active" {
		t.Fatalf("expected active, got %s", s)
	}
}

func TestPrepareFailedChildRetry(t *testing.T) {
	root := initGitRepo(t)
	chdir(t, root)
	useSchemas(t)
	ensureTaskDirs(t, root)

	os.MkdirAll(".ai/tasks/active/PARENT-001", 0755)
	os.MkdirAll(".ai/tasks/failed/PARENT-001-a", 0755)

	childSpec := map[string]any{
		"task_id":     "PARENT-001-a",
		"parent_task": "PARENT-001",
		"summary":     "this is a very long string that satisfies the minimum length requirement",
		"goal":        "this is a very long string that satisfies the minimum length requirement",
		"invariants":  []any{"this is a very long string that satisfies the minimum length requirement"},
		"acceptance":  []any{"this is a very long string that satisfies the minimum length requirement"},
		"risk":        "low",
	}
	childTrace := map[string]any{
		"task_id":           "PARENT-001-a",
		"summary":           "this is a very long string that satisfies the minimum length requirement",
		"started_at":        "2024-01-01T00:00:00Z",
		"execution_mode":    "patch_only",
		"total_cost_usd":    0.0,
		"violations":        []any{},
		"context_overflows": []any{},
		"attempts":          []any{},
	}

	if _, err := SaveArtifact(".ai/tasks/failed/PARENT-001-a/00-spec.yaml", childSpec); err != nil {
		t.Fatalf("Failed to save spec: %v", err)
	}
	if _, err := SaveArtifact(".ai/tasks/failed/PARENT-001-a/07-trace.json", childTrace); err != nil {
		t.Fatalf("Failed to save trace: %v", err)
	}

	childVal := map[string]any{
		"task_id":        "PARENT-001-a",
		"summary":        "this is a very long string that satisfies the minimum length requirement",
		"executed_at":    "2024-01-01T00:00:00Z",
		"overall_result": "passed",
		"commands": []any{
			map[string]any{
				"command":        "go test",
				"exit_code":      0,
				"duration_s":     0.0,
				"output_excerpt": "ok",
			},
		},
	}
	childRev := map[string]any{
		"task_id":                 "PARENT-001-a",
		"summary":                 "this is a very long string that satisfies the minimum length requirement",
		"verdict":                 "approve",
		"contract_compliance":     true,
		"forbidden_files_touched": []any{},
		"unrequested_refactor":    false,
		"missing_tests":           []any{},
		"functional_risk":         "low",
		"notes":                   []any{},
	}
	if _, err := SaveArtifact(".ai/tasks/failed/PARENT-001-a/05-validation.json", childVal); err != nil {
		t.Fatalf("Failed to save val: %v", err)
	}
	if _, err := SaveArtifact(".ai/tasks/failed/PARENT-001-a/06-review.json", childRev); err != nil {
		t.Fatalf("Failed to save rev: %v", err)
	}

	success := PrepareFailedChildRetry("PARENT-001-a")
	if !success {
		t.Fatalf("PrepareFailedChildRetry failed")
	}

	if _, err := os.Stat(".ai/tasks/active/PARENT-001-a"); err != nil {
		t.Fatalf("Child task not moved to active: %v", err)
	}
	if _, err := os.Stat(".ai/tasks/active/PARENT-001-a/05-validation.json"); err == nil {
		t.Fatalf("Stale validation artifact not removed")
	}
	if _, err := os.Stat(".ai/tasks/active/PARENT-001-a/06-review.json"); err == nil {
		t.Fatalf("Stale review artifact not removed")
	}
	if _, err := os.Stat(".ai/tasks/active/PARENT-001-a/07-trace.json"); err != nil {
		t.Fatalf("Trace artifact missing")
	}
}

func TestInitializeProjectWithOptionsCreatesConfigAndDoesNotCreateMemoryScaffold(t *testing.T) {
	useSchemas(t)
	root := initGitRepo(t)
	chdir(t, root)
	t.Setenv("QUORUM_MEMORY_DB", filepath.Join(t.TempDir(), "memory.db"))

	if err := InitializeProjectWithOptions(InitOptions{ProjectID: "sql-03", ProjectName: "SQL 03", NonInteractive: true}); err != nil {
		t.Fatalf("InitializeProjectWithOptions failed: %v", err)
	}
	if _, err := ReadQuorumConfigFrom(root); err != nil {
		t.Fatalf("expected .quorumrc to be created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "memory")); !os.IsNotExist(err) {
		t.Fatalf("legacy memory scaffold should not be recreated, stat err=%v", err)
	}
	if err := InitializeProjectWithOptions(InitOptions{NonInteractive: true}); err != nil {
		t.Fatalf("second init should be idempotent: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "memory")); !os.IsNotExist(err) {
		t.Fatalf("second init should not recreate memory scaffold, stat err=%v", err)
	}
}

func TestInitializeProjectWithOptionsRequiresIdentityInNonInteractiveMode(t *testing.T) {
	root := initGitRepo(t)
	chdir(t, root)
	t.Setenv("QUORUM_MEMORY_DB", filepath.Join(t.TempDir(), "memory.db"))

	err := InitializeProjectWithOptions(InitOptions{NonInteractive: true})
	if err == nil || !strings.Contains(err.Error(), "provide --project-id and --project-name") {
		t.Fatalf("expected non-interactive identity error, got %v", err)
	}
}
