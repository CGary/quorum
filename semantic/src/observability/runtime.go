package observability

import (
	"context"
	"time"
)

func RunSpan(ctx context.Context, recorder Recorder, trace TraceContext, component, operation, stage string, meta map[string]any, fn func(context.Context) error) error {
	if recorder == nil || !recorder.Enabled() {
		return fn(ctx)
	}
	span, spanCtx := recorder.StartSpan(ctx, StartSpanArgs{TraceID: trace.TraceID, Component: component, Operation: operation, StageName: stage, Meta: meta, StartedAt: time.Now().UTC()})
	err := fn(spanCtx)
	result := SpanResult{Status: "ok", EndedAt: time.Now().UTC()}
	if err != nil {
		result.Status = "error"
		result.ErrorMessage = err.Error()
		_ = recorder.RecordError(spanCtx, ErrorEvent{TraceID: trace.TraceID, SpanID: span.SpanID, Component: component, Operation: operation, Severity: "error", Message: err.Error()})
	}
	_ = recorder.FinishSpan(spanCtx, span, result)
	return err
}
