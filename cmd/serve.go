package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"quorum/internal/core"
	"quorum/internal/server"
)

var port int
var host string

type serveState struct {
	PID        int    `json:"pid"`
	Port       int    `json:"port"`
	Host       string `json:"host"`
	URL        string `json:"url"`
	LogPath    string `json:"log_path"`
	StartedAt  string `json:"started_at"`
	Executable string `json:"executable"`
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start a read-only server for projects, reports, memories, and task state",
	Long:  `Start a read-only API server to serve JSON data about projects, reports, memories, and task state.`,
	Run: func(cmd *cobra.Command, args []string) {
		srv, err := server.NewServer()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to initialize server: %v\n", err)
			os.Exit(1)
		}
		if err := srv.Start(host, port); err != nil {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
			os.Exit(1)
		}
	},
}

var serveStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the Quorum server in the background",
	RunE: func(cmd *cobra.Command, args []string) error {
		return startServeBackground(host, port)
	},
}

var serveStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the background Quorum server",
	RunE: func(cmd *cobra.Command, args []string) error {
		return stopServeBackground()
	},
}

var serveStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show background Quorum server status",
	RunE: func(cmd *cobra.Command, args []string) error {
		return showServeStatus()
	},
}

func init() {
	serveCmd.Flags().StringVar(&host, "host", "127.0.0.1", "Host/interface to bind the server on")
	serveCmd.Flags().IntVarP(&port, "port", "p", 8080, "Port to run the server on")
	serveStartCmd.Flags().StringVar(&host, "host", "127.0.0.1", "Host/interface to bind the server on")
	serveStartCmd.Flags().IntVarP(&port, "port", "p", 8080, "Port to run the server on")
	serveCmd.AddCommand(serveStartCmd, serveStopCmd, serveStatusCmd)
	rootCmd.AddCommand(serveCmd)
}

func startServeBackground(host string, port int) error {
	if host == "" {
		host = "127.0.0.1"
	}
	if state, ok, err := readServeState(); err != nil {
		return err
	} else if ok && serveProcessRunning(state) {
		fmt.Printf("Quorum server is already running on %s\nPID: %d\n", state.URL, state.PID)
		return nil
	} else if ok {
		_ = os.Remove(servePIDPath())
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("port %d is not available: %w", port, err)
	}
	_ = ln.Close()

	runtimeDir, err := serveRuntimeDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		return err
	}
	logPath := filepath.Join(runtimeDir, "server.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer logFile.Close()

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "serve", "--host", host, "--port", strconv.Itoa(port))
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = os.Environ()
	cmd.Dir, _ = os.Getwd()
	if err := cmd.Start(); err != nil {
		return err
	}

	state := serveState{
		PID:        cmd.Process.Pid,
		Port:       port,
		Host:       host,
		URL:        "http://" + addr,
		LogPath:    logPath,
		StartedAt:  time.Now().UTC().Format(time.RFC3339),
		Executable: exe,
	}
	if err := writeServeState(state); err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	_ = cmd.Process.Release()

	if err := waitForServeReady(addr, state.PID, 2*time.Second); err != nil {
		_ = os.Remove(servePIDPath())
		return err
	}

	fmt.Printf("Quorum server started on %s\nPID: %d\nLogs: %s\n", state.URL, state.PID, state.LogPath)
	return nil
}

func stopServeBackground() error {
	state, ok, err := readServeState()
	if err != nil {
		return err
	}
	if !ok {
		fmt.Println("Quorum server is not running.")
		return nil
	}
	if !serveProcessRunning(state) {
		_ = os.Remove(servePIDPath())
		fmt.Println("Quorum server is not running.")
		return nil
	}

	proc, err := os.FindProcess(state.PID)
	if err != nil {
		return err
	}
	if err := proc.Signal(os.Interrupt); err != nil {
		return err
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !processRunning(state.PID) {
			_ = os.Remove(servePIDPath())
			fmt.Printf("Quorum server stopped.\nPID: %d\n", state.PID)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = proc.Kill()
	_ = os.Remove(servePIDPath())
	fmt.Printf("Quorum server stopped.\nPID: %d\n", state.PID)
	return nil
}

func showServeStatus() error {
	state, ok, err := readServeState()
	if err != nil {
		return err
	}
	if !ok || !serveProcessRunning(state) {
		if ok {
			_ = os.Remove(servePIDPath())
		}
		fmt.Println("Quorum server is not running.")
		return nil
	}
	fmt.Printf("Quorum server is running on %s\nPID: %d\nLogs: %s\n", state.URL, state.PID, state.LogPath)
	return nil
}

func waitForServeReady(addr string, pid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processRunning(pid) {
			return errors.New("server process exited before becoming ready")
		}
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("server did not become ready before timeout")
}

func serveRuntimeDir() (string, error) {
	if dir := os.Getenv("QUORUM_RUNTIME_DIR"); dir != "" {
		return dir, nil
	}
	root, err := core.ProjectRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, ".ai", "runtime"), nil
}

func servePIDPath() string {
	dir, err := serveRuntimeDir()
	if err != nil {
		return filepath.Join(".ai", "runtime", "server.pid")
	}
	return filepath.Join(dir, "server.pid")
}

func readServeState() (serveState, bool, error) {
	path := servePIDPath()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return serveState{}, false, nil
	}
	if err != nil {
		return serveState{}, false, err
	}
	var state serveState
	if err := json.Unmarshal(data, &state); err != nil {
		return serveState{}, false, err
	}
	return state, true, nil
}

func writeServeState(state serveState) error {
	path := servePIDPath()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func serveProcessRunning(state serveState) bool {
	if !processRunning(state.PID) {
		return false
	}
	if state.Executable == "" {
		return true
	}
	procExe := filepath.Join("/proc", strconv.Itoa(state.PID), "exe")
	resolved, err := os.Readlink(procExe)
	if err != nil {
		return true
	}
	expected, err := filepath.EvalSymlinks(state.Executable)
	if err != nil {
		expected = state.Executable
	}
	actual, err := filepath.EvalSymlinks(resolved)
	if err != nil {
		actual = resolved
	}
	return actual == expected
}

func processRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
