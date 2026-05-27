package core

import (
	"path/filepath"
	"testing"
)

func TestEnrichBlueprintWithRetrievers(t *testing.T) {
	tempDir := t.TempDir()

	bp := map[string]any{
		"affected_files": []string{"a.go"},
		"dependencies":   []string{"b.go"},
	}

	enriched := EnrichBlueprintWithRetrievers(bp, tempDir, 2)
	
	if a, ok := enriched["affected_files"].([]string); !ok || len(a) != 1 || a[0] != filepath.ToSlash(filepath.Join("a.go")) {
		if len(a) > 0 {
			t.Errorf("expected affected_files [a.go], got %v", a)
		} else {
			t.Errorf("expected affected_files [a.go], got none")
		}
	}
}

func TestProjectRelative(t *testing.T) {
	if rel := projectRelative("/home/user/dev/a.go", "/home/user/dev"); rel != "a.go" {
		t.Errorf("expected a.go, got %q", rel)
	}
	if rel := projectRelative("a.go", "/home/user/dev"); rel != "a.go" {
		t.Errorf("expected a.go, got %q", rel)
	}
}
