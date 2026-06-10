package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hsme/core/src/bootstrap"
	"github.com/hsme/core/src/observability"
)
func main() {
	bCfg := bootstrap.LoadFromEnv()
	flag.Parse()
	bCfg.ApplyFlagOverrides(flag.CommandLine)
	db, err := bootstrap.OpenDB(bCfg)
	if err != nil {
		log.Fatalf("Bootstrap failed: %v", err)
	}
	defer db.Close()

	obsCfg := observability.LoadConfigFromEnv()
	recorder := observability.NewSQLiteRecorder(db, obsCfg)
	ctx := context.Background()
	mode := "once"
	if flag.NArg() > 0 {
		mode = flag.Arg(0)
	}

	switch mode {
	case "once":
		runOnce(ctx, recorder)
	case "summary":
		printSummary(ctx, recorder)
	case "loop":
		for {
			runOnce(ctx, recorder)
			time.Sleep(obsCfg.FlushInterval)
		}
	default:
		fmt.Fprintf(os.Stderr, "uso: hsme-ops [once|summary|loop]\n")
		os.Exit(1)
	}
}

func runOnce(ctx context.Context, recorder observability.Recorder) {
	trace, ctx := recorder.StartTrace(ctx, observability.StartTraceArgs{TraceKind: "maintenance", Component: "ops", Operation: "maintenance_cycle", StartedAt: time.Now().UTC()})
	if err := recorder.FlushRollups(ctx, time.Now().UTC()); err != nil {
		_ = recorder.RecordError(ctx, observability.ErrorEvent{TraceID: trace.TraceID, Component: "ops", Operation: "flush_rollups", Severity: "error", Message: err.Error()})
		_ = recorder.FinishTrace(ctx, trace, observability.TraceResult{Status: "error", ErrorMessage: err.Error(), EndedAt: time.Now().UTC()})
		log.Fatalf("error ejecutando rollups: %v", err)
	}
	if err := recorder.RunRetention(ctx, time.Now().UTC()); err != nil {
		_ = recorder.RecordError(ctx, observability.ErrorEvent{TraceID: trace.TraceID, Component: "ops", Operation: "run_retention", Severity: "error", Message: err.Error()})
		_ = recorder.FinishTrace(ctx, trace, observability.TraceResult{Status: "error", ErrorMessage: err.Error(), EndedAt: time.Now().UTC()})
		log.Fatalf("error ejecutando retención: %v", err)
	}
	_ = recorder.FinishTrace(ctx, trace, observability.TraceResult{Status: "ok", EndedAt: time.Now().UTC()})
	fmt.Println("ops cycle complete")
}

func printSummary(ctx context.Context, recorder observability.Recorder) {
	slow, _ := recorder.RecentSlowOperations(ctx, 10)
	errs, _ := recorder.RecentErrorEvents(ctx, 10)
	rollups, _ := recorder.RollupHealth(ctx, 10)
	fmt.Println("== Slow operations ==")
	for _, s := range slow {
		fmt.Printf("%s | %s.%s | observed=%dus threshold=%dus | trace=%s\n", s.CreatedAtUTC, s.Component, s.Operation, s.ObservedUS, s.ThresholdUS, s.TraceID)
	}
	fmt.Println("== Error events ==")
	for _, e := range errs {
		fmt.Printf("%s | %s.%s | %s | trace=%s\n", e.CreatedAtUTC, e.Component, e.Operation, e.Message, e.TraceID)
	}
	fmt.Println("== Rollup health ==")
	for _, r := range rollups {
		fmt.Printf("%s %s | %s.%s | total=%d errors=%d slow=%d p95=%dus\n", r.BucketLevel, r.BucketStartUTC, r.Component, r.Operation, r.TotalCount, r.ErrorCount, r.SlowCount, r.P95US)
	}
}
