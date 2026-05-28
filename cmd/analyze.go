package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

var analyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Analytical helpers for Quorum tasks",
}

func init() {
	rootCmd.AddCommand(analyzeCmd)
}

func readStdinJSON(target any) error {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read stdin: %w", err)
	}
	if len(data) == 0 {
		return fmt.Errorf("empty stdin")
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("failed to unmarshal stdin: %w", err)
	}
	return nil
}

func printStdoutJSON(payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal stdout: %w", err)
	}
	fmt.Println(string(data))
	return nil
}
