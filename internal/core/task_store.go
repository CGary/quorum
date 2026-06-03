package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type TaskStore struct {
	ProjectRoot string
}

func NewTaskStore(projectRoot string) TaskStore {
	return TaskStore{ProjectRoot: projectRoot}
}

func DefaultTaskStore() (TaskStore, error) {
	root, err := ProjectRoot()
	if err != nil {
		return TaskStore{}, err
	}
	return TaskStore{ProjectRoot: root}, nil
}

func (s TaskStore) FindTask(id string, locations ...string) (*TaskDirMatch, error) {
	return FindTaskDirIn(s.ProjectRoot, id, locations)
}

func (s TaskStore) TaskArtifactPath(task *TaskDirMatch, name string) (string, error) {
	if task == nil {
		return "", fmt.Errorf("task is nil")
	}
	if task.Path == "" {
		return "", fmt.Errorf("task path is empty")
	}
	if name == "" || name == "." || name == ".." {
		return "", fmt.Errorf("invalid artifact name: %q", name)
	}
	if strings.ContainsAny(name, "/\\") {
		return "", fmt.Errorf("artifact name contains path separators: %q", name)
	}
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("artifact name cannot be an absolute path: %q", name)
	}
	if filepath.Base(name) != name {
		return "", fmt.Errorf("artifact name must be a simple base name: %q", name)
	}

	return filepath.Join(task.Path, name), nil
}

func (s TaskStore) LoadArtifact(task *TaskDirMatch, name string) (any, error) {
	path, err := s.TaskArtifactPath(task, name)
	if err != nil {
		return nil, err
	}
	payload, err := LoadArtifactPayload(path)
	if err != nil {
		return nil, err
	}
	if err := ValidateArtifact(path, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (s TaskStore) SaveArtifact(task *TaskDirMatch, name string, payload any) (string, error) {
	path, err := s.TaskArtifactPath(task, name)
	if err != nil {
		return "", err
	}
	return SaveArtifact(path, payload)
}

func (s TaskStore) MoveTask(task *TaskDirMatch, targetLocation string) (*TaskDirMatch, error) {
	if task == nil {
		return nil, fmt.Errorf("task is nil")
	}
	if task.Path == "" || task.Location == "" {
		return nil, fmt.Errorf("task path or location is empty")
	}
	switch targetLocation {
	case "inbox", "active", "done", "failed":
		// valid
	default:
		return nil, fmt.Errorf("invalid target location: %q", targetLocation)
	}

	if task.Location == targetLocation {
		return &TaskDirMatch{Path: task.Path, Location: task.Location}, nil
	}

	basename := filepath.Base(task.Path)
	targetDir := filepath.Join(s.ProjectRoot, ".ai", "tasks", targetLocation, basename)

	if _, err := os.Stat(targetDir); err == nil {
		return nil, fmt.Errorf("target task directory already exists: %s", targetDir)
	}

	if err := os.MkdirAll(filepath.Dir(targetDir), 0755); err != nil {
		return nil, err
	}

	if err := os.Rename(task.Path, targetDir); err != nil {
		return nil, err
	}

	return &TaskDirMatch{
		Path:     targetDir,
		Location: targetLocation,
	}, nil
}
