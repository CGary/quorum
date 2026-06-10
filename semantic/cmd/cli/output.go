//go:build sqlite_fts5 && sqlite_vec

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/hsme/core/src/core/search"
)

func IsTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func ShouldColor() bool {
	return IsTTY() && os.Getenv("NO_COLOR") == "" && !noColorFlag
}

func FormatJSON(v interface{}) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func FormatText(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func FormatStoreResult(v interface{}) string {
	res, ok := v.(map[string]interface{})
	if !ok {
		return FormatText(v)
	}
	return fmt.Sprintf("Memory stored successfully. ID: %s", Green(fmt.Sprintf("%v", res["memory_id"])))
}

func FormatSearchResults(v interface{}) string {
	res, ok := v.(map[string]interface{})
	if !ok {
		return FormatText(v)
	}
	results, ok := res["results"].([]search.MemorySearchResult)
	if !ok {
		return FormatText(v)
	}

	if len(results) == 0 {
		return "No results found."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d results:\n", len(results)))
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("\n%d. %s (Score: %.4f, Coverage: %s)\n", i+1, Green(fmt.Sprintf("Memory %d", r.MemoryID)), r.Score, r.VectorCoverage))
		if r.IsSuperseded {
			sb.WriteString(Yellow("   [SUPERSEDED]\n"))
		}
		for _, h := range r.Highlights {
			sb.WriteString(fmt.Sprintf("   - %s\n", h.Text))
		}
	}
	return sb.String()
}

func FormatExactResults(v interface{}) string {
	res, ok := v.(map[string]interface{})
	if !ok {
		return FormatText(v)
	}
	results, ok := res["results"].([]search.ExactMatchResult)
	if !ok {
		return FormatText(v)
	}

	if len(results) == 0 {
		return "No exact matches found."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d exact matches:\n", len(results)))
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("\n%d. %s (Chunk %d, Index %d)\n", i+1, Green(fmt.Sprintf("Memory %d", r.MemoryID)), r.ChunkID, r.ChunkIndex))
		sb.WriteString(fmt.Sprintf("   %s\n", r.Text))
	}
	return sb.String()
}

func FormatExploreResult(v interface{}) string {
	res, ok := v.(*search.TraceResult)
	if !ok {
		return FormatText(v)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Exploration for: %s\n", Yellow(res.Entity)))
	sb.WriteString(fmt.Sprintf("Nodes: %d, Edges: %d\n", len(res.Nodes), len(res.Edges)))
	if res.Truncated {
		sb.WriteString(Red("Result set was truncated due to limits.\n"))
	}
	sb.WriteString("\nNodes:\n")
	for _, n := range res.Nodes {
		name := n["name"]
		nodeType := n["type"]
		id := n["id"]
		sb.WriteString(fmt.Sprintf("- [%v] %s (%v)\n", id, Green(fmt.Sprintf("%v", name)), nodeType))
	}

	sb.WriteString("\nConnections:\n")
	for _, e := range res.Edges {
		sb.WriteString(fmt.Sprintf("- Memory %d: %d --(%s)--> %d\n", e.MemoryID, e.SourceID, e.RelationType, e.TargetID))
	}

	return sb.String()
}

func FormatAdminBackupResult(v interface{}) string {
	res, ok := v.(map[string]interface{})
	if !ok {
		return FormatText(v)
	}
	return fmt.Sprintf("Backup created successfully: %s", Green(fmt.Sprintf("%v", res["backup"])))
}

func FormatAdminRestoreResult(v interface{}) string {
	res, ok := v.(map[string]interface{})
	if !ok {
		return FormatText(v)
	}
	return fmt.Sprintf("Database restored successfully from: %s", Green(fmt.Sprintf("%v", res["restore"])))
}

func FormatAdminRetryResult(v interface{}) string {
	res, ok := v.(map[string]interface{})
	if !ok {
		return FormatText(v)
	}
	return fmt.Sprintf("Retry complete. Retried tasks: %s", Green(fmt.Sprintf("%v", res["retried_tasks"])))
}

func WriteResult(w io.Writer, v interface{}, format string) error {
	if format == "json" {
		s, err := FormatJSON(v)
		if err != nil {
			return err
		}
		fmt.Fprintln(w, s)
		return nil
	}
	if s, ok := v.(string); ok {
		fmt.Fprintln(w, s)
		return nil
	}
	fmt.Fprintln(w, FormatText(v))
	return nil
}

func WriteError(w io.Writer, err error, code int, format string) {
	if format == "json" {
		res := map[string]interface{}{
			"error": err.Error(),
			"code":  code,
		}
		s, _ := FormatJSON(res)
		fmt.Fprintln(w, s)
		return
	}
	fmt.Fprintf(w, "error: %v\n", err)
}

const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
)

func Green(s string) string {
	if !ShouldColor() {
		return s
	}
	return ColorGreen + s + ColorReset
}

func Red(s string) string {
	if !ShouldColor() {
		return s
	}
	return ColorRed + s + ColorReset
}

func Yellow(s string) string {
	if !ShouldColor() {
		return s
	}
	return ColorYellow + s + ColorReset
}
