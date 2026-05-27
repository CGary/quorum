package core

import (
	"strings"
	"testing"
)

func TestRenderAsciiDag(t *testing.T) {
	decomp := []any{
		map[string]any{"child_id": "F-01-a"},
		map[string]any{"child_id": "F-01-b", "depends_on": []any{"F-01-a"}},
		map[string]any{"child_id": "F-01-c", "depends_on": []any{"F-01-a"}},
		map[string]any{"child_id": "F-01-d", "depends_on": []any{"F-01-b", "F-01-c"}},
	}

	out := RenderAsciiDag(decomp)
	expectedLines := []string{
		"Decomposition DAG:",
		"  order: L0 -> L1 -> L2",
		"  L0        L1        L2",
		"  [F-01-a]  [F-01-b]  [F-01-d]",
		"            [F-01-c]",
		"  edges:",
		"    F-01-a -> F-01-b",
		"    F-01-a -> F-01-c",
		"    F-01-b -> F-01-d",
		"    F-01-c -> F-01-d",
	}

	expected := strings.Join(expectedLines, "\n")
	if out != expected {
		t.Errorf("expected:\n%s\n\ngot:\n%s", expected, out)
	}
}

func TestRenderAsciiDag_Empty(t *testing.T) {
	if out := RenderAsciiDag(nil); out != "" {
		t.Errorf("expected empty string, got %q", out)
	}
}

func TestRenderAsciiDag_MissingDepsIgnoredForLevels(t *testing.T) {
	decomp := []any{
		map[string]any{"child_id": "A", "depends_on": []any{"UNKNOWN"}},
		map[string]any{"child_id": "B", "depends_on": []any{"A"}},
	}
	out := RenderAsciiDag(decomp)
	if !strings.Contains(out, "order: L0 -> L1") {
		t.Errorf("expected L0 -> L1, got:\n%s", out)
	}
	if !strings.Contains(out, "UNKNOWN -> A") {
		t.Errorf("expected edge from UNKNOWN, got:\n%s", out)
	}
}
