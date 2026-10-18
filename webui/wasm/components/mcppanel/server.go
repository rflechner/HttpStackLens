//go:build js && wasm

package mcppanel

import (
	_ "embed"
	"strings"
	"time"

	"httpStackLens/webui/wasm/dom"
	"httpStackLens/webui/wasm/shared"
)

//go:embed server.html
var serverHTML string

// snippetKey remembers which client configuration the pane shows.
const snippetKey = "hsl-mcp-snippet"

// The client configurations the pane can show.
const (
	snippetJSON   = "json"
	snippetClaude = "claude"
)

// ServerPane is the left column: state, start/stop, port, token and a client
// configuration ready to paste.
type ServerPane struct {
	dom.Base
	owner *Panel

	// PortDraft is the port being typed; it applies on Enter or Apply.
	PortDraft int
	// Busy names the action waiting for the backend, to disable its button.
	Busy      string
	Err       string
	ShowToken bool
	Snippet   string
	// Copied names what was just copied, for a short confirmation.
	Copied string
}

func (s *ServerPane) Template() string { return serverHTML }

func (s *ServerPane) OnInit() {
	s.Snippet = snippetJSON
	if dom.LocalGet(snippetKey) == snippetClaude {
		s.Snippet = snippetClaude
	}
}

func (s *ServerPane) syncDraft() { s.PortDraft = s.owner.Settings.Port }

// ── template accessors ─────────────────────────────────────────────────────

func (s *ServerPane) Loaded() bool                 { return s.owner.Loaded }
func (s *ServerPane) LoadErr() string              { return s.owner.LoadErr }
func (s *ServerPane) St() shared.McpStateDto       { return s.owner.State }
func (s *ServerPane) Set() shared.McpSettingsDto   { return s.owner.Settings }
func (s *ServerPane) IsBusy(action string) bool    { return s.Busy == action }
func (s *ServerPane) AnyBusy() bool                { return s.Busy != "" }
func (s *ServerPane) SnippetIs(id string) bool     { return s.Snippet == id }
func (s *ServerPane) WasCopied(what string) bool   { return s.Copied == what }
func (s *ServerPane) HasToken() bool               { return s.owner.Settings.Token != "" }

// StatusClass colours the state dot and label: running, failed (asked to run
// but could not), or stopped.
func (s *ServerPane) StatusClass() string {
	st := s.owner.State
	switch {
	case st.Running:
		return "on"
	case st.Enabled || st.LastError != "":
		return "err"
	default:
		return "off"
	}
}

func (s *ServerPane) StatusLabel() string {
	st := s.owner.State
	switch {
	case st.Running:
		return "Running"
	case st.Enabled:
		return "Not running"
	default:
		return "Stopped"
	}
}

// Since is the start time in the viewer's clock.
func (s *ServerPane) Since() string {
	if s.owner.State.StartedAt == "" {
		return ""
	}
	return localTime(s.owner.State.StartedAt, false)
}

// TokenText is the token as displayed: masked unless asked for, so that a
// screen share does not leak it.
func (s *ServerPane) TokenText() string {
	token := s.owner.Settings.Token
	if s.ShowToken || token == "" {
		return token
	}
	return maskToken(token)
}

func maskToken(token string) string {
	if len(token) <= 8 {
		return strings.Repeat("•", len(token))
	}
	return strings.Repeat("•", 12) + token[len(token)-4:]
}

// SnippetText is the client configuration, with the token masked like the
// field above it. Copy always copies the real one.
func (s *ServerPane) SnippetText() string { return s.snippet(s.TokenText()) }

func (s *ServerPane) snippet(token string) string {
	url := s.owner.State.URL
	auth := s.owner.Settings.RequireToken && s.owner.Settings.Token != ""
	if s.Snippet == snippetClaude {
		cmd := "claude mcp add --transport http httpstacklens " + url
		if auth {
			cmd += ` --header "Authorization: Bearer ` + token + `"`
		}
		return cmd
	}
	var b strings.Builder
	b.WriteString("{\n  \"mcpServers\": {\n    \"httpstacklens\": {\n      \"url\": \"" + url + "\"")
	if auth {
		b.WriteString(",\n      \"headers\": {\n        \"Authorization\": \"Bearer " + token + "\"\n      }")
	}
	b.WriteString("\n    }\n  }\n}")
	return b.String()
}

// ── handlers ───────────────────────────────────────────────────────────────

// Toggle starts or stops the listener. The new state also arrives as an
// mcp_state event; the answer is applied anyway, so that the button never
// waits on the stream.
func (s *ServerPane) Toggle() {
	if s.Busy != "" {
		return
	}
	path, action := startPath, "start"
	if s.owner.State.Running {
		path, action = stopPath, "stop"
	}
	s.run(action, func() error {
		var state shared.McpStateDto
		if err := apiCall("POST", path, nil, &state); err != nil {
			return err
		}
		s.owner.State = state
		return nil
	})
}

func (s *ServerPane) PortKey(e dom.Event) {
	if e.Key() == "Enter" {
		e.PreventDefault()
		s.ApplyPort()
	}
}

func (s *ServerPane) ApplyPort() {
	if s.PortDraft == s.owner.Settings.Port {
		return
	}
	next := s.owner.Settings
	next.Port = s.PortDraft
	s.putSettings("port", next)
}

// ToggleRequire applies at once: switching protection on is one click, and
// the backend generates a token when there is none yet.
func (s *ServerPane) ToggleRequire(e dom.Event) {
	next := s.owner.Settings
	next.RequireToken = e.Checked()
	s.putSettings("require", next)
}

func (s *ServerPane) putSettings(action string, next shared.McpSettingsDto) {
	s.run(action, func() error {
		var settings shared.McpSettingsDto
		if err := apiCall("PUT", settingsPath, next, &settings); err != nil {
			return err
		}
		s.owner.Settings = settings
		s.syncDraft()
		return nil
	})
}

// Regenerate replaces the token. Clients holding the old one are rejected from
// their next request on, hence the question when it is in use.
func (s *ServerPane) Regenerate() {
	if s.owner.Settings.RequireToken && s.owner.Settings.Token != "" &&
		!dom.Confirm("Generate a new token? MCP clients using the current one will be rejected until they are updated.") {
		return
	}
	s.run("token", func() error {
		var settings shared.McpSettingsDto
		if err := apiCall("POST", tokenPath, nil, &settings); err != nil {
			return err
		}
		s.owner.Settings = settings
		return nil
	})
}

func (s *ServerPane) ToggleShow() {
	s.ShowToken = !s.ShowToken
	s.StateHasChanged()
}

func (s *ServerPane) SetSnippet(e dom.Event) {
	s.Snippet = e.Arg()
	dom.LocalSet(snippetKey, s.Snippet)
	s.StateHasChanged()
}

func (s *ServerPane) CopyURL()     { s.copy("url", s.owner.State.URL) }
func (s *ServerPane) CopyToken()   { s.copy("token", s.owner.Settings.Token) }
func (s *ServerPane) CopySnippet() { s.copy("snippet", s.snippet(s.owner.Settings.Token)) }

func (s *ServerPane) copy(what, text string) {
	dom.Clipboard(text)
	s.Copied = what
	s.StateHasChanged()
	go func() {
		time.Sleep(1200 * time.Millisecond)
		if s.Copied == what {
			s.Copied = ""
			s.StateHasChanged()
		}
	}()
}

// run performs one backend action off the JS thread, showing it busy until it
// returns and its error, if any, afterwards.
func (s *ServerPane) run(action string, call func() error) {
	s.Busy, s.Err = action, ""
	s.StateHasChanged()
	go func() {
		if err := call(); err != nil {
			s.Err = err.Error()
		}
		s.Busy = ""
		s.StateHasChanged()
	}()
}
