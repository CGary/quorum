package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"quorum/internal/core"
	"strings"

	"github.com/spf13/cobra"
)

var memoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Centralized memory commands",
}

var memorySaveFile string

var memorySaveCmd = &cobra.Command{
	Use:   "save",
	Short: "Persist a memory entry into centralized SQLite storage",
	Run: func(cmd *cobra.Command, args []string) {
		raw, err := readMemorySaveInput(memorySaveFile, os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}

		result, err := core.SaveMemoryEntry(raw)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		writeMemoryJSON(result)
	},
}

var memoryStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Report centralized memory configuration and database status",
	Run: func(cmd *cobra.Command, args []string) {
		result, err := core.MemoryStatus()
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		writeMemoryJSON(result)
	},
}

func readMemorySaveInput(filePath string, stdin *os.File) ([]byte, error) {
	if filePath != "" {
		if stdinHasData(stdin) {
			rawStdin, err := io.ReadAll(stdin)
			if err != nil {
				return nil, fmt.Errorf("failed to read stdin: %w", err)
			}
			if strings.TrimSpace(string(rawStdin)) != "" {
				return nil, fmt.Errorf("provide memory JSON through stdin or --file, not both")
			}
		}
		raw, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read --file %s: %w", filePath, err)
		}
		if strings.TrimSpace(string(raw)) == "" {
			return nil, fmt.Errorf("memory JSON input is required")
		}
		return raw, nil
	}

	if !stdinHasData(stdin) {
		return nil, fmt.Errorf("memory JSON input is required; pipe JSON to stdin or use --file")
	}
	raw, err := io.ReadAll(stdin)
	if err != nil {
		return nil, fmt.Errorf("failed to read stdin: %w", err)
	}
	if strings.TrimSpace(string(raw)) == "" {
		return nil, fmt.Errorf("memory JSON input is required")
	}
	return raw, nil
}

func stdinHasData(stdin *os.File) bool {
	info, err := stdin.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) == 0
}

func writeMemoryJSON(v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to encode JSON output: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(b))
}

func init() {
	memorySaveCmd.Flags().StringVar(&memorySaveFile, "file", "", "Read memory JSON from file")
	memoryCmd.AddCommand(memorySaveCmd)
	memoryCmd.AddCommand(memoryStatusCmd)
	rootCmd.AddCommand(memoryCmd)
}
