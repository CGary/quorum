package core

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type QuorumConfig struct {
	ProjectID   string `json:"project_id"`
	ProjectName string `json:"project_name"`
}

var projectIDRegex = regexp.MustCompile(`^[a-z0-9-]+$`)
var slugCleanupRegex = regexp.MustCompile(`[^a-z0-9]+`)

// ReadQuorumConfig reads the .quorumrc file from the project root.
func ReadQuorumConfig() (*QuorumConfig, error) {
	root, err := ProjectRoot()
	if err != nil {
		return nil, err
	}
	return ReadQuorumConfigFrom(root)
}

// ReadQuorumConfigFrom reads the .quorumrc file from a specific directory.
func ReadQuorumConfigFrom(dir string) (*QuorumConfig, error) {
	rcPath := filepath.Join(dir, ".quorumrc")
	b, err := os.ReadFile(rcPath)
	if err != nil {
		return nil, err
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}

	for k := range raw {
		if k != "project_id" && k != "project_name" {
			return nil, fmt.Errorf("invalid key '%s' in .quorumrc: only project_id and project_name are allowed", k)
		}
	}

	var config QuorumConfig
	if err := json.Unmarshal(b, &config); err != nil {
		return nil, err
	}
	if err := ValidateQuorumConfig(&config); err != nil {
		return nil, err
	}
	return &config, nil
}

func ValidateQuorumConfig(config *QuorumConfig) error {
	if config == nil {
		return fmt.Errorf("quorum config is required")
	}
	if config.ProjectID == "" {
		return fmt.Errorf("project_id is required")
	}
	if !projectIDRegex.MatchString(config.ProjectID) {
		return fmt.Errorf("project_id must be slug-like (lowercase alphanumeric and hyphens)")
	}
	if config.ProjectName == "" {
		return fmt.Errorf("project_name is required")
	}
	return nil
}

// WriteQuorumConfig writes the QuorumConfig to the .quorumrc file in the project root.
func WriteQuorumConfig(config *QuorumConfig) error {
	root, err := ProjectRoot()
	if err != nil {
		return err
	}
	return WriteQuorumConfigTo(config, root)
}

// WriteQuorumConfigTo writes the QuorumConfig to the .quorumrc file in a specific directory.
func WriteQuorumConfigTo(config *QuorumConfig, dir string) error {
	if err := ValidateQuorumConfig(config); err != nil {
		return err
	}
	rcPath := filepath.Join(dir, ".quorumrc")
	b, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(rcPath, b, 0644)
}

func SuggestProjectIdentity(root string) *QuorumConfig {
	name := "quorum-project"
	if remote := GitRemote(root); remote != "" {
		name = strings.TrimSuffix(filepath.Base(remote), ".git")
	} else if base := filepath.Base(root); base != "." && base != string(filepath.Separator) {
		name = base
	}
	projectID := SlugifyProjectID(name)
	if projectID == "" {
		projectID = "quorum-project"
	}
	return &QuorumConfig{ProjectID: projectID, ProjectName: humanizeProjectName(name)}
}

func SlugifyProjectID(value string) string {
	v := strings.ToLower(strings.TrimSpace(value))
	v = strings.TrimSuffix(v, ".git")
	v = slugCleanupRegex.ReplaceAllString(v, "-")
	v = strings.Trim(v, "-")
	return v
}

func humanizeProjectName(value string) string {
	v := strings.TrimSuffix(filepath.Base(strings.TrimSpace(value)), ".git")
	v = strings.Trim(v, "-_ ")
	if v == "" {
		return "Quorum Project"
	}
	return v
}

func GitRemote(root string) string {
	out, err := exec.Command("git", "-C", root, "config", "--get", "remote.origin.url").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
