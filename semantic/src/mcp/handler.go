package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/hsme/core/src/observability"
)

type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

type Server struct {
	tools      map[string]ToolDefinition
	writeMutex sync.Mutex
	wg         sync.WaitGroup
	recorder   observability.Recorder
}

type ToolDefinition struct {
	Tool    Tool
	Handler func(json.RawMessage) (interface{}, error)
}

type requestMetrics struct {
	readDuration  time.Duration
	parseDuration time.Duration
	bytes         int
}

func NewServer() *Server {
	return &Server{tools: make(map[string]ToolDefinition)}
}

func (s *Server) SetRecorder(recorder observability.Recorder) { s.recorder = recorder }

func (s *Server) RegisterTool(name string, description string, schema interface{}, handler func(json.RawMessage) (interface{}, error)) {
	s.tools[name] = ToolDefinition{Tool: Tool{Name: name, Description: description, InputSchema: schema}, Handler: handler}
}

func (s *Server) Serve() {
	fmt.Fprintf(os.Stderr, "HSME MCP server starting (v1.0.1)...\n")
	reader := bufio.NewReader(os.Stdin)
	for {
		readStarted := time.Now().UTC()
		line, err := reader.ReadString('\n')
		readDuration := time.Since(readStarted)
		if err != nil {
			if err == io.EOF {
				fmt.Fprintf(os.Stderr, "Standard input closed (EOF). Waiting for in-flight requests...\n")
				s.wg.Wait()
				return
			}
			fmt.Fprintf(os.Stderr, "Error reading from stdin after %s: %v\n", readDuration, err)
			continue
		}
		if len(line) == 0 {
			continue
		}
		parseStarted := time.Now().UTC()
		var req JSONRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing JSON-RPC request after %s (read=%s): %v | Raw: %s\n", time.Since(parseStarted), readDuration, err, line)
			continue
		}
		parseDuration := time.Since(parseStarted)
		requestStarted := time.Now().UTC()
		fmt.Fprintf(os.Stderr, "Received request: method=%s id=%s bytes=%d read=%s parse=%s\n", req.Method, string(req.ID), len(line), readDuration, parseDuration)
		metrics := requestMetrics{readDuration: readDuration, parseDuration: parseDuration, bytes: len(line)}
		s.wg.Add(1)
		go func(r JSONRPCRequest, started time.Time, m requestMetrics) {
			defer s.wg.Done()
			s.handleRequest(r, started, m)
		}(req, requestStarted, metrics)
	}
}

func (s *Server) sendResponse(resp JSONRPCResponse) (time.Duration, time.Duration, time.Duration, error) {
	sendStarted := time.Now().UTC()
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()
	marshalStarted := time.Now().UTC()
	data, err := json.Marshal(resp)
	marshalDuration := time.Since(marshalStarted)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling response: %v\n", err)
		return marshalDuration, 0, time.Since(sendStarted), err
	}
	writeStarted := time.Now().UTC()
	_, err = os.Stdout.Write(data)
	if err == nil {
		_, err = os.Stdout.Write([]byte("\n"))
	}
	writeDuration := time.Since(writeStarted)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing response: %v\n", err)
		return marshalDuration, writeDuration, time.Since(sendStarted), err
	}
	fmt.Fprintf(os.Stderr, "Sent response: id=%s bytes=%d marshal=%s write=%s total_send=%s\n", string(resp.ID), len(data)+1, marshalDuration, writeDuration, time.Since(sendStarted))
	return marshalDuration, writeDuration, time.Since(sendStarted), nil
}

func (s *Server) handleRequest(req JSONRPCRequest, started time.Time, metrics requestMetrics) {
	isNotification := req.ID == nil || string(req.ID) == "null"
	var resp JSONRPCResponse
	resp.JSONRPC = "2.0"
	resp.ID = req.ID
	handlerStarted := time.Now().UTC()
	trace, ctx := s.startTrace(req, started, metrics)
	stage := "dispatch"

	switch req.Method {
	case "initialize":
		stage = "initialize"
		resp.Result = map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{"tools": map[string]interface{}{"listChanged": false}},
			"serverInfo":      map[string]interface{}{"name": "hsme", "version": "1.0.1"},
		}
	case "notifications/initialized":
		fmt.Fprintf(os.Stderr, "Handshake complete in %s\n", time.Since(started))
		s.finishTrace(ctx, trace, observability.TraceResult{Status: "ok", EndedAt: time.Now().UTC()})
		return
	case "tools/list", "list_tools":
		stage = "tools/list"
		var tools []Tool
		for _, def := range s.tools {
			tools = append(tools, def.Tool)
		}
		resp.Result = map[string]interface{}{"tools": tools}
	case "tools/call", "call_tool":
		stage = "tools/call.decode"
		callStarted := time.Now().UTC()
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp.Error = &JSONRPCError{Code: -32602, Message: "Invalid params"}
			s.recordError(ctx, trace, "mcp", req.Method, err)
			fmt.Fprintf(os.Stderr, "Tool call decode failed: id=%s after %s: %v\n", string(req.ID), time.Since(callStarted), err)
		} else {
			trace.ToolName = params.Name
			s.recordCompletedSpan(ctx, trace, "mcp", req.Method, "decode_tool_args", callStarted, observability.SpanResult{Status: "ok", EndedAt: time.Now().UTC(), BytesIn: int64(len(params.Arguments))})
			fmt.Fprintf(os.Stderr, "Tool call decoded: id=%s tool=%s decode=%s args_bytes=%d\n", string(req.ID), params.Name, time.Since(callStarted), len(params.Arguments))
			if def, ok := s.tools[params.Name]; ok {
				stage = "tools/call.handler"
				toolStarted := time.Now().UTC()
				result, err := def.Handler(params.Arguments)
				toolDuration := time.Since(toolStarted)
				spanResult := observability.SpanResult{Status: "ok", EndedAt: time.Now().UTC()}
				if err != nil {
					spanResult.Status = "error"
					spanResult.ErrorMessage = err.Error()
				}
				s.recordCompletedSpan(ctx, trace, "mcp", params.Name, "tool_handler", toolStarted, spanResult)
				if err != nil {
					resp.Error = &JSONRPCError{Code: -32000, Message: err.Error()}
					s.recordError(ctx, trace, "mcp", params.Name, err)
					fmt.Fprintf(os.Stderr, "Tool call failed: id=%s tool=%s handler=%s err=%v\n", string(req.ID), params.Name, toolDuration, err)
				} else {
					stage = "tools/call.format"
					formatStarted := time.Now().UTC()
					formatted := formatResult(result)
					formatDuration := time.Since(formatStarted)
					s.recordCompletedSpan(ctx, trace, "mcp", params.Name, "format_response", formatStarted, observability.SpanResult{Status: "ok", EndedAt: time.Now().UTC(), BytesOut: int64(len(formatted))})
					resp.Result = map[string]interface{}{"content": []interface{}{map[string]interface{}{"type": "text", "text": formatted}}}
					fmt.Fprintf(os.Stderr, "Tool call completed: id=%s tool=%s handler=%s format=%s total=%s\n", string(req.ID), params.Name, toolDuration, formatDuration, time.Since(started))
				}
			} else {
				resp.Error = &JSONRPCError{Code: -32601, Message: "Tool not found"}
				s.recordError(ctx, trace, "mcp", req.Method, fmt.Errorf("tool not found: %s", params.Name))
				fmt.Fprintf(os.Stderr, "Tool call failed: id=%s unknown_tool=%s\n", string(req.ID), params.Name)
			}
		}
	case "ping":
		stage = "ping"
		resp.Result = map[string]interface{}{}
	default:
		if isNotification {
			s.finishTrace(ctx, trace, observability.TraceResult{Status: "ok", EndedAt: time.Now().UTC()})
			return
		}
		stage = "unknown_method"
		resp.Error = &JSONRPCError{Code: -32601, Message: fmt.Sprintf("Method not found: %s", req.Method)}
		s.recordError(ctx, trace, "mcp", req.Method, fmt.Errorf("method not found: %s", req.Method))
	}

	if !isNotification {
		fmt.Fprintf(os.Stderr, "Request ready for response: method=%s id=%s stage=%s handler_total=%s total=%s\n", req.Method, string(req.ID), stage, time.Since(handlerStarted), time.Since(started))
		marshalDur, writeDur, _, err := s.sendResponse(resp)
		s.recordCompletedSpan(ctx, trace, "mcp", req.Method, "write_response", time.Now().UTC().Add(-(marshalDur + writeDur)), observability.SpanResult{Status: boolStatus(err == nil), EndedAt: time.Now().UTC()})
		traceResult := observability.TraceResult{Status: "ok", EndedAt: time.Now().UTC()}
		if resp.Error != nil {
			traceResult.Status = "error"
			traceResult.ErrorCode = fmt.Sprintf("%d", resp.Error.Code)
			traceResult.ErrorMessage = resp.Error.Message
		}
		s.finishTrace(ctx, trace, traceResult)
	}
}

func (s *Server) startTrace(req JSONRPCRequest, started time.Time, metrics requestMetrics) (observability.TraceContext, context.Context) {
	if s.recorder == nil || !s.recorder.Enabled() {
		return observability.TraceContext{RequestID: string(req.ID), Operation: req.Method, StartedAtUTC: started}, context.Background()
	}
	trace, ctx := s.recorder.StartTrace(context.Background(), observability.StartTraceArgs{
		TraceKind: "mcp_request",
		Component: "mcp",
		Operation: req.Method,
		RequestID: string(req.ID),
		StartedAt: started.Add(-(metrics.readDuration + metrics.parseDuration)),
		Meta:      map[string]any{"bytes": metrics.bytes},
	})
	if metrics.readDuration > 0 {
		s.recordCompletedSpan(ctx, trace, "mcp", req.Method, "read_stdin", trace.StartedAtUTC, observability.SpanResult{Status: "ok", EndedAt: trace.StartedAtUTC.Add(metrics.readDuration), BytesIn: int64(metrics.bytes)})
	}
	if metrics.parseDuration > 0 {
		s.recordCompletedSpan(ctx, trace, "mcp", req.Method, "parse_json", started.Add(-metrics.parseDuration), observability.SpanResult{Status: "ok", EndedAt: started})
	}
	return trace, ctx
}

func (s *Server) finishTrace(ctx context.Context, trace observability.TraceContext, result observability.TraceResult) {
	if s.recorder != nil && s.recorder.Enabled() {
		_ = s.recorder.FinishTrace(ctx, trace, result)
	}
}

func (s *Server) recordCompletedSpan(ctx context.Context, trace observability.TraceContext, component, operation, stage string, started time.Time, result observability.SpanResult) {
	if s.recorder == nil || !s.recorder.Enabled() {
		return
	}
	span, _ := s.recorder.StartSpan(ctx, observability.StartSpanArgs{TraceID: trace.TraceID, Component: component, Operation: operation, StageName: stage, StartedAt: started})
	_ = s.recorder.FinishSpan(ctx, span, result)
}

func (s *Server) recordError(ctx context.Context, trace observability.TraceContext, component, operation string, err error) {
	if s.recorder == nil || !s.recorder.Enabled() || err == nil {
		return
	}
	_ = s.recorder.RecordError(ctx, observability.ErrorEvent{TraceID: trace.TraceID, ToolName: trace.ToolName, TaskID: trace.TaskID, TaskType: trace.TaskType, MemoryID: trace.MemoryID, Component: component, Operation: operation, Severity: "error", Message: err.Error()})
}

func boolStatus(ok bool) string {
	if ok {
		return "ok"
	}
	return "error"
}

func formatResult(v interface{}) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
