package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestQuorumConfig(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "quorum_config_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	config := &QuorumConfig{
		ProjectID:   "my-test-project",
		ProjectName: "My Test Project",
	}

	err = WriteQuorumConfigTo(config, tempDir)
	if err != nil {
		t.Fatalf("WriteQuorumConfigTo failed: %v", err)
	}

	readConfig, err := ReadQuorumConfigFrom(tempDir)
	if err != nil {
		t.Fatalf("ReadQuorumConfigFrom failed: %v", err)
	}

	if readConfig.ProjectID != "my-test-project" || readConfig.ProjectName != "My Test Project" {
		t.Errorf("Read config does not match written config: %+v", readConfig)
	}

	// Test invalid project id (not slug)
	invalidConfig := &QuorumConfig{
		ProjectID:   "Invalid_Project_ID!",
		ProjectName: "Invalid",
	}
	err = WriteQuorumConfigTo(invalidConfig, tempDir)
	if err == nil {
		t.Errorf("Expected error writing invalid project id, got nil")
	}

	// Test invalid keys in .quorumrc
	rcPath := filepath.Join(tempDir, ".quorumrc")
	invalidJSON := `{"project_id": "test", "project_name": "test", "extra_key": "not allowed"}`
	os.WriteFile(rcPath, []byte(invalidJSON), 0644)

	_, err = ReadQuorumConfigFrom(tempDir)
	if err == nil {
		t.Errorf("Expected error reading .quorumrc with invalid keys, got nil")
	}
}

func TestSuggestProjectIdentityAndRejectsPathKeys(t *testing.T) {
	root := initGitRepo(t)
	config := SuggestProjectIdentity(root)
	if config.ProjectID == "" || config.ProjectName == "" {
		t.Fatalf("expected suggested project identity, got %+v", config)
	}

	rcPath := filepath.Join(root, ".quorumrc")
	if err := os.WriteFile(rcPath, []byte(`{"project_id":"demo","project_name":"Demo","project_root":"/tmp/demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ReadQuorumConfigFrom(root)
	if err == nil || !strings.Contains(err.Error(), "invalid key") {
		t.Fatalf("expected invalid key error for local path field, got %v", err)
	}
}
