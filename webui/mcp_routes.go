package webui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	configuration "httpStackLens/configuration"
	"httpStackLens/mcp"
	"httpStackLens/webui/wasm/shared"
)

// MCPRuntime is the MCP listener as the MCP tab sees it. State, settings and
// trace are read directly; every change goes through the runtime supervisor,
// which also writes it back to config.yaml.
type MCPRuntime interface {
	State() shared.McpStateDto
	Config() configuration.MCPConfig
	TracedCalls() []shared.McpCallDto
	ClearCalls()
}

// minTokenLength rejects a token typed by hand that would be easy to guess.
// Generated tokens are 64 hex characters.
const minTokenLength = 16

func registerMCPRoutes(mux *http.ServeMux, runtime MCPRuntime, send func(RuntimeCommand) RuntimeCommandResult) {
	available := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if runtime == nil {
				http.Error(w, "the MCP server is unavailable", http.StatusServiceUnavailable)
				return
			}
			next(w, r)
		}
	}
	mux.HandleFunc("GET /api/mcp/state", available(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, runtime.State())
	}))
	mux.HandleFunc("POST /api/mcp/start", available(mcpRunHandler(runtime, send, StartMCP)))
	mux.HandleFunc("POST /api/mcp/stop", available(mcpRunHandler(runtime, send, StopMCP)))
	mux.HandleFunc("GET /api/mcp/settings", available(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, mcpSettingsDto(runtime.Config()))
	}))
	mux.HandleFunc("PUT /api/mcp/settings", available(mcpSettingsHandler(runtime, send)))
	mux.HandleFunc("POST /api/mcp/token", available(mcpTokenHandler(runtime, send)))
	mux.HandleFunc("GET /api/mcp/calls", available(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, shared.McpCallsDto{Calls: runtime.TracedCalls(), Capacity: mcp.CallLogCapacity})
	}))
	mux.HandleFunc("DELETE /api/mcp/calls", available(func(w http.ResponseWriter, r *http.Request) {
		runtime.ClearCalls()
		w.WriteHeader(http.StatusNoContent)
	}))
}

// mcpRunHandler starts or stops the listener. A listener that cannot start
// answers 500 with the reason, which the state also carries as last_error.
func mcpRunHandler(runtime MCPRuntime, send func(RuntimeCommand) RuntimeCommandResult, kind RuntimeCommandKind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := send(RuntimeCommand{Kind: kind}).Err; err != nil {
			log.Printf("Error changing the MCP server state: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, runtime.State())
	}
}

func mcpSettingsHandler(runtime MCPRuntime, send func(RuntimeCommand) RuntimeCommandResult) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var dto shared.McpSettingsDto
		decoder := json.NewDecoder(io.LimitReader(r.Body, 4096))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&dto); err != nil {
			http.Error(w, "invalid MCP settings", http.StatusBadRequest)
			return
		}
		next, err := mcpSettingsFromDto(dto)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		applyMCPSettings(w, runtime, send, next)
	}
}

// mcpTokenHandler replaces the token with a new random one. Clients using the
// old token are rejected from their next request on.
func mcpTokenHandler(runtime MCPRuntime, send func(RuntimeCommand) RuntimeCommandResult) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, err := mcp.NewToken()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		next := runtime.Config()
		next.Token = token
		applyMCPSettings(w, runtime, send, next)
	}
}

func applyMCPSettings(w http.ResponseWriter, runtime MCPRuntime, send func(RuntimeCommand) RuntimeCommandResult, next configuration.MCPConfig) {
	if err := send(RuntimeCommand{Kind: SetMCPSettings, MCP: next}).Err; err != nil {
		log.Printf("Error applying MCP settings: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, mcpSettingsDto(runtime.Config()))
}

func mcpSettingsDto(config configuration.MCPConfig) shared.McpSettingsDto {
	return shared.McpSettingsDto{Port: config.Port, RequireToken: config.RequireToken, Token: config.Token}
}

// mcpSettingsFromDto validates the settings. Requiring a token without giving
// one generates it: switching protection on is then a single click.
func mcpSettingsFromDto(dto shared.McpSettingsDto) (configuration.MCPConfig, error) {
	if dto.Port < 1 || dto.Port > 65535 {
		return configuration.MCPConfig{}, fmt.Errorf("port must be between 1 and 65535")
	}
	token := strings.TrimSpace(dto.Token)
	if token != "" {
		if len(token) < minTokenLength {
			return configuration.MCPConfig{}, fmt.Errorf("token must be at least %d characters", minTokenLength)
		}
		for _, r := range token {
			if r < 0x21 || r > 0x7e {
				return configuration.MCPConfig{}, errors.New("token may only contain visible ASCII characters, without spaces")
			}
		}
	}
	if dto.RequireToken && token == "" {
		generated, err := mcp.NewToken()
		if err != nil {
			return configuration.MCPConfig{}, err
		}
		token = generated
	}
	return configuration.MCPConfig{Port: dto.Port, RequireToken: dto.RequireToken, Token: token}, nil
}
