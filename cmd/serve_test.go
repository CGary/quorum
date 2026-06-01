package cmd

import (
	"testing"
)

func TestServeCmdFlags(t *testing.T) {
	f := serveCmd.Flags().Lookup("port")
	if f == nil {
		t.Fatal("Expected --port flag to exist")
	}
	if f.DefValue != "8080" {
		t.Errorf("Expected default port to be 8080, got %s", f.DefValue)
	}
}
