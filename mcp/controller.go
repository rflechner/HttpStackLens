package mcp

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"httpStackLens/configuration"
	"httpStackLens/webui/wasm/shared"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Controller owns the MCP listener for the life of the application: the Web UI
// starts, stops and reconfigures it at runtime, and every message it receives
// is traced in its CallLog.
//
// It never touches config.yaml. Persisting a change is the caller's business —
// the runtime supervisor, which serialises every write to that file.
type Controller struct {
	calls *CallLog

	mu        sync.Mutex
	deps      Dependencies
	config    configuration.MCPConfig
	server    *http.Server
	url       string
	startedAt time.Time
	lastErr   string
}

// NewController returns a stopped controller holding config. Nothing listens
// until Apply is called.
func NewController(config configuration.MCPConfig) *Controller {
	return &Controller{calls: newCallLog(), config: config}
}

// Attach hands the controller the stores its tools read, and the callbacks
// reporting to the Web UI. It is separate from NewController because those
// callbacks publish on the Web UI event hub, which exists only once the Web UI
// is served — and the Web UI needs the controller to be served. Call it before
// the first Apply.
func (c *Controller) Attach(deps Dependencies) {
	c.mu.Lock()
	c.deps = deps
	c.mu.Unlock()
	c.calls.setListener(deps.Called)
}

// TracedCalls returns the messages received by the listener, newest first.
func (c *Controller) TracedCalls() []shared.McpCallDto { return c.calls.List() }

// ClearCalls empties the trace and tells the Web UI, whose other windows then
// drop their copy.
func (c *Controller) ClearCalls() {
	c.calls.Clear()
	c.notifyState()
}

// Config returns the configuration in force.
func (c *Controller) Config() configuration.MCPConfig {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.config
}

// Apply makes the listener match config: it is started, stopped or moved to
// another port as needed, and a token change applies from the next request on.
// When the listener cannot start, the previous configuration stays in force and
// the error is kept for State until the next successful Apply.
func (c *Controller) Apply(config configuration.MCPConfig) error {
	c.mu.Lock()
	err := c.apply(config)
	if err != nil {
		c.lastErr = err.Error()
	} else {
		c.lastErr = ""
	}
	state := c.stateLocked()
	onState := c.deps.StateChanged
	c.mu.Unlock()

	if onState != nil {
		onState(state)
	}
	return err
}

func (c *Controller) apply(next configuration.MCPConfig) error {
	if err := ValidateConfig(next); err != nil {
		return err
	}
	prev := c.config
	moved := c.server != nil && (!next.Enabled || next.Port != prev.Port)
	if moved {
		c.closeLocked()
	}
	c.config = next
	if !next.Enabled || c.server != nil {
		return nil
	}
	if err := c.startLocked(); err != nil {
		// Back to what was running: a port taken by another program must not
		// leave clients without the listener they were using.
		c.config = prev
		if moved {
			if restartErr := c.startLocked(); restartErr != nil {
				slog.Warn("Could not restart the MCP server on its previous port", "port", prev.Port, "error", restartErr)
			}
		}
		return err
	}
	return nil
}

// ValidateConfig checks what Apply needs to start a listener. The port is only
// checked when the listener is to run, so that a disabled section with a
// placeholder port can still be loaded.
func ValidateConfig(config configuration.MCPConfig) error {
	if config.Enabled && (config.Port < 1 || config.Port > 65535) {
		return fmt.Errorf("MCP port must be between 1 and 65535, got %d", config.Port)
	}
	if config.RequireToken && config.Token == "" {
		return errors.New("an MCP token is required when require_token is set")
	}
	return nil
}

func (c *Controller) startLocked() error {
	addr := net.JoinHostPort("localhost", strconv.Itoa(c.config.Port))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("start MCP listener: %w", err)
	}
	server := &http.Server{Addr: listener.Addr().String(), Handler: c.requireToken(newHandler(c.deps, c.calls))}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("MCP server stopped", "error", err)
		}
	}()
	c.server = server
	c.url = "http://" + server.Addr + "/"
	c.startedAt = time.Now()
	slog.Info("MCP server listening", "url", c.url, "requireToken", c.config.RequireToken)
	return nil
}

// closeLocked stops the listener and drops the open MCP sessions with it.
func (c *Controller) closeLocked() {
	if c.server == nil {
		return
	}
	if err := c.server.Close(); err != nil {
		slog.Warn("Error closing the MCP server", "error", err)
	}
	slog.Info("MCP server stopped", "url", c.url)
	c.server = nil
	c.startedAt = time.Time{}
}

// Close stops the listener when the application exits. The configuration is
// left as it is, so that config.yaml is not rewritten on the way out.
func (c *Controller) Close() {
	c.mu.Lock()
	c.closeLocked()
	c.mu.Unlock()
}

// Addr is the address the listener is bound to, or "" while it is stopped.
func (c *Controller) Addr() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.server == nil {
		return ""
	}
	return c.server.Addr
}

// State is the listener as the Web UI shows it, without the token.
func (c *Controller) State() shared.McpStateDto {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stateLocked()
}

func (c *Controller) stateLocked() shared.McpStateDto {
	state := shared.McpStateDto{
		Enabled:         c.config.Enabled,
		Running:         c.server != nil,
		Port:            c.config.Port,
		URL:             fmt.Sprintf("http://127.0.0.1:%d/", c.config.Port),
		RequireToken:    c.config.RequireToken,
		TokenConfigured: c.config.Token != "",
		LastError:       c.lastErr,
		CallCount:       c.calls.Len(),
	}
	if c.server != nil {
		state.URL = c.url
		state.StartedAt = c.startedAt.UTC().Format(time.RFC3339)
	}
	return state
}

// notifyState publishes the state outside of Apply.
func (c *Controller) notifyState() {
	c.mu.Lock()
	state := c.stateLocked()
	onState := c.deps.StateChanged
	c.mu.Unlock()
	if onState != nil {
		onState(state)
	}
}

// requireToken rejects requests without the configured bearer token, when one
// is required. It reads the configuration on every request, so that a new
// token takes effect without restarting the listener or dropping sessions.
func (c *Controller) requireToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		required, token := c.config.RequireToken, c.config.Token
		c.mu.Unlock()

		if required {
			reason := ""
			got, ok := bearerToken(r.Header.Get("Authorization"))
			switch {
			case !ok:
				reason = "missing bearer token"
			case subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1:
				reason = "invalid bearer token"
			}
			if reason != "" {
				c.calls.add(shared.McpCallDto{
					At:     time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00"),
					Method: "http",
					Status: CallUnauthorized,
					Error:  reason,
					Client: r.UserAgent(),
					Remote: r.RemoteAddr,
				})
				w.Header().Set("WWW-Authenticate", `Bearer realm="HttpStackLens MCP"`)
				http.Error(w, "unauthorized: "+reason, http.StatusUnauthorized)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// bearerToken extracts the credentials of an "Authorization: Bearer …" header.
// The scheme is case-insensitive (RFC 7235).
func bearerToken(header string) (string, bool) {
	scheme, token, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	return token, token != ""
}

// NewToken returns a random 256-bit token, hex encoded so that it can be pasted
// into any client configuration without quoting.
func NewToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate MCP token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
