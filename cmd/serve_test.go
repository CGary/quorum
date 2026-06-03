package cmd

import (
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServeCmdFlags(t *testing.T) {
	f := serveCmd.Flags().Lookup("port")
	if f == nil {
		t.Fatal("Expected --port flag to exist")
	}
	if f.DefValue != "8080" {
		t.Errorf("Expected default port to be 8080, got %s", f.DefValue)
	}

	hostFlag := serveCmd.Flags().Lookup("host")
	if hostFlag == nil {
		t.Fatal("Expected --host flag to exist")
	}
	if hostFlag.DefValue != "127.0.0.1" {
		t.Errorf("Expected default host to be 127.0.0.1, got %s", hostFlag.DefValue)
	}

	startHostFlag := serveStartCmd.Flags().Lookup("host")
	if startHostFlag == nil {
		t.Fatal("Expected serve start --host flag to exist")
	}
	if startHostFlag.DefValue != "127.0.0.1" {
		t.Errorf("Expected serve start default host to be 127.0.0.1, got %s", startHostFlag.DefValue)
	}

	startFlag := serveStartCmd.Flags().Lookup("port")
	if startFlag == nil {
		t.Fatal("Expected serve start --port flag to exist")
	}
	if startFlag.DefValue != "8080" {
		t.Errorf("Expected serve start default port to be 8080, got %s", startFlag.DefValue)
	}
}

func TestServeCmdSubcommands(t *testing.T) {
	for _, name := range []string{"start", "stop", "status"} {
		cmd, _, err := serveCmd.Find([]string{name})
		if err != nil {
			t.Fatalf("Find(%s) returned error: %v", name, err)
		}
		if cmd == nil || cmd.Name() != name {
			t.Fatalf("Expected serve subcommand %s to exist", name)
		}
	}
}

func TestServeStateRoundTripUsesRuntimeDir(t *testing.T) {
	runtimeDir := t.TempDir()
	t.Setenv("QUORUM_RUNTIME_DIR", runtimeDir)
	state := serveState{
		PID:       12345,
		Port:      9090,
		Host:      "0.0.0.0",
		URL:       "http://0.0.0.0:9090",
		LogPath:   filepath.Join(runtimeDir, "server.log"),
		StartedAt: "2026-06-03T00:00:00Z",
	}
	if err := writeServeState(state); err != nil {
		t.Fatalf("writeServeState failed: %v", err)
	}
	got, ok, err := readServeState()
	if err != nil {
		t.Fatalf("readServeState failed: %v", err)
	}
	if !ok {
		t.Fatal("Expected state to exist")
	}
	if got != state {
		t.Fatalf("state = %+v, want %+v", got, state)
	}

	data, err := os.ReadFile(filepath.Join(runtimeDir, "server.pid"))
	if err != nil {
		t.Fatalf("server.pid was not written: %v", err)
	}
	var decoded serveState
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("server.pid is not JSON: %v", err)
	}
}

func TestServeStartStatusStopIntegration(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "quorum")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, string(out))
	}

	port := freeTCPPort(t)
	runtimeDir := t.TempDir()
	memoryDB := filepath.Join(t.TempDir(), "memory.db")
	env := append(os.Environ(),
		"QUORUM_RUNTIME_DIR="+runtimeDir,
		"QUORUM_MEMORY_DB="+memoryDB,
	)

	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Env = env
		cmd.Dir = ".."
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s failed: %v\n%s", strings.Join(args, " "), err, string(out))
		}
		return string(out)
	}

	out := run("serve", "start", "--host", "0.0.0.0", "--port", port)
	if !strings.Contains(out, "Quorum server started on http://0.0.0.0:"+port) {
		t.Fatalf("unexpected start output: %s", out)
	}
	defer func() {
		cmd := exec.Command(bin, "serve", "stop")
		cmd.Env = env
		cmd.Dir = ".."
		_ = cmd.Run()
	}()

	out = run("serve", "status")
	if !strings.Contains(out, "Quorum server is running on http://0.0.0.0:"+port) {
		t.Fatalf("unexpected status output: %s", out)
	}

	out = run("serve", "stop")
	if !strings.Contains(out, "Quorum server stopped.") {
		t.Fatalf("unexpected stop output: %s", out)
	}

	out = run("serve", "status")
	if !strings.Contains(out, "Quorum server is not running.") {
		t.Fatalf("unexpected stopped status output: %s", out)
	}
}

func freeTCPPort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to allocate port: %v", err)
	}
	defer ln.Close()
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("failed to parse address: %v", err)
	}
	// Give the OS a moment to release the port for the child process on loaded runners.
	time.Sleep(10 * time.Millisecond)
	return port
}
