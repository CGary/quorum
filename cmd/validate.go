package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"quorum/internal/core"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newValidateCmd() *cobra.Command {
	return &cobra.Command{
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

			if err := core.ValidateArtifact(path, payload); err != nil {
				if ve, ok := err.(core.ArtifactValidationError); ok {
					fmt.Fprintln(os.Stderr, ve.Message)
				} else {
					fmt.Fprintln(os.Stderr, err.Error())
				}
				os.Exit(1)
			}
		},
	}
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
