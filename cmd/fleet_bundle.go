package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"quorum/internal/core"
)

// fleetBundleContractPaths is the minimal shape read out of 02-contract.yaml
// to resolve context_bundle; the contract schema defines no context_bundle
// field, so it is resolved as the union of read and touch (see
// core.ResolveContextBundle).
type fleetBundleContractPaths struct {
	Read  []string `yaml:"read"`
	Touch []string `yaml:"touch"`
}

var fleetBundleMaxBytes int

var fleetBundleCmd = &cobra.Command{
	Use:   "bundle [task_id]",
	Short: "Build a deterministic dispatch context bundle for a task",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		taskID := args[0]

		store, err := core.DefaultTaskStore()
		if err != nil {
			fmt.Println("[!] Error initializing task store:", err)
			os.Exit(1)
		}

		taskDir, err := store.FindTask(taskID, "active")
		if err != nil {
			fmt.Println("[!] Error:", err)
			os.Exit(1)
		}
		if taskDir == nil {
			fmt.Printf("[!] Active task %s not found.\n", taskID)
			os.Exit(1)
		}

		specYAML, err := readFleetTaskArtifact(store, taskDir, "00-spec.yaml")
		if err != nil {
			fmt.Println("[!] Error reading 00-spec.yaml:", err)
			os.Exit(1)
		}
		blueprintYAML, err := readFleetTaskArtifact(store, taskDir, "01-blueprint.yaml")
		if err != nil {
			fmt.Println("[!] Error reading 01-blueprint.yaml:", err)
			os.Exit(1)
		}
		contractYAML, err := readFleetTaskArtifact(store, taskDir, "02-contract.yaml")
		if err != nil {
			fmt.Println("[!] Error reading 02-contract.yaml:", err)
			os.Exit(1)
		}

		var contractPaths fleetBundleContractPaths
		if err := yaml.Unmarshal(contractYAML, &contractPaths); err != nil {
			fmt.Println("[!] Error parsing 02-contract.yaml read/touch:", err)
			os.Exit(1)
		}

		contextBundle := core.ResolveContextBundle(contractPaths.Read, contractPaths.Touch)

		slices, err := resolveFleetBundleSlices(store.ProjectRoot, contextBundle)
		if err != nil {
			fmt.Println("[!] Error resolving context_bundle slices:", err)
			os.Exit(1)
		}

		input := core.BundleInput{
			TaskID:        taskID,
			SpecYAML:      specYAML,
			BlueprintYAML: blueprintYAML,
			ContractYAML:  contractYAML,
			ContextBundle: contextBundle,
			Slices:        slices,
		}

		createdAt := time.Now().UTC().Format(time.RFC3339)
		bundle, err := core.BuildBundle(input, fleetBundleMaxBytes, createdAt)
		if err != nil {
			fmt.Println("[!] Error building bundle:", err)
			os.Exit(1)
		}

		dispatchID := bundle.Manifest.BundleHash[:12]
		dispatchDir := filepath.Join(taskDir.Path, "dispatch", dispatchID)
		if err := os.MkdirAll(dispatchDir, 0755); err != nil {
			fmt.Println("[!] Error creating dispatch directory:", err)
			os.Exit(1)
		}

		promptPath := filepath.Join(dispatchDir, "prompt.md")
		if err := os.WriteFile(promptPath, []byte(bundle.Prompt), 0644); err != nil {
			fmt.Println("[!] Error writing prompt.md:", err)
			os.Exit(1)
		}

		manifestJSON, err := json.MarshalIndent(bundle.Manifest, "", "  ")
		if err != nil {
			fmt.Println("[!] Error marshaling manifest.json:", err)
			os.Exit(1)
		}
		manifestPath := filepath.Join(dispatchDir, "manifest.json")
		if err := os.WriteFile(manifestPath, manifestJSON, 0644); err != nil {
			fmt.Println("[!] Error writing manifest.json:", err)
			os.Exit(1)
		}

		fmt.Printf("[+] Bundle written: %s (bundle_hash=%s)\n", dispatchDir, bundle.Manifest.BundleHash)
	},
}

func init() {
	fleetBundleCmd.Flags().IntVar(&fleetBundleMaxBytes, "max-bytes", core.DefaultFleetBundleMaxBytes, "maximum byte budget for the assembled bundle before deterministic truncation")
	fleetCmd.AddCommand(fleetBundleCmd)
}

func readFleetTaskArtifact(store core.TaskStore, taskDir *core.TaskDirMatch, name string) ([]byte, error) {
	path, err := store.TaskArtifactPath(taskDir, name)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

// resolveFleetBundleSlices reads the whole-file content of every path in
// contextBundle, relative to projectRoot. Each path is first rejected via
// core.ResolveRepoBoundedPath if absolute or resolving outside projectRoot.
// Directories are skipped (a context_bundle entry is a file reference); this
// is not a size-driven truncation and is therefore never recorded as a
// manifest drop.
func resolveFleetBundleSlices(projectRoot string, contextBundle []string) ([]core.BundleSlice, error) {
	slices := make([]core.BundleSlice, 0, len(contextBundle))
	for _, relPath := range contextBundle {
		absPath, err := core.ResolveRepoBoundedPath(projectRoot, relPath)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if info.IsDir() {
			continue
		}
		content, err := os.ReadFile(absPath)
		if err != nil {
			return nil, err
		}
		slices = append(slices, core.BundleSlice{Path: relPath, Content: content})
	}
	return slices, nil
}
