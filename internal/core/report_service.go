package core

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var reportMetaDateLine = regexp.MustCompile(`(?m)^(\s+)date:\s*".*"`)

func scaffoldReportTemplate(tmpl []byte, id string) string {
	out := strings.Replace(string(tmpl), `id: "template-id"`, fmt.Sprintf(`id: %q`, id), 1)
	out = reportMetaDateLine.ReplaceAllString(out, fmt.Sprintf(`${1}date: %q`, time.Now().UTC().Format(time.RFC3339)))
	return out
}

func loadReportTemplate(projectRoot, kind string) ([]byte, error) {
	if kind == "" || kind == "generic" {
		onDisk := filepath.Join(projectRoot, ".agents", "templates", "report.yaml")
		if b, err := os.ReadFile(onDisk); err == nil {
			return b, nil
		}
		if b, ok := EmbeddedAgentFile("templates/report.yaml"); ok {
			return b, nil
		}
		return nil, fmt.Errorf("report template not found on disk (%s) or embedded in the binary", onDisk)
	}

	onDisk := filepath.Join(projectRoot, ".agents", "templates", "reports", kind+".yaml")
	if b, err := os.ReadFile(onDisk); err == nil {
		return b, nil
	}
	if b, ok := EmbeddedAgentFile("templates/reports/" + kind + ".yaml"); ok {
		return b, nil
	}
	return nil, fmt.Errorf("report template for kind %q not found on disk (%s) or embedded in the binary", kind, onDisk)
}

func fillReportMetadata(payload any) {
	root, ok := payload.(map[string]any)
	if !ok {
		return
	}
	meta, ok := root["meta"].(map[string]any)
	if !ok {
		meta = map[string]any{}
		root["meta"] = meta
	}
	if s, _ := meta["schemaVersion"].(string); strings.TrimSpace(s) == "" {
		meta["schemaVersion"] = "1.1"
	}
	if d, _ := meta["date"].(string); strings.TrimSpace(d) == "" {
		meta["date"] = time.Now().UTC().Format(time.RFC3339)
	}
}

type ReportService struct {
	ProjectRoot string
}

type ReportNewOptions struct {
	OutputPath string
	Kind       string
}

type ReportSaveOptions struct {
	DryRun bool
}

type ReportNewResult struct {
	Path       string
	IsScaffold bool
}

type ReportSaveResult struct {
	Path   string
	DryRun bool
}

func (s ReportService) NewReport(id string, opts ReportNewOptions) (ReportNewResult, error) {
	if err := ValidateReportID(id); err != nil {
		return ReportNewResult{}, err
	}

	if opts.OutputPath != "" {
		tmplData, err := loadReportTemplate(s.ProjectRoot, opts.Kind)
		if err != nil {
			return ReportNewResult{}, err
		}
		scaffold := scaffoldReportTemplate(tmplData, id)

		var payload map[string]any
		if err := yaml.Unmarshal([]byte(scaffold), &payload); err != nil {
			return ReportNewResult{}, fmt.Errorf("error parsing scaffolded template: %v", err)
		}
		fillReportMetadata(payload)
		if err := ValidateAgainstSchema("report.schema.json", opts.OutputPath, payload); err != nil {
			return ReportNewResult{}, err
		}
		if err := os.MkdirAll(filepath.Dir(opts.OutputPath), 0755); err != nil {
			return ReportNewResult{}, fmt.Errorf("error creating output directory: %v", err)
		}
		if err := os.WriteFile(opts.OutputPath, []byte(scaffold), 0644); err != nil {
			return ReportNewResult{}, fmt.Errorf("error writing scaffold: %v", err)
		}
		return ReportNewResult{Path: opts.OutputPath, IsScaffold: true}, nil
	}

	config, err := ReadQuorumConfigFrom(s.ProjectRoot)
	if config == nil || err != nil {
		config = &QuorumConfig{ProjectID: filepath.Base(s.ProjectRoot), ProjectName: filepath.Base(s.ProjectRoot)}
	}

	db, err := OpenMemoryDB("")
	if err != nil {
		return ReportNewResult{}, fmt.Errorf("error opening memory db: %v", err)
	}
	defer db.Close()

	remote := GitRemote(s.ProjectRoot)
	if err := EnsureMemoryProject(db, config, s.ProjectRoot, remote); err != nil {
		return ReportNewResult{}, fmt.Errorf("error registering project in memory: %v", err)
	}

	reportsDir := filepath.Join(s.ProjectRoot, ".ai", "reports")
	reportPath := filepath.Join(reportsDir, fmt.Sprintf("%s.yaml", id))
	if _, err := os.Stat(reportPath); err == nil {
		return ReportNewResult{}, fmt.Errorf("error: report file %s already exists", reportPath)
	}

	tmplData, err := loadReportTemplate(s.ProjectRoot, opts.Kind)
	if err != nil {
		return ReportNewResult{}, err
	}

	var payload map[string]any
	if err := yaml.Unmarshal(tmplData, &payload); err != nil {
		return ReportNewResult{}, fmt.Errorf("error parsing template: %v", err)
	}

	if meta, ok := payload["meta"].(map[string]any); ok {
		meta["id"] = id
		meta["date"] = time.Now().UTC().Format(time.RFC3339)
	}

	if _, err := SaveArtifact(reportPath, payload); err != nil {
		return ReportNewResult{}, err
	}

	return ReportNewResult{Path: reportPath, IsScaffold: false}, nil
}

func (s ReportService) SaveReport(id string, raw []byte, opts ReportSaveOptions) (ReportSaveResult, error) {
	if err := ValidateReportID(id); err != nil {
		return ReportSaveResult{}, err
	}

	reportsDir := filepath.Join(s.ProjectRoot, ".ai", "reports")
	if err := os.MkdirAll(reportsDir, 0755); err != nil {
		return ReportSaveResult{}, fmt.Errorf("error creating reports directory: %v", err)
	}

	reportPath := filepath.Join(reportsDir, fmt.Sprintf("%s.yaml", id))
	tmpPath := reportPath + ".tmp"
	if err := os.WriteFile(tmpPath, raw, 0644); err != nil {
		return ReportSaveResult{}, err
	}
	defer os.Remove(tmpPath)

	payload, err := LoadArtifactPayload(tmpPath)
	if err != nil {
		return ReportSaveResult{}, fmt.Errorf("error: payload parse failed: %v", err)
	}

	fillReportMetadata(payload)

	if err := CheckReportIDMatches(payload, id); err != nil {
		return ReportSaveResult{}, err
	}

	if opts.DryRun {
		if err := ValidateArtifact(reportPath, payload); err != nil {
			return ReportSaveResult{}, err
		}
		return ReportSaveResult{Path: reportPath, DryRun: true}, nil
	}

	if _, err := SaveArtifact(reportPath, payload); err != nil {
		return ReportSaveResult{}, err
	}

	return ReportSaveResult{Path: reportPath, DryRun: false}, nil
}

func (s ReportService) ListReports() ([]ReportSummary, error) {
	reportsDir := filepath.Join(s.ProjectRoot, ".ai", "reports")
	return ListReports(reportsDir)
}
