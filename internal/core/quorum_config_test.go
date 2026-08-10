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

func TestQuorumConfigGitHideFlags(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "quorum_config_githide")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	t.Run("round-trip both flags true", func(t *testing.T) {
		config := &QuorumConfig{
			ProjectID:      "rt-project",
			ProjectName:    "RT Project",
			GitHideRuntime: true,
			GitHideAgents:  true,
		}
		if err := WriteQuorumConfigTo(config, tempDir); err != nil {
			t.Fatalf("WriteQuorumConfigTo failed: %v", err)
		}
		read, err := ReadQuorumConfigFrom(tempDir)
		if err != nil {
			t.Fatalf("ReadQuorumConfigFrom failed: %v", err)
		}
		if !read.GitHideRuntime || !read.GitHideAgents {
			t.Fatalf("expected both flags true, got GitHideRuntime=%v GitHideAgents=%v", read.GitHideRuntime, read.GitHideAgents)
		}
	})

	t.Run("defaults to false when absent", func(t *testing.T) {
		rcPath := filepath.Join(tempDir, ".quorumrc")
		if err := os.WriteFile(rcPath, []byte(`{"project_id":"absent","project_name":"Absent"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		read, err := ReadQuorumConfigFrom(tempDir)
		if err != nil {
			t.Fatalf("ReadQuorumConfigFrom failed: %v", err)
		}
		if read.GitHideRuntime || read.GitHideAgents {
			t.Fatalf("expected both flags false when absent, got GitHideRuntime=%v GitHideAgents=%v", read.GitHideRuntime, read.GitHideAgents)
		}
	})

	t.Run("explicitly false is accepted", func(t *testing.T) {
		rcPath := filepath.Join(tempDir, ".quorumrc")
		if err := os.WriteFile(rcPath, []byte(`{"project_id":"explicit","project_name":"Explicit","git_hide_runtime":false,"git_hide_agents":false}`), 0o644); err != nil {
			t.Fatal(err)
		}
		read, err := ReadQuorumConfigFrom(tempDir)
		if err != nil {
			t.Fatalf("ReadQuorumConfigFrom failed: %v", err)
		}
		if read.GitHideRuntime || read.GitHideAgents {
			t.Fatalf("expected both flags false, got GitHideRuntime=%v GitHideAgents=%v", read.GitHideRuntime, read.GitHideAgents)
		}
	})

	t.Run("unknown keys still rejected", func(t *testing.T) {
		rcPath := filepath.Join(tempDir, ".quorumrc")
		if err := os.WriteFile(rcPath, []byte(`{"project_id":"uk","project_name":"UK","bogus_key":"nope"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := ReadQuorumConfigFrom(tempDir)
		if err == nil || !strings.Contains(err.Error(), "invalid key") {
			t.Fatalf("expected invalid key error, got %v", err)
		}
	})
}
