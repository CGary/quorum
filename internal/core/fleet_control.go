package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

func FleetControlPath(projectRoot string) string {
	return filepath.Join(projectRoot, ".ai", "fleet-control.json")
}

func LoadFleetControlState(projectRoot string) (ControlState, error) {
	path := FleetControlPath(projectRoot)
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ControlState{}, nil
		}
		return ControlState{}, err
	}
	var state ControlState
	if err := json.Unmarshal(b, &state); err != nil {
		return ControlState{}, err
	}
	return state, nil
}

func SaveFleetControlState(projectRoot string, state ControlState) error {
	path := FleetControlPath(projectRoot)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "fleet-control-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmpName, path)
}

type fleetControlAgentsFile struct {
	Transports map[string]struct {
		Models map[string]struct {
			Provider string `yaml:"provider"`
		} `yaml:"models"`
	} `yaml:"transports"`
}

func ValidateFleetTarget(projectRoot, target string) error {
	agentsPath := filepath.Join(projectRoot, ".agents", "fleet", "agents.yaml")
	b, err := os.ReadFile(agentsPath)
	if err != nil {
		return fmt.Errorf("read agents.yaml: %w", err)
	}
	var file fleetControlAgentsFile
	if err := yaml.Unmarshal(b, &file); err != nil {
		return fmt.Errorf("parse agents.yaml: %w", err)
	}

	agent := target
	model := ""
	if idx := strings.Index(target, "/"); idx != -1 {
		agent = target[:idx]
		model = target[idx+1:]
	}

	t, ok := file.Transports[agent]
	if !ok {
		return fmt.Errorf("unknown transport: %q", agent)
	}

	if model != "" {
		if _, ok := t.Models[model]; !ok {
			return fmt.Errorf("unknown model %q on transport %q", model, agent)
		}
	}

	return nil
}

var fleetControlMu sync.Mutex

func DisableFleetTarget(projectRoot, target, reason, by string) (ControlState, error) {
	target = strings.TrimSpace(target)
	reason = strings.TrimSpace(reason)
	if target == "" {
		return ControlState{}, fmt.Errorf("empty target")
	}
	if reason == "" {
		return ControlState{}, fmt.Errorf("empty reason")
	}

	if err := ValidateFleetTarget(projectRoot, target); err != nil {
		return ControlState{}, err
	}

	fleetControlMu.Lock()
	defer fleetControlMu.Unlock()

	state, err := LoadFleetControlState(projectRoot)
	if err != nil {
		return ControlState{}, err
	}

	found := false
	for i, e := range state.Disabled {
		if e.Target == target {
			state.Disabled[i] = ControlEntry{
				Target: target,
				Reason: reason,
				By:     by,
				At:     time.Now().UTC().Format(time.RFC3339),
			}
			found = true
			break
		}
	}
	if !found {
		state.Disabled = append(state.Disabled, ControlEntry{
			Target: target,
			Reason: reason,
			By:     by,
			At:     time.Now().UTC().Format(time.RFC3339),
		})
	}
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	if err := SaveFleetControlState(projectRoot, state); err != nil {
		return ControlState{}, err
	}

	return state, nil
}

func EnableFleetTarget(projectRoot, target string) (ControlState, error) {
	if target == "" {
		return ControlState{}, fmt.Errorf("empty target")
	}

	fleetControlMu.Lock()
	defer fleetControlMu.Unlock()

	state, err := LoadFleetControlState(projectRoot)
	if err != nil {
		return ControlState{}, err
	}

	newDisabled := []ControlEntry{}
	for _, e := range state.Disabled {
		if e.Target != target {
			newDisabled = append(newDisabled, e)
		}
	}
	state.Disabled = newDisabled
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	if err := SaveFleetControlState(projectRoot, state); err != nil {
		return ControlState{}, err
	}

	return state, nil
}

type FleetStatusEntry struct {
	Target     string  `json:"target"`
	Reason     string  `json:"reason"`
	By         string  `json:"by"`
	At         string  `json:"at"`
	AgeSeconds float64 `json:"age_seconds"`
}

type FleetStatusReport struct {
	Disabled  []FleetStatusEntry `json:"disabled"`
	UpdatedAt string             `json:"updated_at"`
}

func BuildFleetStatusReport(state ControlState, now time.Time) FleetStatusReport {
	entries := []FleetStatusEntry{}
	for _, e := range state.Disabled {
		parsedAt, err := time.Parse(time.RFC3339, e.At)
		ageSeconds := 0.0
		if err == nil {
			ageSeconds = now.Sub(parsedAt).Seconds()
		}
		entries = append(entries, FleetStatusEntry{
			Target:     e.Target,
			Reason:     e.Reason,
			By:         e.By,
			At:         e.At,
			AgeSeconds: ageSeconds,
		})
	}
	return FleetStatusReport{
		Disabled:  entries,
		UpdatedAt: state.UpdatedAt,
	}
}
