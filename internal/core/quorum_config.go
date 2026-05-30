package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

type QuorumConfig struct {
	ProjectID   string `json:"project_id"`
	ProjectName string `json:"project_name"`
}

var projectIDRegex = regexp.MustCompile(`^[a-z0-9-]+$`)

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

	if config.ProjectID == "" {
		return nil, fmt.Errorf("project_id is required")
	}
	if !projectIDRegex.MatchString(config.ProjectID) {
		return nil, fmt.Errorf("project_id must be slug-like (lowercase alphanumeric and hyphens)")
	}
	if config.ProjectName == "" {
		return nil, fmt.Errorf("project_name is required")
	}

	return &config, nil
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
	if config.ProjectID == "" {
		return fmt.Errorf("project_id is required")
	}
	if !projectIDRegex.MatchString(config.ProjectID) {
		return fmt.Errorf("project_id must be slug-like (lowercase alphanumeric and hyphens)")
	}
	if config.ProjectName == "" {
		return fmt.Errorf("project_name is required")
	}

	rcPath := filepath.Join(dir, ".quorumrc")
	b, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(rcPath, b, 0644)
}
