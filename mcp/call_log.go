package mcp

import (
	"context"
	"encoding/json"
	"httpStackLens/webui/wasm/shared"
	"strings"
	"sync"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// CallLogCapacity is how many traced calls are retained, oldest dropped first.
const CallLogCapacity = 200

// maxTracedBytes caps the arguments and the result kept for one call: the
// trace is there to see what a client asked and roughly what it got, not to
// hold every search page in memory.
const maxTracedBytes = 4096

// Call statuses, as shown in the Web UI.
const (
	CallOK           = "ok"
	CallToolError    = "tool_error"
	CallError        = "error"
	CallUnauthorized = "unauthorized"
)

// CallLog traces the messages the MCP listener receives. It lives in memory
// only and is never written to disk: arguments may hold whatever a client sent,
// header values of a composer request included.
type CallLog struct {
	mu     sync.Mutex
	calls  []shared.McpCallDto // oldest first
	nextID int64
	// onCall is told about every call added. Optional.
	onCall func(shared.McpCallDto)
}

func newCallLog() *CallLog { return &CallLog{} }

// add stamps the call with an id and keeps it, dropping the oldest one beyond
// CallLogCapacity.
func (l *CallLog) add(call shared.McpCallDto) {
	l.mu.Lock()
	l.nextID++
	call.ID = l.nextID
	if len(l.calls) >= CallLogCapacity {
		l.calls = append(l.calls[:0], l.calls[1:]...)
	}
	l.calls = append(l.calls, call)
	onCall := l.onCall
	l.mu.Unlock()

	if onCall != nil {
		onCall(call)
	}
}

// List returns the retained calls, newest first.
func (l *CallLog) List() []shared.McpCallDto {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]shared.McpCallDto, len(l.calls))
	for i, call := range l.calls {
		out[len(l.calls)-1-i] = call
	}
	return out
}

func (l *CallLog) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.calls)
}

func (l *CallLog) Clear() {
	l.mu.Lock()
	l.calls = nil
	l.mu.Unlock()
}

func (l *CallLog) setListener(onCall func(shared.McpCallDto)) {
	l.mu.Lock()
	l.onCall = onCall
	l.mu.Unlock()
}

// untraced reports protocol chatter: keep-alives and notifications would bury
// the calls a developer is looking for.
func untraced(method string) bool {
	return method == "ping" || strings.HasPrefix(method, "notifications/")
}

// middleware traces every request the MCP server handles. It sits inside the
// SDK, after JSON-RPC decoding, so it sees the method and typed parameters, and
// the result once the handler has run.
func (l *CallLog) middleware(next sdk.MethodHandler) sdk.MethodHandler {
	return func(ctx context.Context, method string, req sdk.Request) (sdk.Result, error) {
		if untraced(method) {
			return next(ctx, method, req)
		}
		start := time.Now()
		result, err := next(ctx, method, req)

		call := shared.McpCallDto{
			At:         start.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
			DurationMs: float64(time.Since(start).Microseconds()) / 1000,
			Method:     method,
			Status:     CallOK,
		}
		if session, ok := req.GetSession().(*sdk.ServerSession); ok && session != nil {
			call.SessionID = session.ID()
			call.Client = clientName(session.InitializeParams())
		}
		switch params := req.GetParams().(type) {
		case *sdk.CallToolParamsRaw:
			call.Tool = params.Name
			call.Arguments, call.Truncated = traceRaw(params.Arguments)
		case *sdk.InitializeParams:
			// The session records the client only once initialize has returned.
			if call.Client == "" {
				call.Client = clientName(params)
			}
		}

		var cut bool
		switch {
		case err != nil:
			call.Status = CallError
			call.Error = err.Error()
		case method == "tools/list":
			call.Result = toolNames(result)
		default:
			if res, ok := result.(*sdk.CallToolResult); ok && res != nil {
				if res.IsError {
					call.Status = CallToolError
					call.Error = firstText(res)
				}
				call.Result, cut = traceToolResult(res)
			} else {
				call.Result, cut = traceValue(result)
			}
		}
		call.Truncated = call.Truncated || cut
		l.add(call)
		return result, err
	}
}

func clientName(params *sdk.InitializeParams) string {
	if params == nil || params.ClientInfo == nil {
		return ""
	}
	info := params.ClientInfo
	if info.Version == "" {
		return info.Name
	}
	return info.Name + " " + info.Version
}

// toolNames summarises a tools/list result: the schemas themselves are the same
// on every call and only the names tell what the client was offered.
func toolNames(result sdk.Result) string {
	list, ok := result.(*sdk.ListToolsResult)
	if !ok || list == nil {
		return ""
	}
	names := make([]string, 0, len(list.Tools))
	for _, tool := range list.Tools {
		names = append(names, tool.Name)
	}
	return strings.Join(names, ", ")
}

// traceToolResult keeps the structured output when the tool has one — the text
// content then duplicates it — and the content otherwise.
func traceToolResult(res *sdk.CallToolResult) (string, bool) {
	if res.StructuredContent != nil {
		return traceValue(res.StructuredContent)
	}
	return traceValue(res.Content)
}

func firstText(res *sdk.CallToolResult) string {
	for _, content := range res.Content {
		if text, ok := content.(*sdk.TextContent); ok {
			return truncate(text.Text)
		}
	}
	return ""
}

func traceValue(value any) (string, bool) {
	if value == nil {
		return "", false
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", false
	}
	return traceRaw(data)
}

func traceRaw(data json.RawMessage) (string, bool) {
	if len(data) == 0 || string(data) == "null" {
		return "", false
	}
	text := truncate(string(data))
	return text, len(text) < len(data)
}

// truncate cuts s at maxTracedBytes without splitting a UTF-8 sequence.
func truncate(s string) string {
	if len(s) <= maxTracedBytes {
		return s
	}
	cut := maxTracedBytes
	for cut > 0 && s[cut]&0xC0 == 0x80 {
		cut--
	}
	return s[:cut]
}
