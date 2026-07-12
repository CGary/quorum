package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"quorum/internal/core"
)

// contractCheckRequest is the fixed stdin contract for `quorum analyze
// contract-check`: a path to a 02-contract.yaml-shaped file plus
// caller-computed changed_files and diff_stat. This shim owns all
// filesystem I/O (reading and parsing contract_path); core.CheckContract
// itself stays a pure function over the already-decoded contract.
type contractCheckRequest struct {
	ContractPath string        `json:"contract_path"`
	ChangedFiles []string      `json:"changed_files"`
	DiffStat     core.DiffStat `json:"diff_stat"`
}

var analyzeContractCheckCmd = &cobra.Command{
	Use:   "contract-check",
	Short: "Check changed_files/diff_stat against a 02-contract.yaml's touch/forbid/limits rules from stdin JSON",
	Args:  cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		var req contractCheckRequest
		if err := readStdinJSON(&req); err != nil {
			fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
			os.Exit(1)
		}

		raw, err := os.ReadFile(req.ContractPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading contract_path: %v\n", err)
			os.Exit(1)
		}

		var contract core.Contract
		if err := yaml.Unmarshal(raw, &contract); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing contract_path: %v\n", err)
			os.Exit(1)
		}

		result := core.CheckContract(contract, req.ChangedFiles, req.DiffStat)

		if err := printStdoutJSON(result); err != nil {
			fmt.Fprintf(os.Stderr, "Error printing JSON: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	analyzeCmd.AddCommand(analyzeContractCheckCmd)
}
