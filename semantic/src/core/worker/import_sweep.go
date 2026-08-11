package worker

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// SweepCounts holds the counts of items processed during a sweep phase.
type SweepCounts struct {
	Fetched  int
	Ingested int
	Skipped  int
	Errored  int
}

// SweepResult contains the outcome of a single sweep run.
type SweepResult struct {
	Skipped    bool
	SkipReason string
	Curated    SweepCounts
	Capsules   SweepCounts
	Error      error
}

// CuratedImportFunc is an injectable function that performs the curated delta import.
type CuratedImportFunc func(quorumDBPath, project string) (SweepCounts, error)

// CapsuleImportFunc is an injectable function that performs the capsule import.
type CapsuleImportFunc func(tasksRoot, project string) (SweepCounts, error)

// ProbeFunc checks if required import source locations exist.
type ProbeFunc func(quorumDBPath, tasksRoot string) error

// Sweeper manages periodic import sweeps.
type Sweeper struct {
	QuorumDBPath      string
	TasksRoot         string
	Project           string
	CuratedImportFunc CuratedImportFunc
	CapsuleImportFunc CapsuleImportFunc
	Probe             ProbeFunc

	running   int32
	probeDone bool
	probeErr  error
	probeMu   sync.Mutex
}

// ShouldRunSweep evaluates configuration and determines if sweeps should run.
func ShouldRunSweep(intervalS int, project string) (bool, string) {
	if intervalS <= 0 {
		return false, ""
	}
	if project == "" {
		return false, "IMPORT_INTERVAL_S set but IMPORT_PROJECT is empty; periodic import sweep disabled"
	}
	return true, ""
}

// RunOnce executes a single import sweep.
func (s *Sweeper) RunOnce(ctx context.Context) (SweepResult, error) {
	if s.Probe != nil {
		s.probeMu.Lock()
		if !s.probeDone {
			s.probeErr = s.Probe(s.QuorumDBPath, s.TasksRoot)
			s.probeDone = true
			if s.probeErr != nil {
				log.Printf("[import-sweep] startup source check failed: %v", s.probeErr)
			}
		}
		pErr := s.probeErr
		s.probeMu.Unlock()

		if pErr != nil {
			return SweepResult{
				Skipped:    true,
				SkipReason: fmt.Sprintf("source missing: %v", pErr),
			}, nil
		}
	}

	if !atomic.CompareAndSwapInt32(&s.running, 0, 1) {
		return SweepResult{
			Skipped:    true,
			SkipReason: "overlap",
		}, nil
	}
	defer atomic.StoreInt32(&s.running, 0)

	var res SweepResult

	if s.CuratedImportFunc != nil {
		cCounts, err := s.CuratedImportFunc(s.QuorumDBPath, s.Project)
		res.Curated = cCounts
		if err != nil {
			log.Printf("[import-sweep] curated import error: %v", err)
			res.Error = err
			return res, err
		}
	}

	if s.CapsuleImportFunc != nil {
		capCounts, err := s.CapsuleImportFunc(s.TasksRoot, s.Project)
		res.Capsules = capCounts
		if err != nil {
			log.Printf("[import-sweep] capsule import error: %v", err)
			res.Error = err
			return res, err
		}
	}

	log.Printf("[import-sweep] sweep completed: curated (%d fetched, %d ingested, %d skipped, %d errored), capsules (%d scanned, %d ingested, %d skipped, %d errored)",
		res.Curated.Fetched, res.Curated.Ingested, res.Curated.Skipped, res.Curated.Errored,
		res.Capsules.Fetched, res.Capsules.Ingested, res.Capsules.Skipped, res.Capsules.Errored)

	return res, nil
}

// Start launches a periodic ticker loop calling RunOnce until ctx is canceled.
func (s *Sweeper) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = s.RunOnce(ctx)
		}
	}
}
