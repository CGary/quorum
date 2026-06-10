package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hsme/core/src/bootstrap"
	"github.com/hsme/core/src/core/worker"
	"github.com/hsme/core/src/observability"
)

func main() {
	cfg := bootstrap.LoadFromEnv()
	flag.Parse()
	cfg.ApplyFlagOverrides(flag.CommandLine)
	db, embedder, extractor, err := bootstrap.OpenWithWorker(cfg)
	if err != nil {
		log.Fatalf("Bootstrap failed: %v", err)
	}
	defer db.Close()

	obsCfg := observability.LoadConfigFromEnv()
	recorder := observability.NewSQLiteRecorder(db, obsCfg)
	w := worker.NewWorker(db, embedder, extractor, recorder)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fmt.Printf("Worker HSME independiente iniciado (DB: %s)\n", cfg.DBPath)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nApagando worker...")
		cancel()
		os.Exit(0)
	}()

	for {
		leaseStarted := time.Now().UTC()
		task, err := w.LeaseNextTask(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error al arrendar tarea: %v\n", err)
			if recorder.Enabled() {
				_, lctx := recorder.StartTrace(ctx, observability.StartTraceArgs{TraceKind: "worker_task", Component: "worker", Operation: "lease_task", StartedAt: leaseStarted})
				_ = recorder.RecordError(lctx, observability.ErrorEvent{Component: "worker", Operation: "lease_task", Severity: "error", Message: err.Error()})
			}
			time.Sleep(5 * time.Second)
			continue
		}
		if task != nil {
			fmt.Printf("[%s] Ejecutando tarea %d (Tipo: %s, Memoria: %d)...\n", time.Now().Format("15:04:05"), task.ID, task.TaskType, task.MemoryID)
			if recorder.Enabled() {
				trace, lctx := recorder.StartTrace(ctx, observability.StartTraceArgs{TraceKind: "worker_task", Component: "worker", Operation: "lease_task", TaskID: task.ID, TaskType: task.TaskType, MemoryID: task.MemoryID, StartedAt: leaseStarted})
				span, _ := recorder.StartSpan(lctx, observability.StartSpanArgs{TraceID: trace.TraceID, Component: "worker", Operation: "lease_task", StageName: "lease_query", StartedAt: leaseStarted})
				_ = recorder.FinishSpan(lctx, span, observability.SpanResult{Status: "ok", EndedAt: time.Now().UTC()})
				_ = recorder.FinishTrace(lctx, trace, observability.TraceResult{Status: "ok", EndedAt: time.Now().UTC()})
			}
			if err := w.ExecuteTask(ctx, task); err != nil {
				fmt.Fprintf(os.Stderr, "Error ejecutando tarea %d: %v\n", task.ID, err)
				// Persistir el error y liberar el lease para reintento (o marcar failed si se agotaron intentos).
				const maxAttempts = 5
				nextStatus := "pending"
				if task.AttemptCount >= maxAttempts {
					nextStatus = "failed"
				}
				if _, uerr := db.Exec(
					"UPDATE async_tasks SET status = ?, last_error = ?, leased_until = NULL, updated_at = ? WHERE id = ?",
					nextStatus, err.Error(), time.Now().Format(time.RFC3339), task.ID,
				); uerr != nil {
					fmt.Fprintf(os.Stderr, "Error registrando fallo de tarea %d: %v\n", task.ID, uerr)
				}
			} else {
				fmt.Printf("[%s] Tarea %d completada con éxito\n", time.Now().Format("15:04:05"), task.ID)
			}
		} else {
			time.Sleep(2 * time.Second)
		}
	}
}
