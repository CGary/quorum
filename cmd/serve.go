package cmd

import (
	"log"

	"github.com/spf13/cobra"
	"quorum/internal/server"
)

var port int

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start a read-only local server for projects and reports",
	Long:  `Start a local API server on 127.0.0.1 to serve JSON data about projects and their reports.`,
	Run: func(cmd *cobra.Command, args []string) {
		srv, err := server.NewServer()
		if err != nil {
			log.Fatalf("Failed to initialize server: %v", err)
		}
		if err := srv.Start(port); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	},
}

func init() {
	serveCmd.Flags().IntVarP(&port, "port", "p", 8080, "Port to run the server on")
	rootCmd.AddCommand(serveCmd)
}
