package server

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repoRoot resolves the repository root from this test file's location, so the
// viewer-vs-schema drift checks read the live app.js and report.schema.json
// instead of duplicated copies.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func readReportSchemaEnum(t *testing.T, defKey, field string) []string {
	t.Helper()
	root := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, ".agents", "schemas", "report.schema.json"))
	if err != nil {
		t.Fatalf("read report schema: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("parse report schema: %v", err)
	}
	defs, _ := schema["$defs"].(map[string]any)
	def, _ := defs[defKey].(map[string]any)
	props, _ := def["properties"].(map[string]any)
	f, _ := props[field].(map[string]any)
	enum, _ := f["enum"].([]any)
	var out []string
	for _, e := range enum {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		t.Fatalf("no enum values found at $defs.%s.properties.%s.enum", defKey, field)
	}
	return out
}

func readAppJS(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "internal", "server", "web", "app.js"))
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	return string(b)
}

// TestViewerRoleRenderersCoverSchema guards proposal §11.2: every semantic role
// the schema accepts must have a render branch in app.js. The dispatch is a
// `switch (sec.role)` (not a ROLE_RENDERERS map as the proposal sketched), so we
// assert a `case '<role>':` exists per role. A new role added to the schema
// without a renderer fails here.
func TestViewerRoleRenderersCoverSchema(t *testing.T) {
	roles := readReportSchemaEnum(t, "semanticSection", "role")
	app := readAppJS(t)
	for _, role := range roles {
		if !strings.Contains(app, fmt.Sprintf("case '%s':", role)) {
			t.Errorf("app.js has no `case '%s':` branch for schema role %q (viewer/schema drift)", role, role)
		}
	}
}

// TestViewerPreservesAuthoredSectionOrder guards the inverted-pyramid contract
// (q-report cognitive-load directives, 2026-08-03): the author owns section
// order, so app.js must render content.sections exactly as authored and never
// re-sort by presentation profile.
func TestViewerPreservesAuthoredSectionOrder(t *testing.T) {
	app := readAppJS(t)
	for _, forbidden := range []string{"PROFILE_ORDER", "orderSections("} {
		if strings.Contains(app, forbidden) {
			t.Errorf("app.js reintroduces %q — sections must render in authored order (inverted pyramid)", forbidden)
		}
	}
	if !strings.Contains(app, "const orderedSections = content.sections || [];") {
		t.Error("app.js no longer renders content.sections in authored order")
	}
}

func TestViewerLoadsSplitStylesheetsAndMemoryUI(t *testing.T) {
	root := repoRoot(t)
	index, err := os.ReadFile(filepath.Join(root, "internal", "server", "web", "index.html"))
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	app := readAppJS(t)
	idx := string(index)
	for _, want := range []string{`href="styles.css"`, `href="style.css"`, `id="memories-tab"`, `id="memory-list"`, `id="tasks-tab"`, `id="task-list"`} {
		if !strings.Contains(idx, want) {
			t.Errorf("index.html missing %s", want)
		}
	}
	for _, want := range []string{"/memories", "renderMemory", "memoryType", "/tasks", "loadTasks", "renderTask"} {
		if !strings.Contains(app, want) {
			t.Errorf("app.js missing memory/task UI marker %q", want)
		}
	}
}

func TestViewerAppJSSyntax(t *testing.T) {
	appPath := filepath.Join(repoRoot(t), "internal", "server", "web", "app.js")
	cmd := exec.Command("node", "--check", appPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("app.js syntax check failed: %v\n%s", err, string(out))
	}
}
