package core

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// fixtureContractSpec builds a ContractCheckSpec pointed at the checked-in
// fake_cli.sh fixture, toggled via FLEET_CONTRACT_FIXTURE_MODE.
func fixtureContractSpec(t *testing.T, mode string) ContractCheckSpec {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	script := filepath.Join(filepath.Dir(file), "testdata", "fleet_contract", "fake_cli.sh")
	t.Setenv("FLEET_CONTRACT_FIXTURE_MODE", mode)
	return ContractCheckSpec{
		Transport: "fixture",
		Binary:    script,
		Checks: []ContractProbe{
			{Name: "model_flag", ProbeArgs: []string{"--help"}, ExpectSubstring: "--model"},
			// Trailing space avoids a false "present" match against the
			// unrelated "--print-timeout" flag, which also contains "--print"
			// as a raw substring.
			{Name: "print_flag", ProbeArgs: []string{"--help"}, ExpectSubstring: "--print "},
			{Name: "print_timeout_flag", ProbeArgs: []string{"--help"}, ExpectSubstring: "--print-timeout"},
		},
	}
}

func TestFleetContractLevel1OkFixturePasses(t *testing.T) {
	res := RunContractLevel1(fixtureContractSpec(t, "ok"))
	if res.Skipped {
		t.Fatalf("want the ok fixture binary to resolve, got skipped: %s", res.SkipReason)
	}
	if !res.OK {
		t.Fatalf("want the ok fixture to pass every check, got %+v", res)
	}
	for _, c := range res.Checks {
		if !c.Present {
			t.Fatalf("check %s unexpectedly missing in ok fixture: %+v", c.Name, res)
		}
		if c.Cause != nil {
			t.Fatalf("check %s must have no cause when present, got %q", c.Name, *c.Cause)
		}
	}
}

func TestFleetContractLevel1BrokenFixtureReportsWrapperBroken(t *testing.T) {
	res := RunContractLevel1(fixtureContractSpec(t, "broken"))
	if res.Skipped {
		t.Fatalf("want the broken fixture binary to resolve, got skipped: %s", res.SkipReason)
	}
	if res.OK {
		t.Fatal("want the broken fixture (missing --print) to fail the overall contract check")
	}
	var found bool
	for _, c := range res.Checks {
		if c.Name != "print_flag" {
			continue
		}
		found = true
		if c.Present {
			t.Fatal("want print_flag check to report missing for the broken fixture")
		}
		if c.Cause == nil || *c.Cause != "wrapper_broken" {
			t.Fatalf("want print_flag cause=wrapper_broken (never reroute_quota), got %v", c.Cause)
		}
	}
	if !found {
		t.Fatal("print_flag check not found in results")
	}
	// The unrelated flags must still classify as present -- one broken flag
	// must not taint sibling checks.
	for _, c := range res.Checks {
		if c.Name != "print_flag" && !c.Present {
			t.Fatalf("unrelated check %s unexpectedly missing in broken fixture: %+v", c.Name, res)
		}
	}
}

func TestFleetContractLevel1SkipsWhenBinaryAbsent(t *testing.T) {
	spec := ContractCheckSpec{Transport: "ghost", Binary: "quorum-fleet-ghost-cli-does-not-exist"}
	res := RunContractLevel1(spec)
	if !res.Skipped {
		t.Fatal("want RunContractLevel1 to skip gracefully when the binary is absent from PATH")
	}
	if res.OK {
		t.Fatal("a skipped transport must not report OK=true")
	}
}

// TestFleetContractLevel1RealTransports is a best-effort loop over every
// checked-in .agents/fleet/contract_tests/*.yaml file (agy and claude from
// this task, codex from the parallel FLEET-006-b task). It skips (never
// fails) any transport whose real vendor binary is absent from PATH, so
// go test ./... stays green under CGO_ENABLED=0 in a CI environment with no
// vendor CLIs installed.
func TestFleetContractLevel1RealTransports(t *testing.T) {
	root, err := ProjectRoot()
	if err != nil {
		t.Skipf("cannot resolve project root: %v", err)
	}
	dir := filepath.Join(root, ".agents", "fleet", "contract_tests")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("contract_tests directory not found: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			spec, err := LoadFleetContractSpec(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("load contract spec %s: %v", name, err)
			}
			res := RunContractLevel1(spec)
			if res.Skipped {
				t.Skipf("transport %s vendor binary not on PATH: %s", spec.Transport, res.SkipReason)
			}
		})
	}
}
