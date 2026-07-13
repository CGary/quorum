package core

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Level 1 structural fleet contract-test harness (FLEET-007 AC-3/AC-5): a
// pure, model-free check that a transport's binary is resolvable and that
// the CLI flags an adapter depends on still exist. It runs in CI with NO
// model/LLM invocation -- every probe is a cheap --help/version-style call
// (or a fixture standing in for one), never a dispatch/prompt call. A
// missing/renamed flag classifies as wrapper_broken, never reroute_quota;
// that vocabulary distinction belongs to the level-1 harness's own result
// type here, and is never produced by editing the engine's ADR-0011 classes
// in fleet_dispatch.go.
const contractProbeTimeout = 5 * time.Second

// ContractProbe is one flag-level check within a transport's contract test.
type ContractProbe struct {
	Name            string   `yaml:"name"`
	ProbeArgs       []string `yaml:"probe_args"`
	ExpectSubstring string   `yaml:"expect_substring"`
}

// ContractCheckSpec is the parsed shape of a
// .agents/fleet/contract_tests/<transport>.yaml file.
type ContractCheckSpec struct {
	Transport string          `yaml:"transport"`
	Binary    string          `yaml:"binary"`
	Checks    []ContractProbe `yaml:"checks"`
}

// ContractProbeResult is the classification of a single declared flag check.
type ContractProbeResult struct {
	Name    string  `json:"name"`
	Present bool    `json:"present"`
	Cause   *string `json:"cause"` // "wrapper_broken" when Present is false; never "reroute_quota"
}

// ContractLevel1Result is the aggregate outcome of RunContractLevel1 for one
// transport. Skipped is true (and OK/Checks empty) only when the transport
// binary itself is not resolvable on PATH -- a graceful, CI-safe skip, never
// a failure.
type ContractLevel1Result struct {
	Transport      string                 `json:"transport"`
	BinaryResolved bool                   `json:"binary_resolved"`
	Skipped        bool                   `json:"skipped"`
	SkipReason     string                 `json:"skip_reason,omitempty"`
	Checks         []ContractProbeResult  `json:"checks"`
	OK             bool                   `json:"ok"`
}

// LoadFleetContractSpec reads and parses a contract_tests/<transport>.yaml file.
func LoadFleetContractSpec(path string) (ContractCheckSpec, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ContractCheckSpec{}, err
	}
	var spec ContractCheckSpec
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		return ContractCheckSpec{}, err
	}
	return spec, nil
}

// RunContractLevel1 resolves spec.Binary via exec.LookPath and, for each
// declared check, runs a cheap probe with a short timeout and classifies the
// flag as present/missing by substring match against combined stdout+stderr.
// A binary that cannot be resolved on PATH is reported as Skipped so callers
// (including go test in CI with no vendor CLIs installed) can skip
// gracefully instead of failing.
func RunContractLevel1(spec ContractCheckSpec) ContractLevel1Result {
	result := ContractLevel1Result{Transport: spec.Transport}
	binPath, err := exec.LookPath(spec.Binary)
	if err != nil {
		result.Skipped = true
		result.SkipReason = "binary not found on PATH: " + spec.Binary
		return result
	}
	result.BinaryResolved = true
	result.OK = true
	for _, check := range spec.Checks {
		present := probeContainsSubstring(binPath, check.ProbeArgs, check.ExpectSubstring)
		cr := ContractProbeResult{Name: check.Name, Present: present}
		if !present {
			cause := "wrapper_broken"
			cr.Cause = &cause
			result.OK = false
		}
		result.Checks = append(result.Checks, cr)
	}
	return result
}

// probeContainsSubstring runs one cheap, non-model probe (bounded by
// contractProbeTimeout) and reports whether expect appears in its combined
// stdout+stderr. A non-zero exit code from the probe itself (e.g. --help
// returning 1) is not by itself a failure -- only the missing substring is.
func probeContainsSubstring(binPath string, args []string, expect string) bool {
	if expect == "" {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), contractProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binPath, args...)
	out, _ := cmd.CombinedOutput()
	return strings.Contains(string(out), expect)
}
