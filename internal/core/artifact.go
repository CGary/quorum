package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func EnsureTraceAppendOnly(path string, existingPayload, newPayload any) error {
	existing, next := attempts(existingPayload), attempts(newPayload)
	if len(next) < len(existing) {
		return ArtifactValidationError{fmt.Sprintf("artifact=%s; field=$.attempts; reason=append-only trace cannot remove existing attempts", path)}
	}
	for i := range existing {
		if !sameJSON(existing[i], next[i]) {
			return ArtifactValidationError{fmt.Sprintf("artifact=%s; field=$.attempts; reason=append-only trace cannot reorder or mutate existing attempts", path)}
		}
	}

	// events[] gets the same append-only protection as attempts[]. An existing
	// payload with no "events" key at all (older traces predating this field)
	// is treated as an empty baseline, not an error.
	existingEvents, nextEvents := events(existingPayload), events(newPayload)
	if len(nextEvents) < len(existingEvents) {
		return ArtifactValidationError{fmt.Sprintf("artifact=%s; field=$.events; reason=append-only trace cannot remove existing events", path)}
	}
	for i := range existingEvents {
		if !sameJSON(existingEvents[i], nextEvents[i]) {
			return ArtifactValidationError{fmt.Sprintf("artifact=%s; field=$.events; reason=append-only trace cannot reorder or mutate existing events", path)}
		}
	}
	return nil
}

func attempts(payload any) []any {
	if obj, ok := payload.(map[string]any); ok {
		if items, ok := asSlice(obj["attempts"]); ok {
			return items
		}
	}
	return nil
}

func events(payload any) []any {
	if obj, ok := payload.(map[string]any); ok {
		if items, ok := asSlice(obj["events"]); ok {
			return items
		}
	}
	return nil
}

func sameJSON(a, b any) bool {
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	return string(aj) == string(bj)
}

func ParseArtifactPayload(path string, raw []byte) (any, error) {
	var payload any
	if filepath.Ext(path) == ".json" {
		if err := json.Unmarshal(raw, &payload); err != nil {
			return nil, err
		}
	} else {
		if err := yaml.Unmarshal(raw, &payload); err != nil {
			return nil, err
		}
	}
	return payload, nil
}

func LoadArtifactPayload(path string) (any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseArtifactPayload(path, raw)
}

func DumpArtifactPayload(path string, payload any) error {
	var b []byte
	var err error
	if filepath.Ext(path) == ".json" {
		b, err = json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		b = append(b, '\n')
	} else {
		b, err = yaml.Marshal(payload)
		if err != nil {
			return err
		}
	}
	return os.WriteFile(path, b, 0644)
}

func SaveArtifact(artifactPath string, payload any) (string, error) {
	var existingPayload any
	if _, err := os.Stat(artifactPath); err == nil {
		existingPayload, _ = LoadArtifactPayload(artifactPath)
	}

	if filepath.Base(artifactPath) == "07-trace.json" && existingPayload != nil {
		if err := EnsureTraceAppendOnly(artifactPath, existingPayload, payload); err != nil {
			return "", err
		}
	}

	if err := ValidateArtifact(artifactPath, payload); err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(artifactPath), 0755); err != nil {
		return "", err
	}
	if err := DumpArtifactPayload(artifactPath, payload); err != nil {
		return "", err
	}
	return artifactPath, nil
}
