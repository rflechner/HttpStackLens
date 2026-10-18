//go:build js && wasm

package mcppanel

import (
	"encoding/json"
	"errors"
	"strings"

	"httpStackLens/webui/wasm/dom"
	"httpStackLens/webui/wasm/shared"
)

// The MCP routes of the Web UI API (see openapi.yaml). Every change goes
// through the backend, which writes it to the mcp section of config.yaml.
const (
	statePath    = "/api/mcp/state"
	startPath    = "/api/mcp/start"
	stopPath     = "/api/mcp/stop"
	settingsPath = "/api/mcp/settings"
	tokenPath    = "/api/mcp/token"
	callsPath    = "/api/mcp/calls"
)

// apiCall sends body as JSON when it is not nil and decodes the answer into
// out when it is not nil. An error answer comes back as its text: the backend
// answers in plain words, such as the port already in use.
func apiCall(method, path string, body, out any) error {
	var headers map[string]string
	payload := ""
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = string(data)
		headers = map[string]string{"Content-Type": "application/json"}
	}
	res, err := dom.Fetch(method, path, headers, payload)
	if err != nil {
		return errors.New("could not reach HttpStackLens: " + err.Error())
	}
	if res.Status < 200 || res.Status > 299 {
		msg := strings.TrimSpace(res.Body)
		if msg == "" {
			msg = res.StatusText
		}
		return errors.New(msg)
	}
	if out == nil || res.Body == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(res.Body), out); err != nil {
		return errors.New("unreadable answer from HttpStackLens: " + err.Error())
	}
	return nil
}

func fetchState() (shared.McpStateDto, error) {
	var state shared.McpStateDto
	err := apiCall("GET", statePath, nil, &state)
	return state, err
}

func fetchSettings() (shared.McpSettingsDto, error) {
	var settings shared.McpSettingsDto
	err := apiCall("GET", settingsPath, nil, &settings)
	return settings, err
}

func fetchCalls() (shared.McpCallsDto, error) {
	var calls shared.McpCallsDto
	err := apiCall("GET", callsPath, nil, &calls)
	return calls, err
}
