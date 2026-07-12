package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// fleetBundleGoldenFixtureDir points at the synthetic golden fixture task,
// resolved relative to this test file so `go test ./...` works regardless of
// the caller's working directory.
const fleetBundleGoldenFixtureDir = "testdata/fleet_bundle/synthetic_task"

// fleetBundleGoldenCreatedAt is a fixed value used for both the actual and
// the committed golden manifest, so the comparison is byte-for-byte while
// still honoring "created_at is excluded from the hash computation."
const fleetBundleGoldenCreatedAt = "2026-01-01T00:00:00Z"

type fleetBundleGoldenContractPaths struct {
	Read  []string `yaml:"read"`
	Touch []string `yaml:"touch"`
}

func loadFleetBundleGoldenInput(t *testing.T) BundleInput {
	t.Helper()

	specYAML, err := os.ReadFile(filepath.Join(fleetBundleGoldenFixtureDir, "00-spec.yaml"))
	if err != nil {
		t.Fatalf("failed to read fixture spec: %v", err)
	}
	blueprintYAML, err := os.ReadFile(filepath.Join(fleetBundleGoldenFixtureDir, "01-blueprint.yaml"))
	if err != nil {
		t.Fatalf("failed to read fixture blueprint: %v", err)
	}
	contractYAML, err := os.ReadFile(filepath.Join(fleetBundleGoldenFixtureDir, "02-contract.yaml"))
	if err != nil {
		t.Fatalf("failed to read fixture contract: %v", err)
	}

	var contractPaths fleetBundleGoldenContractPaths
	if err := yaml.Unmarshal(contractYAML, &contractPaths); err != nil {
		t.Fatalf("failed to parse fixture contract read/touch: %v", err)
	}

	contextBundle := ResolveContextBundle(contractPaths.Read, contractPaths.Touch)

	slices := make([]BundleSlice, 0, len(contextBundle))
	for _, relPath := range contextBundle {
		content, err := os.ReadFile(filepath.Join(fleetBundleGoldenFixtureDir, relPath))
		if err != nil {
			t.Fatalf("failed to read fixture context_bundle file %q: %v", relPath, err)
		}
		slices = append(slices, BundleSlice{Path: relPath, Content: content})
	}

	return BundleInput{
		TaskID:        "GOLD-001",
		SpecYAML:      specYAML,
		BlueprintYAML: blueprintYAML,
		ContractYAML:  contractYAML,
		ContextBundle: contextBundle,
		Slices:        slices,
	}
}

// fleetBundleGoldenSeparator delimits the prompt section from the manifest
// section inside the single committed golden fixture file. Keeping both in
// one file (instead of two) keeps the fixture footprint inside the
// contract's max_files_changed budget.
const fleetBundleGoldenSeparator = "\n===MANIFEST===\n"

// TestBuildBundleGoldenMaster exercises the full bundle output for a
// synthetic task fixture (AC-5): the produced prompt and manifest must match
// the committed golden output byte for byte, excluding created_at (which is
// pinned to a fixed value here on both sides of the comparison).
func TestBuildBundleGoldenMaster(t *testing.T) {
	input := loadFleetBundleGoldenInput(t)

	first, err := BuildBundle(input, 0, fleetBundleGoldenCreatedAt)
	if err != nil {
		t.Fatalf("BuildBundle first run: %v", err)
	}
	second, err := BuildBundle(input, 0, fleetBundleGoldenCreatedAt)
	if err != nil {
		t.Fatalf("BuildBundle second run: %v", err)
	}
	if first.Manifest.BundleHash != second.Manifest.BundleHash {
		t.Fatalf("bundle_hash not stable across identical runs: %q vs %q", first.Manifest.BundleHash, second.Manifest.BundleHash)
	}
	if first.Prompt != second.Prompt {
		t.Fatal("prompt not stable across identical runs")
	}

	gotManifest, err := json.MarshalIndent(first.Manifest, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}
	got := first.Prompt + fleetBundleGoldenSeparator + string(gotManifest) + "\n"

	want, err := os.ReadFile(filepath.Join(fleetBundleGoldenFixtureDir, "golden", "output.golden"))
	if err != nil {
		t.Fatalf("failed to read golden output: %v", err)
	}
	if got != string(want) {
		t.Fatalf("bundle output does not match golden fixture.\n--- got ---\n%s\n--- want ---\n%s", got, string(want))
	}

	member := make(map[string]bool, len(input.ContextBundle))
	for _, p := range input.ContextBundle {
		member[p] = true
	}
	for _, f := range first.Manifest.Files {
		if !member[f] {
			t.Fatalf("golden manifest file %q leaked outside context_bundle", f)
		}
	}
}
