package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"quorum/internal/core"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// validateSchemaAliases is the closed whitelist for `validate --schema <name>`.
// It lets a temp-located artifact (e.g. .tmp/draft.yaml) be validated without
// forcing a rigid directory layout, while preventing arbitrary schema injection:
// only known artifact schemas can be selected.
var validateSchemaAliases = map[string]string{
	"spec":               "spec.schema.json",
	"blueprint":          "blueprint.schema.json",
	"contract":           "contract.schema.json",
	"implementation-log": "implementation-log.schema.json",
	"validation":         "validation.schema.json",
	"review":             "review.schema.json",
	"trace":              "trace.schema.json",
	"memory":             "memory.schema.json",
	"feedback":           "feedback.schema.json",
	"report":             "report.schema.json",
}

func newValidateCmd() *cobra.Command {
	var schemaOverride string
	cmd := &cobra.Command{
		Use:   "validate <artifact-path>",
		Short: "Validate an artifact against its schema",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			path := args[0]
			data, err := os.ReadFile(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error reading file: %v\n", err)
				os.Exit(1)
			}

			var payload any
			ext := filepath.Ext(path)
			if ext == ".yaml" || ext == ".yml" {
				if err := yaml.Unmarshal(data, &payload); err != nil {
					fmt.Fprintf(os.Stderr, "error parsing yaml: %v\n", err)
					os.Exit(1)
				}
			} else if ext == ".json" {
				if err := json.Unmarshal(data, &payload); err != nil {
					fmt.Fprintf(os.Stderr, "error parsing json: %v\n", err)
					os.Exit(1)
				}
			}

			payload = sanitizeYAML(payload)

			fail := func(err error) {
				if ve, ok := err.(core.ArtifactValidationError); ok {
					fmt.Fprintln(os.Stderr, ve.Message)
				} else {
					fmt.Fprintln(os.Stderr, err.Error())
				}
				os.Exit(1)
			}

			// --schema overrides path-based detection so temp files validate
			// without a rigid directory layout. The alias whitelist keeps the
			// override from selecting an arbitrary schema.
			if schemaOverride != "" {
				schemaName, ok := validateSchemaAliases[schemaOverride]
				if !ok {
					names := make([]string, 0, len(validateSchemaAliases))
					for k := range validateSchemaAliases {
						names = append(names, k)
					}
					sort.Strings(names)
					fmt.Fprintf(os.Stderr, "error: unknown schema %q; valid values: %s\n", schemaOverride, strings.Join(names, ", "))
					os.Exit(1)
				}
				if err := core.ValidateAgainstSchema(schemaName, path, payload); err != nil {
					fail(err)
				}
				return
			}

			if err := core.ValidateArtifact(path, payload); err != nil {
				fail(err)
			}
		},
	}
	cmd.Flags().StringVar(&schemaOverride, "schema", "", "Validate against this schema by name (e.g. report) instead of path-based detection")
	return cmd
}

func sanitizeYAML(v any) any {
	switch x := v.(type) {
	case map[any]any:
		m := map[string]any{}
		for k, val := range x {
			m[fmt.Sprintf("%v", k)] = sanitizeYAML(val)
		}
		return m
	case []any:
		for i, val := range x {
			x[i] = sanitizeYAML(val)
		}
		return x
	}
	return v
}

func init() {
	rootCmd.AddCommand(newValidateCmd())
}
