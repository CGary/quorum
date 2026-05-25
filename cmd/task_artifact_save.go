package cmd

import (
	"fmt"
	"io"
	"os"
	"quorum/internal/core"

	"github.com/spf13/cobra"
)

var artifactSaveCmd = &cobra.Command{
	Use:   "artifact-save [task_id] [artifact_path]",
	Short: "Save artifact from stdin",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		taskID := args[0]
		artifactPath := args[1]
		
		taskDir, err := core.FindTaskDir(taskID, nil)
		if err != nil {
			fmt.Println("[!] Error:", err)
			os.Exit(1)
		}
		if taskDir == nil {
			fmt.Printf("[!] Task %s not found.\n", taskID)
			os.Exit(1)
		}
		
		raw, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Println("[!] Error reading stdin:", err)
			os.Exit(1)
		}
		
		destPath := taskDir.Path + "/" + artifactPath
		
		// Write to temporary file to parse
		tmpPath := destPath + ".tmp"
		if err := os.WriteFile(tmpPath, raw, 0644); err != nil {
			fmt.Println("[!] Error:", err)
			os.Exit(1)
		}
		defer os.Remove(tmpPath)
		
		payload, err := core.LoadArtifactPayload(tmpPath)
		if err != nil {
			fmt.Printf("[!] artifact=%s; field=$; reason=payload parse failed: %v\n", destPath, err)
			os.Exit(1)
		}
		
		if _, err := core.SaveArtifact(destPath, payload); err != nil {
			fmt.Printf("[!] %v\n", err)
			os.Exit(1)
		}
		
		fmt.Printf("[+] Saved artifact: %s\n", destPath)
	},
}

func init() {
	taskCmd.AddCommand(artifactSaveCmd)
}
