package core

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// fleetPreflightFixtureDir points at internal/core/testdata/fleet_preflight,
// resolved relative to this test file so `go test ./...` works regardless of
// the caller's working directory. These fixtures are self-contained and never
// reference .agents/fleet/agents.yaml or .agents/schemas/agents.schema.json,
// because FLEET-002-a (which will introduce those real files) has not merged
// yet.
const fleetPreflightFixtureDir = "testdata/fleet_preflight"

type fleetPreflightLevelsConfig struct {
	Levels map[string]struct {
		Primary string `yaml:"primary"`
	} `yaml:"levels"`
}

func readFleetFixtureFile(t *testing.T, caseName, fileName string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fleetPreflightFixtureDir, caseName, fileName))
	if err != nil {
		t.Fatalf("failed to read fixture %s/%s: %v", caseName, fileName, err)
	}
	return data
}

// fleetLevelModelsFromConfig extracts every level's primary model, in
// ascending level-key order, mirroring how a caller would flatten
// config.yaml.levels before invoking RunFleetPreflight.
func fleetLevelModelsFromConfig(t *testing.T, raw []byte) []string {
	t.Helper()
	var cfg fleetPreflightLevelsConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("failed to parse fixture config.yaml: %v", err)
	}
	keys := make([]string, 0, len(cfg.Levels))
	for k := range cfg.Levels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	models := make([]string, 0, len(keys))
	for _, k := range keys {
		models = append(models, cfg.Levels[k].Primary)
	}
	return models
}

func TestRunFleetPreflightValidJoin(t *testing.T) {
	agentsYAML := readFleetFixtureFile(t, "valid_join", "agents.yaml")
	schemaJSON := readFleetFixtureFile(t, "valid_join", "agents.schema.json")
	configYAML := readFleetFixtureFile(t, "valid_join", "config.yaml")
	levelModels := fleetLevelModelsFromConfig(t, configYAML)

	result, err := RunFleetPreflight(agentsYAML, schemaJSON, levelModels)
	if err != nil {
		t.Fatalf("RunFleetPreflight returned unexpected error: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected zero errors, got %v", result.Errors)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("expected zero warnings, got %v", result.Warnings)
	}
}

func TestRunFleetPreflightModelNoTransport(t *testing.T) {
	agentsYAML := readFleetFixtureFile(t, "model_no_transport", "agents.yaml")
	schemaJSON := readFleetFixtureFile(t, "valid_join", "agents.schema.json")
	configYAML := readFleetFixtureFile(t, "model_no_transport", "config.yaml")
	levelModels := fleetLevelModelsFromConfig(t, configYAML)

	result, err := RunFleetPreflight(agentsYAML, schemaJSON, levelModels)
	if err != nil {
		t.Fatalf("RunFleetPreflight returned unexpected error: %v", err)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected exactly one error, got %v", result.Errors)
	}
	got := result.Errors[0]
	for _, want := range []string{"openai/gpt-6-ultra", "agents.yaml", "no active transport declares this model"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected error %q to mention %q", got, want)
		}
	}
}

func TestRunFleetPreflightModelDuplicated(t *testing.T) {
	agentsYAML := readFleetFixtureFile(t, "model_duplicated", "agents.yaml")
	schemaJSON := readFleetFixtureFile(t, "valid_join", "agents.schema.json")
	configYAML := readFleetFixtureFile(t, "model_duplicated", "config.yaml")
	levelModels := fleetLevelModelsFromConfig(t, configYAML)

	result, err := RunFleetPreflight(agentsYAML, schemaJSON, levelModels)
	if err != nil {
		t.Fatalf("RunFleetPreflight returned unexpected error: %v", err)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected exactly one error, got %v", result.Errors)
	}
	got := result.Errors[0]
	for _, want := range []string{"openai/gpt-5.5-medium", "codex", "codex-secondary"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected ambiguity error %q to mention %q", got, want)
		}
	}
}

func TestRunFleetPreflightUnusedTransport(t *testing.T) {
	agentsYAML := readFleetFixtureFile(t, "unused_transport", "agents.yaml")
	schemaJSON := readFleetFixtureFile(t, "valid_join", "agents.schema.json")
	configYAML := readFleetFixtureFile(t, "unused_transport", "config.yaml")
	levelModels := fleetLevelModelsFromConfig(t, configYAML)

	result, err := RunFleetPreflight(agentsYAML, schemaJSON, levelModels)
	if err != nil {
		t.Fatalf("RunFleetPreflight returned unexpected error: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected zero errors, got %v", result.Errors)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("expected exactly one warning, got %v", result.Warnings)
	}
	if !strings.Contains(result.Warnings[0], "agy") {
		t.Errorf("expected warning %q to mention unused transport %q", result.Warnings[0], "agy")
	}
}

func TestRunFleetPreflightInvalidSchema(t *testing.T) {
	agentsYAML := readFleetFixtureFile(t, "invalid_schema", "agents.yaml")
	schemaJSON := readFleetFixtureFile(t, "valid_join", "agents.schema.json")

	result, err := RunFleetPreflight(agentsYAML, schemaJSON, []string{"openai/gpt-5.5-medium"})
	if err != nil {
		t.Fatalf("RunFleetPreflight returned unexpected error: %v", err)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected exactly one schema error and no join attempted, got errors=%v warnings=%v", result.Errors, result.Warnings)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("expected zero warnings when schema validation short-circuits, got %v", result.Warnings)
	}
	if !strings.Contains(result.Errors[0], "schema") {
		t.Errorf("expected schema error %q to mention schema validation", result.Errors[0])
	}
}

func TestRunFleetPreflightInactiveExcluded(t *testing.T) {
	agentsYAML := readFleetFixtureFile(t, "inactive_excluded", "agents.yaml")
	schemaJSON := readFleetFixtureFile(t, "valid_join", "agents.schema.json")
	configYAML := readFleetFixtureFile(t, "inactive_excluded", "config.yaml")
	levelModels := fleetLevelModelsFromConfig(t, configYAML)

	result, err := RunFleetPreflight(agentsYAML, schemaJSON, levelModels)
	if err != nil {
		t.Fatalf("RunFleetPreflight returned unexpected error: %v", err)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected exactly one error (inactive transport must not satisfy the join), got %v", result.Errors)
	}
	if !strings.Contains(result.Errors[0], "anthropic/claude-opus-4-7") {
		t.Errorf("expected error %q to mention the model only declared by the inactive transport", result.Errors[0])
	}
	if len(result.Warnings) != 0 {
		t.Errorf("expected zero warnings: the inactive transport must never warn as unused, got %v", result.Warnings)
	}
}
