package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"quorum/internal/core"

	"github.com/spf13/cobra"
)

var memoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Manage centralized Quorum memory",
}

var memorySearchType string
var memorySearchAllProjects bool
var memorySearchLimit int
var memorySearchJSON bool

var memorySearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search centralized Quorum memory",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectID := ""
		if !memorySearchAllProjects {
			cfg, err := core.ReadQuorumConfig()
			if err != nil {
				fmt.Fprintf(os.Stderr, "[!] .quorumrc is required unless --all-projects is set: %v\n", err)
				os.Exit(1)
			}
			projectID = cfg.ProjectID
		}

		db, err := core.OpenMemoryDB("")
		if err != nil {
			fmt.Fprintf(os.Stderr, "[!] Error opening memory database: %v\n", err)
			os.Exit(1)
		}
		defer db.Close()

		results, err := core.SearchMemoryEntries(db, core.MemorySearchOptions{
			Query:       args[0],
			Type:        memorySearchType,
			ProjectID:   projectID,
			AllProjects: memorySearchAllProjects,
			Limit:       memorySearchLimit,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "[!] Error searching memory: %v\n", err)
			os.Exit(1)
		}

		if memorySearchJSON {
			payload := map[string]any{"count": len(results), "results": results}
			b, err := json.MarshalIndent(payload, "", "  ")
			if err != nil {
				fmt.Fprintf(os.Stderr, "[!] Error rendering JSON: %v\n", err)
				os.Exit(1)
			}
			fmt.Println(string(b))
			return
		}

		if len(results) == 0 {
			fmt.Println("No memory entries found.")
			return
		}
		fmt.Printf("%-14s %-10s %-10s %s\n", "PROJECT", "TYPE", "ID", "TITLE")
		for _, r := range results {
			fmt.Printf("%-14s %-10s %-10s %s\n", r.ProjectID, r.Type, r.ID, r.Title)
		}
	},
}

func init() {
	memorySearchCmd.Flags().StringVar(&memorySearchType, "type", "", "filter by memory type: decision, pattern, or lesson")
	memorySearchCmd.Flags().BoolVar(&memorySearchAllProjects, "all-projects", false, "search all projects in the central database")
	memorySearchCmd.Flags().IntVar(&memorySearchLimit, "limit", core.DefaultMemorySearchLimit, "maximum number of results")
	memorySearchCmd.Flags().BoolVar(&memorySearchJSON, "json", false, "emit machine-readable JSON")
	memoryCmd.AddCommand(memorySearchCmd)
	rootCmd.AddCommand(memoryCmd)
}
