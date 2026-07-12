package core

import (
	"strings"
	"testing"
)

func fleetBundleSampleInput() BundleInput {
	contextBundle := ResolveContextBundle(
		[]string{"cmd/task.go", "internal/core/task_manager.go"},
		[]string{"internal/core/task_manager.go", "cmd/fleet_bundle.go"},
	)
	return BundleInput{
		TaskID:        "FEAT-999",
		SpecYAML:      []byte("task_id: FEAT-999\ngoal: sample goal\n"),
		BlueprintYAML: []byte("task_id: FEAT-999\nsummary: sample blueprint\n"),
		ContractYAML:  []byte("task_id: FEAT-999\ntouch:\n  - cmd/fleet_bundle.go\n"),
		ContextBundle: contextBundle,
		Slices: []BundleSlice{
			{Path: "cmd/task.go", Content: []byte("package cmd\n")},
			{Path: "internal/core/task_manager.go", Content: []byte("package core\n")},
			{Path: "cmd/fleet_bundle.go", Content: []byte("package cmd\n")},
		},
	}
}

// AC-1: running BuildBundle twice on identical inputs yields the same
// bundle_hash; file ordering and created_at are excluded from the hash.
func TestBuildBundleHashStability(t *testing.T) {
	input := fleetBundleSampleInput()

	first, err := BuildBundle(input, 0, "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("BuildBundle first run: %v", err)
	}

	// Shuffle slice order and use a different created_at; the hash must not move.
	shuffled := fleetBundleSampleInput()
	shuffled.Slices = []BundleSlice{
		shuffled.Slices[2], shuffled.Slices[0], shuffled.Slices[1],
	}
	second, err := BuildBundle(shuffled, 0, "2099-12-31T23:59:59Z")
	if err != nil {
		t.Fatalf("BuildBundle second run: %v", err)
	}

	if first.Manifest.BundleHash != second.Manifest.BundleHash {
		t.Fatalf("bundle_hash not stable: %q vs %q", first.Manifest.BundleHash, second.Manifest.BundleHash)
	}
	if first.Manifest.CreatedAt == second.Manifest.CreatedAt {
		t.Fatalf("expected created_at to differ between runs, both are %q", first.Manifest.CreatedAt)
	}
}

// AC-2: every file path present in the bundle is a member of context_bundle;
// a negative test confirms a path outside it never leaks in.
func TestBuildBundleRejectsSliceOutsideContextBundle(t *testing.T) {
	input := fleetBundleSampleInput()
	input.Slices = append(input.Slices, BundleSlice{
		Path:    "quorum.md",
		Content: []byte("leaked content"),
	})

	_, err := BuildBundle(input, 0, "2026-01-01T00:00:00Z")
	if err == nil {
		t.Fatal("expected BuildBundle to reject a slice outside context_bundle, got nil error")
	}
}

func TestBuildBundleMembershipHolds(t *testing.T) {
	input := fleetBundleSampleInput()
	bundle, err := BuildBundle(input, 0, "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("BuildBundle: %v", err)
	}

	member := make(map[string]bool, len(input.ContextBundle))
	for _, p := range input.ContextBundle {
		member[p] = true
	}
	for _, f := range bundle.Manifest.Files {
		if !member[f] {
			t.Fatalf("manifest file %q is not a member of context_bundle", f)
		}
	}
	if strings.Contains(bundle.Prompt, "quorum.md") {
		t.Fatal("bundle prompt leaked a path outside context_bundle")
	}
}

// AC-3: manifest.json contains task_id, bundle_hash, files, protocol_version,
// created_at, and protocol_version equals the fixed template version.
func TestBuildBundleManifestShape(t *testing.T) {
	input := fleetBundleSampleInput()
	bundle, err := BuildBundle(input, 0, "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("BuildBundle: %v", err)
	}

	if bundle.Manifest.TaskID != "FEAT-999" {
		t.Fatalf("task_id = %q, want FEAT-999", bundle.Manifest.TaskID)
	}
	if bundle.Manifest.BundleHash == "" {
		t.Fatal("bundle_hash is empty")
	}
	if len(bundle.Manifest.Files) == 0 {
		t.Fatal("files is empty")
	}
	if bundle.Manifest.ProtocolVersion != fleetBundleProtocolVersion {
		t.Fatalf("protocol_version = %q, want %q", bundle.Manifest.ProtocolVersion, fleetBundleProtocolVersion)
	}
	if bundle.Manifest.CreatedAt != "2026-01-01T00:00:00Z" {
		t.Fatalf("created_at = %q, want 2026-01-01T00:00:00Z", bundle.Manifest.CreatedAt)
	}
	if !strings.Contains(bundle.Prompt, "BLOCKED: missing_file=") {
		t.Fatal("bundle prompt is missing the standardized BLOCKED signal template")
	}
	if !strings.Contains(bundle.Prompt, "NOTES:") {
		t.Fatal("bundle prompt is missing the delimited NOTES block")
	}
}

// AC-4: when combined content exceeds the byte budget, truncation drops code
// slices first, then blueprint, then spec, then contract last, and every
// dropped item is recorded; nothing is dropped silently.
func TestBuildBundleDeterministicTruncation(t *testing.T) {
	input := fleetBundleSampleInput()

	full, err := BuildBundle(input, 0, "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("BuildBundle full: %v", err)
	}

	// Force truncation down to a budget that fits only the protocol header
	// and the contract, forcing every slice, the blueprint, and the spec to
	// drop, while the contract survives.
	tiny, err := BuildBundle(input, 1050, "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("BuildBundle tiny: %v", err)
	}

	if len(tiny.Prompt) >= len(full.Prompt) {
		t.Fatalf("expected truncated prompt shorter than full prompt: tiny=%d full=%d", len(tiny.Prompt), len(full.Prompt))
	}
	if len(tiny.Manifest.Dropped) == 0 {
		t.Fatal("expected truncation to record dropped items, got none")
	}

	var sawCodeSlice, sawBlueprint, sawSpec bool
	codeSliceIdx, blueprintIdx, specIdx := -1, -1, -1
	for i, d := range tiny.Manifest.Dropped {
		switch d.Component {
		case "code_slice":
			sawCodeSlice = true
			if codeSliceIdx == -1 {
				codeSliceIdx = i
			}
		case "blueprint":
			sawBlueprint = true
			blueprintIdx = i
		case "spec":
			sawSpec = true
			specIdx = i
		case "contract":
			t.Fatal("contract must be the last-resort drop; it should not be dropped under this budget")
		}
	}
	if !sawCodeSlice || !sawBlueprint || !sawSpec {
		t.Fatalf("expected code_slice, blueprint, and spec drops, got %+v", tiny.Manifest.Dropped)
	}
	if codeSliceIdx > blueprintIdx {
		t.Fatalf("expected code slices to drop before blueprint, got order %+v", tiny.Manifest.Dropped)
	}
	if blueprintIdx > specIdx {
		t.Fatalf("expected blueprint to drop before spec, got order %+v", tiny.Manifest.Dropped)
	}
	if len(tiny.Manifest.Files) != 0 {
		t.Fatalf("expected all code slices dropped, got files=%v", tiny.Manifest.Files)
	}

	for _, d := range tiny.Manifest.Dropped {
		if d.Reason == "" {
			t.Fatalf("dropped item %+v is missing a reason", d)
		}
	}
}

// Negative test for fleet-bundle-path-traversal-guard: a context_bundle
// member path with ".." segments or an absolute path must never resolve to
// a location outside projectRoot.
func TestResolveRepoBoundedPathRejectsEscape(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		name    string
		relPath string
		wantErr bool
	}{
		{"relative within root", "cmd/fleet_bundle.go", false},
		{"traversal escapes root", "../../etc/passwd", true},
		{"absolute path rejected", "/etc/passwd", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ResolveRepoBoundedPath(root, tc.relPath)
			if tc.wantErr && err == nil {
				t.Fatalf("ResolveRepoBoundedPath(%q) = nil error, want rejection", tc.relPath)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("ResolveRepoBoundedPath(%q) unexpected error: %v", tc.relPath, err)
			}
		})
	}
}

func TestResolveContextBundleDedupesAndSorts(t *testing.T) {
	got := ResolveContextBundle(
		[]string{"b.go", "a.go", ""},
		[]string{"a.go", "c.go"},
	)
	want := []string{"a.go", "b.go", "c.go"}
	if len(got) != len(want) {
		t.Fatalf("ResolveContextBundle length = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ResolveContextBundle = %v, want %v", got, want)
		}
	}
}
