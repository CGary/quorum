package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTransitionTableDefinesAuthorizedForwardLifecycle(t *testing.T) {
	transitions := map[string]TaskTransition{}
	for _, transition := range transitionTable() {
		if transition.Name == "back" {
			t.Fatalf("back must remain a reversal dispatcher, not a forward transition")
		}
		if transition.Guard == nil || transition.Effect == nil {
			t.Fatalf("transition %s must expose guard and effect", transition.Name)
		}
		transitions[transition.Name] = transition
	}

	cases := []struct {
		name string
		from []string
		to   string
	}{
		{name: transitionBlueprint, from: []string{"inbox"}, to: "active"},
		{name: transitionStart, from: []string{"active", "inbox"}, to: "active"},
		{name: transitionClean, from: []string{"active", "done", "failed"}, to: "done"},
		{name: transitionAutoArchiveParent, from: []string{"active"}, to: "done"},
		{name: transitionRetryPrepare, from: []string{"failed"}, to: "active"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			transition, ok := transitions[tc.name]
			if !ok {
				t.Fatalf("transition %s missing", tc.name)
			}
			if transition.To != tc.to {
				t.Fatalf("transition %s target = %s, want %s", tc.name, transition.To, tc.to)
			}
			for _, from := range tc.from {
				if !transition.AllowsFrom(from) {
					t.Fatalf("transition %s does not allow source %s", tc.name, from)
				}
			}
		})
	}
	if _, ok := transitions["back"]; ok {
		t.Fatalf("back must not be registered as a forward lifecycle transition")
	}
}

func TestStartTransitionGuardBlocksMissingContractBeforeGit(t *testing.T) {
	useSchemas(t)
	root := mkFakeRepoRoot(t)
	taskDir := mkActiveTask(t, root, "FEAT-301")

	fake := newFakeGitRunner()
	out := captureStdout(t, func() { startTaskWith(fake, "FEAT-301") })
	if !strings.Contains(out, "Contract (02-contract.yaml) not found") {
		t.Fatalf("output %q missing contract guard message", out)
	}
	fake.assertNotCalled(t, "add ")
	for _, artifact := range []string{"04-implementation-log.yaml", "07-trace.json"} {
		if _, err := os.Stat(filepath.Join(taskDir, artifact)); !os.IsNotExist(err) {
			t.Fatalf("%s should not be initialized after guard failure: %v", artifact, err)
		}
	}
}
