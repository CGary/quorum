//go:build sqlite_fts5 && sqlite_vec

package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestPrintTopLevelHelp(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printTopLevelHelp()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)

	if !strings.Contains(buf.String(), "Usage: hsme-cli <subcommand>") {
		t.Errorf("top level help missing usage, got: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "Subcommands:") {
		t.Errorf("top level help missing subcommands, got: %s", buf.String())
	}
}

func TestRunHelp(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		contains string
	}{
		{
			name:     "no args",
			args:     []string{},
			contains: "HSME CLI",
		},
		{
			name:     "store help",
			args:     []string{"store"},
			contains: "hsme-cli store",
		},
		{
			name:     "search-fuzzy help",
			args:     []string{"search-fuzzy"},
			contains: "hsme-cli search-fuzzy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			runHelp(tt.args)

			w.Close()
			os.Stdout = old

			var buf bytes.Buffer
			io.Copy(&buf, r)

			if !strings.Contains(buf.String(), tt.contains) {
				t.Errorf("help for %v missing %q, got: %s", tt.args, tt.contains, buf.String())
			}
		})
	}
}
