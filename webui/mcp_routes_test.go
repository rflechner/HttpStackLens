package webui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	configuration "httpStackLens/configuration"
	"httpStackLens/webui/wasm/shared"
)

// fakeMCPRuntime plays the controller and the supervisor at once: commands
// change its configuration directly, or fail with failWith.
type fakeMCPRuntime struct {
	config   configuration.MCPConfig
	calls    []shared.McpCallDto
	failWith error
	commands []RuntimeCommand
}

func (f *fakeMCPRuntime) State() shared.McpStateDto {
	return shared.McpStateDto{Enabled: f.config.Enabled, Running: f.config.Enabled, Port: f.config.Port, RequireToken: f.config.RequireToken, TokenConfigured: f.config.Token != "", CallCount: len(f.calls)}
}
func (f *fakeMCPRuntime) Config() configuration.MCPConfig  { return f.config }
func (f *fakeMCPRuntime) TracedCalls() []shared.McpCallDto { return f.calls }
func (f *fakeMCPRuntime) ClearCalls()                      { f.calls = nil }
func (f *fakeMCPRuntime) Tools(context.Context) ([]shared.McpToolDto, error) {
	return []shared.McpToolDto{{Name: "get_status", Description: "Read the state", ReadOnly: true, InputSchema: json.RawMessage(`{"type":"object"}`)}}, nil
}

func (f *fakeMCPRuntime) send(command RuntimeCommand) RuntimeCommandResult {
	f.commands = append(f.commands, command)
	if f.failWith != nil {
		return RuntimeCommandResult{Err: f.failWith}
	}
	switch command.Kind {
	case StartMCP:
		f.config.Enabled = true
	case StopMCP:
		f.config.Enabled = false
	case SetMCPSettings:
		f.config.Port, f.config.RequireToken, f.config.Token = command.MCP.Port, command.MCP.RequireToken, command.MCP.Token
	}
	return RuntimeCommandResult{}
}

func serveMCP(t *testing.T, runtime *fakeMCPRuntime, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	if runtime == nil {
		registerMCPRoutes(mux, nil, nil)
	} else {
		registerMCPRoutes(mux, runtime, runtime.send)
	}
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(method, path, strings.NewReader(body)))
	return res
}

func TestMCPRoutesUnavailable(t *testing.T) {
	if res := serveMCP(t, nil, http.MethodGet, "/api/mcp/state", ""); res.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", res.Code)
	}
}

func TestMCPStartStop(t *testing.T) {
	runtime := &fakeMCPRuntime{config: configuration.MCPConfig{Port: 8087}}
	res := serveMCP(t, runtime, http.MethodPost, "/api/mcp/start", "")
	var state shared.McpStateDto
	if err := json.Unmarshal(res.Body.Bytes(), &state); err != nil || res.Code != http.StatusOK || !state.Running {
		t.Fatalf("start = %d %s", res.Code, res.Body)
	}
	if res := serveMCP(t, runtime, http.MethodPost, "/api/mcp/stop", ""); res.Code != http.StatusOK || runtime.config.Enabled {
		t.Fatalf("stop = %d %s", res.Code, res.Body)
	}

	runtime.failWith = errors.New("start MCP listener: address already in use")
	res = serveMCP(t, runtime, http.MethodPost, "/api/mcp/start", "")
	if res.Code != http.StatusInternalServerError || !strings.Contains(res.Body.String(), "address already in use") {
		t.Fatalf("failed start = %d %s", res.Code, res.Body)
	}
}

func TestMCPSettings(t *testing.T) {
	runtime := &fakeMCPRuntime{config: configuration.MCPConfig{Enabled: true, Port: 8087}}

	// Requiring a token without one generates it.
	res := serveMCP(t, runtime, http.MethodPut, "/api/mcp/settings", `{"port":9091,"require_token":true,"token":""}`)
	var settings shared.McpSettingsDto
	if err := json.Unmarshal(res.Body.Bytes(), &settings); err != nil || res.Code != http.StatusOK {
		t.Fatalf("put = %d %s", res.Code, res.Body)
	}
	if settings.Port != 9091 || !settings.RequireToken || len(settings.Token) != 64 {
		t.Fatalf("settings = %+v", settings)
	}
	if !runtime.config.Enabled {
		t.Fatal("settings must not stop the listener")
	}

	// A new token replaces the old one and keeps the rest.
	old := settings.Token
	res = serveMCP(t, runtime, http.MethodPost, "/api/mcp/token", "")
	if err := json.Unmarshal(res.Body.Bytes(), &settings); err != nil || res.Code != http.StatusOK {
		t.Fatalf("token = %d %s", res.Code, res.Body)
	}
	if settings.Token == old || len(settings.Token) != 64 || settings.Port != 9091 || !settings.RequireToken {
		t.Fatalf("regenerated settings = %+v", settings)
	}

	res = serveMCP(t, runtime, http.MethodGet, "/api/mcp/settings", "")
	if !strings.Contains(res.Body.String(), settings.Token) {
		t.Fatalf("get settings = %s", res.Body)
	}
	// The state never carries the token.
	res = serveMCP(t, runtime, http.MethodGet, "/api/mcp/state", "")
	if strings.Contains(res.Body.String(), settings.Token) || !strings.Contains(res.Body.String(), `"token_configured":true`) {
		t.Fatalf("state = %s", res.Body)
	}
}

func TestMCPSettingsValidation(t *testing.T) {
	for name, body := range map[string]string{
		"port too low":  `{"port":0,"require_token":false,"token":""}`,
		"port too high": `{"port":65536,"require_token":false,"token":""}`,
		"short token":   `{"port":8087,"require_token":true,"token":"short"}`,
		"token spaces":  `{"port":8087,"require_token":true,"token":"a token with spaces"}`,
		"unknown field": `{"port":8087,"enabled":true}`,
		"not json":      `port=8087`,
	} {
		t.Run(name, func(t *testing.T) {
			runtime := &fakeMCPRuntime{config: configuration.MCPConfig{Port: 8087}}
			res := serveMCP(t, runtime, http.MethodPut, "/api/mcp/settings", body)
			if res.Code != http.StatusBadRequest || len(runtime.commands) != 0 {
				t.Fatalf("status = %d, commands = %d", res.Code, len(runtime.commands))
			}
		})
	}
}

func TestMCPCalls(t *testing.T) {
	runtime := &fakeMCPRuntime{calls: []shared.McpCallDto{{ID: 2, Method: "tools/call", Tool: "search_requests"}, {ID: 1, Method: "initialize"}}}
	res := serveMCP(t, runtime, http.MethodGet, "/api/mcp/calls", "")
	var calls shared.McpCallsDto
	if err := json.Unmarshal(res.Body.Bytes(), &calls); err != nil || len(calls.Calls) != 2 || calls.Capacity == 0 {
		t.Fatalf("calls = %d %s", res.Code, res.Body)
	}
	if res := serveMCP(t, runtime, http.MethodDelete, "/api/mcp/calls", ""); res.Code != http.StatusNoContent || runtime.calls != nil {
		t.Fatalf("clear = %d", res.Code)
	}
}

func TestMCPTools(t *testing.T) {
	res := serveMCP(t, &fakeMCPRuntime{}, http.MethodGet, "/api/mcp/tools", "")
	var tools shared.McpToolsDto
	if err := json.Unmarshal(res.Body.Bytes(), &tools); err != nil || res.Code != http.StatusOK {
		t.Fatalf("tools = %d %s", res.Code, res.Body)
	}
	if len(tools.Tools) != 1 || !tools.Tools[0].ReadOnly || string(tools.Tools[0].InputSchema) != `{"type":"object"}` {
		t.Fatalf("tools = %+v", tools)
	}
}
