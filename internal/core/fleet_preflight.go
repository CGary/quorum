package core

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

// FleetPreflightResult reports the outcome of schema-validating agents.yaml
// and joining config.yaml.levels model names against its active transports.
type FleetPreflightResult struct {
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

// fleetPreflightSchemaURL is an internal identifier used only to register the
// ad-hoc compiled schema resource; it is never resolved against disk or the
// network.
const fleetPreflightSchemaURL = "fleet-preflight://agents.schema.json"

// RunFleetPreflight schema-validates agentsYAML against schemaJSON, then
// joins every entry of levelModels against the models declared by
// active:true transports in agentsYAML.
//
// It is a pure transform over the supplied bytes and model list: no
// filesystem or network access happens inside this function, and no model
// name is ever hardcoded here. Callers own all I/O (reading agents.yaml,
// agents.schema.json, and config.yaml.levels from disk).
//
// If agentsYAML fails schema validation, RunFleetPreflight returns
// immediately with a single error and the join is never attempted.
// Otherwise, for every levelModels entry:
//   - zero matching active transports produces a noisy error naming the
//     model, the file, and what is missing;
//   - more than one matching active transport produces an ambiguity error
//     naming the model and every matching transport key;
//   - exactly one match resolves silently.
//
// Finally, any active transport that never resolved a levelModels entry
// produces a warning (never an error); inactive transports never join and
// never warn.
func RunFleetPreflight(agentsYAML []byte, schemaJSON []byte, levelModels []string) (*FleetPreflightResult, error) {
	result := &FleetPreflightResult{Errors: []string{}, Warnings: []string{}}

	schema, err := compileFleetPreflightSchema(schemaJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to compile agents schema: %w", err)
	}

	var agentsPayload any
	if err := yaml.Unmarshal(agentsYAML, &agentsPayload); err != nil {
		return nil, fmt.Errorf("failed to parse agents yaml: %w", err)
	}

	if err := schema.Validate(agentsPayload); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("agents.yaml failed schema validation against agents.schema.json: %v", err))
		return result, nil
	}

	transports, err := activeFleetTransports(agentsPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to read active transports from agents.yaml: %w", err)
	}

	used := make(map[string]bool, len(transports))
	for _, model := range levelModels {
		matches := matchingTransports(transports, model)
		switch len(matches) {
		case 0:
			result.Errors = append(result.Errors, fmt.Sprintf(
				"model=%s; file=agents.yaml; missing=no active transport declares this model", model))
		case 1:
			used[matches[0]] = true
		default:
			sort.Strings(matches)
			result.Errors = append(result.Errors, fmt.Sprintf(
				"model=%s; file=agents.yaml; ambiguous=matched by transports %v", model, matches))
		}
	}

	transportKeys := make([]string, 0, len(transports))
	for key := range transports {
		transportKeys = append(transportKeys, key)
	}
	sort.Strings(transportKeys)
	for _, key := range transportKeys {
		if !used[key] {
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"transport=%s; file=agents.yaml; unused=declared active but referenced by no level model", key))
		}
	}

	return result, nil
}

func compileFleetPreflightSchema(schemaJSON []byte) (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to parse schema json: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(fleetPreflightSchemaURL, doc); err != nil {
		return nil, fmt.Errorf("failed to add schema resource: %w", err)
	}
	return compiler.Compile(fleetPreflightSchemaURL)
}

// activeFleetTransports extracts, from a decoded agents.yaml payload, the
// model sets declared by every transport with active: true.
func activeFleetTransports(agentsPayload any) (map[string]map[string]bool, error) {
	root, ok := agentsPayload.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("agents.yaml root is not a mapping")
	}
	transportsVal, ok := root["transports"]
	if !ok {
		return nil, fmt.Errorf("agents.yaml has no top-level 'transports' key")
	}
	transportsMap, ok := transportsVal.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("agents.yaml 'transports' is not a mapping")
	}

	result := make(map[string]map[string]bool, len(transportsMap))
	for key, val := range transportsMap {
		entry, ok := val.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("transport %q is not a mapping", key)
		}
		active, _ := entry["active"].(bool)
		if !active {
			continue
		}
		models := make(map[string]bool)
		if modelsVal, ok := entry["models"]; ok {
			modelsMap, ok := modelsVal.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("transport %q 'models' is not a mapping", key)
			}
			for modelName := range modelsMap {
				models[modelName] = true
			}
		}
		result[key] = models
	}
	return result, nil
}

func matchingTransports(transports map[string]map[string]bool, model string) []string {
	matches := []string{}
	for key, models := range transports {
		if models[model] {
			matches = append(matches, key)
		}
	}
	return matches
}
