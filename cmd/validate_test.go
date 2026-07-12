package cmd

import (
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSanitizeYAML(t *testing.T) {
	yamlV3Payload := mustUnmarshalYAML(t, `
name: quorum
enabled: true
count: 2
items:
  - label: first
    values:
      - alpha
      - beta
  - 3
empty_map: {}
empty_slice: []
null_value: null
`)

	t.Run("yaml v3 map string any payload remains normalized", func(t *testing.T) {
		if _, ok := yamlV3Payload.(map[string]any); !ok {
			t.Fatalf("yaml.v3 produced %T, want map[string]any", yamlV3Payload)
		}

		want := map[string]any{
			"name":        "quorum",
			"enabled":     true,
			"count":       2,
			"items":       []any{map[string]any{"label": "first", "values": []any{"alpha", "beta"}}, 3},
			"empty_map":   map[string]any{},
			"empty_slice": []any{},
			"null_value":  nil,
		}

		if got := sanitizeYAML(yamlV3Payload); !reflect.DeepEqual(got, want) {
			t.Fatalf("sanitizeYAML() = %#v, want %#v", got, want)
		}
	})

	// yaml.v3 unmarshals generic mappings into map[string]any, so this legacy
	// map[any]any branch is dead code for real YAML parsing and is tested here
	// only with a directly constructed value.
	t.Run("legacy map any any converts recursively", func(t *testing.T) {
		input := map[any]any{
			"name": "quorum",
			7:      []any{map[any]any{"nested": true}},
		}
		want := map[string]any{
			"name": "quorum",
			"7":    []any{map[string]any{"nested": true}},
		}

		got := sanitizeYAML(input)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("sanitizeYAML() = %#v, want %#v", got, want)
		}
		if _, ok := got.(map[string]any); !ok {
			t.Fatalf("sanitizeYAML() type = %T, want map[string]any", got)
		}
	})

	for _, tc := range []struct {
		name  string
		input any
		want  any
	}{
		{name: "slice", input: []any{"leaf", []any{1, true}}, want: []any{"leaf", []any{1, true}}},
		{name: "string scalar", input: "leaf", want: "leaf"},
		{name: "int scalar", input: 42, want: 42},
		{name: "bool scalar", input: false, want: false},
		{name: "nil scalar", input: nil, want: nil},
		{name: "empty legacy map", input: map[any]any{}, want: map[string]any{}},
		{name: "empty slice", input: []any{}, want: []any{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeYAML(tc.input); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("sanitizeYAML() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func mustUnmarshalYAML(t *testing.T, data string) any {
	t.Helper()

	var payload any
	if err := yaml.Unmarshal([]byte(data), &payload); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}
	return payload
}
