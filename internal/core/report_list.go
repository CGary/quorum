package core

import (
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// ReportSummary is the stable, list-oriented view of a persisted report.
type ReportSummary struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
	Date  string `json:"date"`
}

// ListReports enumerates report YAML files in reportsDir and extracts the
// summary fields used by human and JSON list output. Unparsable files are
// skipped so one corrupt report cannot hide the rest of the directory.
func ListReports(reportsDir string) ([]ReportSummary, error) {
	reports := []ReportSummary{}

	matches, err := filepath.Glob(filepath.Join(reportsDir, "*.yaml"))
	if err != nil {
		return reports, err
	}
	sort.Strings(matches)

	for _, path := range matches {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var payload map[string]any
		if err := yaml.Unmarshal(raw, &payload); err != nil {
			continue
		}

		reports = append(reports, reportSummaryFromPayload(payload))
	}

	sort.SliceStable(reports, func(i, j int) bool {
		if reports[i].ID == reports[j].ID {
			return reports[i].Title < reports[j].Title
		}
		return reports[i].ID < reports[j].ID
	})
	return reports, nil
}

func reportSummaryFromPayload(payload map[string]any) ReportSummary {
	meta, _ := payload["meta"].(map[string]any)
	content, _ := payload["content"].(map[string]any)

	return ReportSummary{
		ID:    stringField(meta, "id"),
		Kind:  stringField(payload, "kind"),
		Title: stringField(content, "title"),
		Date:  stringField(meta, "date"),
	}
}

func stringField(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, _ := values[key].(string)
	return value
}
