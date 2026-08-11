package worker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestShouldRunSweep(t *testing.T) {
	tests := []struct {
		name        string
		intervalS   int
		project     string
		wantEnabled bool
		wantWarn    bool
	}{
		{
			name:        "disabled interval <= 0 empty project",
			intervalS:   0,
			project:     "",
			wantEnabled: false,
			wantWarn:    false,
		},
		{
			name:        "disabled interval <= 0 with project",
			intervalS:   -5,
			project:     "quorum",
			wantEnabled: false,
			wantWarn:    false,
		},
		{
			name:        "disabled interval > 0 empty project",
			intervalS:   60,
			project:     "",
			wantEnabled: false,
			wantWarn:    true,
		},
		{
			name:        "enabled interval > 0 with project",
			intervalS:   30,
			project:     "quorum",
			wantEnabled: true,
			wantWarn:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enabled, warn := ShouldRunSweep(tt.intervalS, tt.project)
			if enabled != tt.wantEnabled {
				t.Errorf("ShouldRunSweep() enabled = %v, want %v", enabled, tt.wantEnabled)
			}
			if (warn != "") != tt.wantWarn {
				t.Errorf("ShouldRunSweep() warning = %q, want warning present: %v", warn, tt.wantWarn)
			}
		})
	}
}

func TestSweeper_RunOnce_SuccessOrder(t *testing.T) {
	var executionOrder []string
	var mu sync.Mutex

	curatedFunc := func(dbPath, project string) (SweepCounts, error) {
		mu.Lock()
		executionOrder = append(executionOrder, "curated")
		mu.Unlock()
		return SweepCounts{Fetched: 10, Ingested: 5, Skipped: 4, Errored: 1}, nil
	}

	capsuleFunc := func(tasksRoot, project string) (SweepCounts, error) {
		mu.Lock()
		executionOrder = append(executionOrder, "capsules")
		mu.Unlock()
		return SweepCounts{Fetched: 8, Ingested: 8, Skipped: 0, Errored: 0}, nil
	}

	sweeper := &Sweeper{
		QuorumDBPath:      "test.db",
		TasksRoot:         ".ai/tasks",
		Project:           "testproj",
		CuratedImportFunc: curatedFunc,
		CapsuleImportFunc: capsuleFunc,
	}

	res, err := sweeper.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Skipped {
		t.Fatalf("expected sweep not to be skipped, got skip reason %q", res.SkipReason)
	}

	if len(executionOrder) != 2 || executionOrder[0] != "curated" || executionOrder[1] != "capsules" {
		t.Errorf("expected execution order [curated capsules], got %v", executionOrder)
	}

	if res.Curated.Ingested != 5 || res.Capsules.Ingested != 8 {
		t.Errorf("unexpected sweep counts in result: %+v", res)
	}
}

func TestSweeper_RunOnce_CuratedFailFast(t *testing.T) {
	curatedCalled := false
	capsuleCalled := false

	curatedFunc := func(dbPath, project string) (SweepCounts, error) {
		curatedCalled = true
		return SweepCounts{Fetched: 2, Errored: 2}, errors.New("db locked")
	}

	capsuleFunc := func(tasksRoot, project string) (SweepCounts, error) {
		capsuleCalled = true
		return SweepCounts{Fetched: 5}, nil
	}

	sweeper := &Sweeper{
		CuratedImportFunc: curatedFunc,
		CapsuleImportFunc: capsuleFunc,
	}

	res, err := sweeper.RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected error from RunOnce, got nil")
	}

	if !curatedCalled {
		t.Error("expected CuratedImportFunc to be called")
	}
	if capsuleCalled {
		t.Error("expected CapsuleImportFunc NOT to be called after curated failure (fail-fast)")
	}
	if res.Curated.Errored != 2 {
		t.Errorf("expected Curated.Errored 2, got %d", res.Curated.Errored)
	}
}

func TestSweeper_RunOnce_OverlapSkip(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var startOnce sync.Once

	blockingCurated := func(dbPath, project string) (SweepCounts, error) {
		startOnce.Do(func() {
			close(started)
		})
		<-release
		return SweepCounts{Ingested: 1}, nil
	}

	sweeper := &Sweeper{
		CuratedImportFunc: blockingCurated,
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = sweeper.RunOnce(context.Background())
	}()

	<-started

	// Second concurrent call while first is running
	res2, err := sweeper.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error on overlap check: %v", err)
	}
	if !res2.Skipped || res2.SkipReason != "overlap" {
		t.Errorf("expected overlap skip, got %+v", res2)
	}

	close(release)
	wg.Wait()

	// Third call after first finishes should succeed
	res3, err := sweeper.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error on subsequent call: %v", err)
	}
	if res3.Skipped {
		t.Errorf("expected non-skipped result after completion, got %+v", res3)
	}
}

func TestSweeper_RunOnce_MissingSourceProbe(t *testing.T) {
	probeCount := 0
	probeFunc := func(dbPath, tasksRoot string) error {
		probeCount++
		return errors.New("quorum DB missing")
	}

	imported := false
	curatedFunc := func(dbPath, project string) (SweepCounts, error) {
		imported = true
		return SweepCounts{}, nil
	}

	sweeper := &Sweeper{
		QuorumDBPath:      "missing.db",
		TasksRoot:         ".ai/tasks",
		Probe:             probeFunc,
		CuratedImportFunc: curatedFunc,
	}

	res1, err := sweeper.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res1.Skipped {
		t.Error("expected sweep to be skipped due to missing source")
	}

	res2, err := sweeper.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error on second call: %v", err)
	}
	if !res2.Skipped {
		t.Error("expected second sweep to also be skipped")
	}

	if probeCount != 1 {
		t.Errorf("expected probe to be called exactly once, got %d", probeCount)
	}
	if imported {
		t.Error("expected import func never to be called when probe fails")
	}
}

func TestSweeper_RunOnce_TransientCuratedError(t *testing.T) {
	attempts := 0
	curatedFunc := func(dbPath, project string) (SweepCounts, error) {
		attempts++
		if attempts == 1 {
			return SweepCounts{}, errors.New("transient database lock")
		}
		return SweepCounts{Ingested: 3}, nil
	}

	sweeper := &Sweeper{
		CuratedImportFunc: curatedFunc,
	}

	// First run fails with error
	_, err1 := sweeper.RunOnce(context.Background())
	if err1 == nil {
		t.Fatal("expected error on attempt 1")
	}

	// Second run succeeds
	res2, err2 := sweeper.RunOnce(context.Background())
	if err2 != nil {
		t.Fatalf("unexpected error on attempt 2: %v", err2)
	}
	if res2.Curated.Ingested != 3 {
		t.Errorf("expected 3 ingested on attempt 2, got %d", res2.Curated.Ingested)
	}
}

func TestSweeper_Start_TickerOverlapSkip(t *testing.T) {
	var calls int32
	release := make(chan struct{})
	firstCallStarted := make(chan struct{})
	var startOnce sync.Once

	blockingCurated := func(dbPath, project string) (SweepCounts, error) {
		atomic.AddInt32(&calls, 1)
		startOnce.Do(func() {
			close(firstCallStarted)
		})
		<-release
		return SweepCounts{Ingested: 1}, nil
	}

	sweeper := &Sweeper{
		CuratedImportFunc: blockingCurated,
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		sweeper.Start(ctx, 5*time.Millisecond)
		close(done)
	}()

	// Wait for the first tick to enter the blocking import func.
	select {
	case <-firstCallStarted:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("first tick never invoked the import func")
	}

	// Let several more ticks fire while the first call is still blocked.
	time.Sleep(50 * time.Millisecond)

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected import func to run exactly once while blocked, got %d calls", got)
	}

	// Release the blocked call and cancel the context; Start must exit cleanly.
	close(release)
	cancel()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("sweeper.Start did not stop promptly on context cancel after release")
	}
}

func TestSweeper_Start_ContextCancel(t *testing.T) {
	runs := 0
	var mu sync.Mutex

	curatedFunc := func(dbPath, project string) (SweepCounts, error) {
		mu.Lock()
		runs++
		mu.Unlock()
		return SweepCounts{Ingested: 1}, nil
	}

	sweeper := &Sweeper{
		CuratedImportFunc: curatedFunc,
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		sweeper.Start(ctx, 10*time.Millisecond)
		close(done)
	}()

	time.Sleep(35 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("sweeper.Start did not stop promptly on context cancel")
	}

	mu.Lock()
	r := runs
	mu.Unlock()

	if r == 0 {
		t.Error("expected at least 1 sweep run before cancellation")
	}
}
