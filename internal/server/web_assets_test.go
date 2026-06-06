package server

import (
	"encoding/json"
	"fmt"
	"os"
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

// TestViewerProfileOrderCoversSchema guards proposal §11.2: every presentation
// profile the schema accepts must have a PROFILE_ORDER entry in app.js, so a new
// profile cannot ship without a defined section order.
func TestViewerProfileOrderCoversSchema(t *testing.T) {
	profiles := readReportSchemaEnum(t, "presentationModel", "profile")
	app := readAppJS(t)

	start := strings.Index(app, "const PROFILE_ORDER = {")
	if start == -1 {
		t.Fatal("app.js does not declare `const PROFILE_ORDER = {`")
	}
	block := app[start:]
	if end := strings.Index(block, "};"); end != -1 {
		block = block[:end]
	}

	for _, profile := range profiles {
		// Match the object key `profile:` at the start of a (trimmed) line.
		key := profile + ":"
		found := false
		for _, line := range strings.Split(block, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), key) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("PROFILE_ORDER in app.js has no entry for schema profile %q (viewer/schema drift)", profile)
		}
	}
}
