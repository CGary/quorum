//go:build sqlite_fts5 && sqlite_vec

package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/hsme/core/src/bootstrap"
)

func runStatus(args []string, cfg bootstrap.Config) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	var watch bool
	var interval time.Duration
	fs.BoolVar(&watch, "watch", false, "Watch status changes in real-time")
	fs.DurationVar(&interval, "interval", 2*time.Second, "Watch refresh interval")

	RegisterDBFlags(fs, &cfg)
	fs.Parse(args)
	ScanTrailingFlags(fs)

	db, err := bootstrap.OpenDB(cfg)
	if err != nil {
		WriteError(os.Stderr, fmt.Errorf("failed to open database: %w", err), exitRuntime, outputFormat)
		os.Exit(exitRuntime)
	}
	defer db.Close()

	for {
		status, err := getStatus(context.Background(), db)
		if err != nil {
			WriteError(os.Stderr, err, exitRuntime, outputFormat)
			os.Exit(exitRuntime)
		}

		if outputFormat == "json" {
			WriteResult(os.Stdout, status, outputFormat)
		} else {
			printStatusText(status)
		}

		if !watch {
			break
		}

		time.Sleep(interval)
		if IsTTY() {
			fmt.Print("\033[H\033[2J") // Clear screen
		}
	}
}

type GraphStats struct {
	Nodes int64 `json:"nodes"`
	Edges int64 `json:"edges"`
}

type StatusInfo struct {
	Memories      int64            `json:"memories"`
	Graph         GraphStats       `json:"graph"`
	Tasks         map[string]int64 `json:"tasks"`
	WorkerRunning bool             `json:"worker_running"`
	LatestErrors  []string         `json:"latest_errors,omitempty"`
}

func getStatus(ctx context.Context, db *sql.DB) (*StatusInfo, error) {
	info := &StatusInfo{
		Tasks: make(map[string]int64),
	}

	err := db.QueryRowContext(ctx, "SELECT count(*) FROM memories").Scan(&info.Memories)
	if err != nil {
		return nil, fmt.Errorf("failed to count memories: %w", err)
	}

	err = db.QueryRowContext(ctx, "SELECT count(*) FROM kg_nodes").Scan(&info.Graph.Nodes)
	if err != nil {
		return nil, fmt.Errorf("failed to count graph nodes: %w", err)
	}

	err = db.QueryRowContext(ctx, "SELECT count(*) FROM kg_edge_evidence").Scan(&info.Graph.Edges)
	if err != nil {
		return nil, fmt.Errorf("failed to count graph edges: %w", err)
	}

	rows, err := db.QueryContext(ctx, "SELECT status, count(*) FROM async_tasks GROUP BY status")
	if err != nil {
		return nil, fmt.Errorf("failed to count tasks: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var s string
		var count int64
		if err := rows.Scan(&s, &count); err != nil {
			return nil, err
		}
		info.Tasks[s] = count
	}

	// Get latest errors
	errRows, err := db.QueryContext(ctx, "SELECT last_error FROM async_tasks WHERE last_error IS NOT NULL ORDER BY updated_at DESC LIMIT 5")
	if err == nil {
		defer errRows.Close()
		for errRows.Next() {
			var e string
			if err := errRows.Scan(&e); err == nil && e != "" {
				info.LatestErrors = append(info.LatestErrors, e)
			}
		}
	}

	info.WorkerRunning = checkWorkerRunning()

	return info, nil
}

func checkWorkerRunning() bool {
	if runtime.GOOS == "linux" {
		// Check /proc/*/comm
		files, err := os.ReadDir("/proc")
		if err == nil {
			for _, f := range files {
				if f.IsDir() && isDigit(f.Name()) {
					commPath := fmt.Sprintf("/proc/%s/comm", f.Name())
					comm, err := os.ReadFile(commPath)
					if err == nil && strings.TrimSpace(string(comm)) == "hsme-worker" {
						return true
					}
				}
			}
		}
	}

	// Fallback to pgrep
	cmd := exec.Command("pgrep", "hsme-worker")
	err := cmd.Run()
	return err == nil
}

func isDigit(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func printStatusText(s *StatusInfo) {
	workerStatus := Red("STOPPED")
	if s.WorkerRunning {
		workerStatus = Green("RUNNING")
	}
	fmt.Printf("Worker Status: %s\n", workerStatus)
	fmt.Printf("Memories: %d\n", s.Memories)
	fmt.Printf("Graph:    %d nodes, %d edges\n", s.Graph.Nodes, s.Graph.Edges)
	fmt.Println("Tasks:")
	states := []string{"pending", "processing", "completed", "failed"}
	for _, state := range states {
		count := s.Tasks[state]
		label := state
		switch state {
		case "pending":
			label = Yellow(state)
		case "processing":
			label = Green(state)
		case "completed":
			label = Green(state)
		case "failed":
			label = Red(state)
		}
		fmt.Printf("  - %-12s: %d\n", label, count)
	}

	if len(s.LatestErrors) > 0 {
		fmt.Println("\nLatest Errors:")
		for _, e := range s.LatestErrors {
			fmt.Printf("  - %s\n", Red(e))
		}
	}
}
