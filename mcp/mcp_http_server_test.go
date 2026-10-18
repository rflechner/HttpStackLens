package mcp

import (
	"context"
	"errors"
	"httpStackLens/configuration"
	"httpStackLens/storage"
	"httpStackLens/webui/wasm/shared"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// freePort returns a port nothing listens on, as far as one can tell.
func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	return port
}

func connect(t *testing.T, endpoint string, client *http.Client) (context.Context, *sdk.ClientSession, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	sdkClient := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil)
	session, err := sdkClient.Connect(ctx, &sdk.StreamableClientTransport{Endpoint: endpoint, HTTPClient: client}, nil)
	if err == nil {
		t.Cleanup(func() { _ = session.Close() })
	}
	return ctx, session, err
}

func TestApplyDisabled(t *testing.T) {
	// Even an invalid port is ignored when MCP is disabled.
	c := NewController(configuration.MCPConfig{Port: -1})
	if err := c.Apply(configuration.MCPConfig{Port: -1}); err != nil {
		t.Fatal(err)
	}
	if state := c.State(); state.Running || state.Enabled {
		t.Fatalf("state = %+v", state)
	}
}

func TestApplyInvalidPort(t *testing.T) {
	for _, port := range []int{-1, 0, 65536} {
		c := NewController(configuration.MCPConfig{})
		err := c.Apply(configuration.MCPConfig{Enabled: true, Port: port})
		if err == nil {
			c.Close()
			t.Fatalf("expected error for port %d", port)
		}
		if state := c.State(); state.Running || state.Enabled || state.LastError == "" {
			t.Fatalf("state after invalid port %d = %+v", port, state)
		}
	}
}

func TestApplyRequiresTokenValue(t *testing.T) {
	c := NewController(configuration.MCPConfig{})
	if err := c.Apply(configuration.MCPConfig{Enabled: true, Port: freePort(t), RequireToken: true}); err == nil {
		c.Close()
		t.Fatal("expected an error without a token")
	}
}

func TestStartupFailureKeepsEnabledAndReportsError(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	config := configuration.MCPConfig{Enabled: true, Port: listener.Addr().(*net.TCPAddr).Port}

	c := NewController(config)
	if err := c.Apply(config); err == nil {
		c.Close()
		t.Fatal("expected an error on an occupied port")
	}
	state := c.State()
	if !state.Enabled || state.Running || !strings.Contains(state.LastError, "start MCP listener") {
		t.Fatalf("state = %+v", state)
	}

	// Freeing the port and retrying clears the error.
	_ = listener.Close()
	if err := c.Apply(config); err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if state := c.State(); !state.Running || state.LastError != "" || state.StartedAt == "" {
		t.Fatalf("state after retry = %+v", state)
	}
}

func TestStartStopAndListTools(t *testing.T) {
	var states []shared.McpStateDto
	c := NewController(configuration.MCPConfig{})
	c.Attach(Dependencies{StateChanged: func(s shared.McpStateDto) { states = append(states, s) }})
	config := configuration.MCPConfig{Enabled: true, Port: freePort(t)}
	if err := c.Apply(config); err != nil {
		t.Fatal(err)
	}
	addr := c.Addr()
	ctx, session, err := connect(t, "http://"+addr+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Tools) != 8 {
		t.Fatalf("unexpected tools: %+v", result.Tools)
	}
	_ = session.Close()

	config.Enabled = false
	if err := c.Apply(config); err != nil {
		t.Fatal(err)
	}
	if c.Addr() != "" || c.State().Running {
		t.Fatalf("listener still running: %+v", c.State())
	}
	// Stopping MCP must release the port.
	reopened, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("MCP listener still open: %v", err)
	}
	_ = reopened.Close()

	if len(states) != 2 || !states[0].Running || states[1].Running {
		t.Fatalf("published states = %+v", states)
	}
}

func TestApplyMovesListenerToNewPort(t *testing.T) {
	c := NewController(configuration.MCPConfig{})
	first := configuration.MCPConfig{Enabled: true, Port: freePort(t)}
	if err := c.Apply(first); err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// A taken port leaves the listener where it was.
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	if err := c.Apply(configuration.MCPConfig{Enabled: true, Port: taken.Addr().(*net.TCPAddr).Port}); err == nil {
		t.Fatal("expected an error on an occupied port")
	}
	if state := c.State(); !state.Running || state.Port != first.Port || state.LastError == "" {
		t.Fatalf("state after failed move = %+v", state)
	}

	second := configuration.MCPConfig{Enabled: true, Port: freePort(t)}
	if err := c.Apply(second); err != nil {
		t.Fatal(err)
	}
	if state := c.State(); !state.Running || state.Port != second.Port || !strings.HasSuffix(state.URL, ":"+strconv.Itoa(second.Port)+"/") {
		t.Fatalf("state after move = %+v", state)
	}
}

// bearerTransport adds a token to every request, as an MCP client configured
// with an Authorization header does.
type bearerTransport struct{ token string }

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

func TestRequireToken(t *testing.T) {
	token, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	c := NewController(configuration.MCPConfig{})
	config := configuration.MCPConfig{Enabled: true, Port: freePort(t), RequireToken: true, Token: token}
	if err := c.Apply(config); err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	endpoint := "http://" + c.Addr() + "/"

	if _, _, err := connect(t, endpoint, nil); err == nil {
		t.Fatal("connected without a token")
	}
	if _, _, err := connect(t, endpoint, &http.Client{Transport: bearerTransport{"wrong"}}); err == nil {
		t.Fatal("connected with a wrong token")
	}
	ctx, session, err := connect(t, endpoint, &http.Client{Transport: bearerTransport{token}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.ListTools(ctx, nil); err != nil {
		t.Fatal(err)
	}

	calls := c.TracedCalls()
	var unauthorized []string
	for _, call := range calls {
		if call.Status == CallUnauthorized {
			unauthorized = append(unauthorized, call.Error)
		}
	}
	if len(unauthorized) < 2 || !slices.Contains(unauthorized, "missing bearer token") || !slices.Contains(unauthorized, "invalid bearer token") {
		t.Fatalf("unauthorized calls = %v", unauthorized)
	}

	// Switching the requirement off applies to the running listener.
	config.RequireToken = false
	if err := c.Apply(config); err != nil {
		t.Fatal(err)
	}
	if _, _, err := connect(t, endpoint, nil); err != nil {
		t.Fatalf("token still required: %v", err)
	}
}

func TestBearerToken(t *testing.T) {
	for header, want := range map[string]string{
		"Bearer abc":   "abc",
		"bearer  abc ": "abc",
		"Basic abc":    "",
		"Bearer":       "",
		"":             "",
	} {
		got, ok := bearerToken(header)
		if got != want || ok != (want != "") {
			t.Errorf("bearerToken(%q) = (%q, %v)", header, got, ok)
		}
	}
}

func TestCallLogTracesToolCalls(t *testing.T) {
	var published []shared.McpCallDto
	c := NewController(configuration.MCPConfig{})
	c.Attach(Dependencies{
		Requests: storage.NewRequestStore(storage.DefaultRequestStoreSize),
		Called:   func(call shared.McpCallDto) { published = append(published, call) },
	})
	if err := c.Apply(configuration.MCPConfig{Enabled: true, Port: freePort(t)}); err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, session, err := connect(t, "http://"+c.Addr()+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "search_requests", Arguments: map[string]any{"method": "GET"}}); err != nil {
		t.Fatal(err)
	}
	// Status needs live stores: without them the tool reports an error.
	if _, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "get_status"}); err != nil {
		t.Fatal(err)
	}

	// The SDK client may open with a discovery request: find the calls by what
	// they are rather than by position.
	calls := c.TracedCalls()
	if len(published) != len(calls) {
		t.Fatalf("published %d calls, retained %d", len(published), len(calls))
	}
	find := func(method, tool string) shared.McpCallDto {
		t.Helper()
		for _, call := range calls {
			if call.Method == method && call.Tool == tool {
				return call
			}
		}
		t.Fatalf("no %s %s call in %+v", method, tool, calls)
		return shared.McpCallDto{}
	}
	init := find("initialize", "")
	search := find("tools/call", "search_requests")
	failed := find("tools/call", "get_status")
	if init.Client != "test 1" {
		t.Errorf("initialize call = %+v", init)
	}
	if search.Status != CallOK || search.Arguments != `{"method":"GET"}` || !strings.Contains(search.Result, `"requests":[]`) || search.Client != "test 1" || search.SessionID == "" {
		t.Errorf("search_requests call = %+v", search)
	}
	if failed.Status != CallToolError || !strings.Contains(failed.Error, "unavailable") {
		t.Errorf("get_status call = %+v", failed)
	}
	if calls[0] != failed || !(failed.ID > search.ID && search.ID > init.ID) {
		t.Errorf("calls not newest first: %+v", calls)
	}

	c.ClearCalls()
	if len(c.TracedCalls()) != 0 || c.State().CallCount != 0 {
		t.Fatal("trace not cleared")
	}
}

func TestCallLogCapacityAndTruncation(t *testing.T) {
	log := newCallLog()
	for range CallLogCapacity + 5 {
		log.add(shared.McpCallDto{Method: "tools/call"})
	}
	calls := log.List()
	if len(calls) != CallLogCapacity || calls[0].ID != CallLogCapacity+5 || calls[len(calls)-1].ID != 6 {
		t.Fatalf("kept %d calls, newest %d, oldest %d", len(calls), calls[0].ID, calls[len(calls)-1].ID)
	}

	long := strings.Repeat("é", maxTracedBytes)
	text, cut := traceRaw([]byte(long))
	if !cut || len(text) > maxTracedBytes || !strings.HasPrefix(long, text) || strings.ContainsRune(text, '�') {
		t.Fatalf("truncated to %d bytes, cut=%v", len(text), cut)
	}
}

func TestNewTokenIsRandom(t *testing.T) {
	a, errA := NewToken()
	b, errB := NewToken()
	if err := errors.Join(errA, errB); err != nil {
		t.Fatal(err)
	}
	if len(a) != 64 || a == b {
		t.Fatalf("tokens %q and %q", a, b)
	}
}

func TestToolsCatalog(t *testing.T) {
	// The catalog does not need the listener, nor does it appear in the trace.
	c := NewController(configuration.MCPConfig{})
	tools, err := c.Tools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 8 || len(c.TracedCalls()) != 0 {
		t.Fatalf("%d tools, %d traced calls", len(tools), len(c.TracedCalls()))
	}
	byName := map[string]shared.McpToolDto{}
	for i, tool := range tools {
		if tool.Description == "" || len(tool.InputSchema) == 0 {
			t.Errorf("tool %s lacks a description or input schema", tool.Name)
		}
		if i > 0 && tools[i-1].Name >= tool.Name {
			t.Errorf("tools not sorted: %s before %s", tools[i-1].Name, tool.Name)
		}
		byName[tool.Name] = tool
	}

	search := byName["search_requests"]
	if !search.ReadOnly || search.Destructive || search.OpenWorld || !strings.Contains(string(search.InputSchema), `"path_contains"`) {
		t.Errorf("search_requests = %+v", search)
	}
	add := byName["add_composer_request"]
	if add.ReadOnly || add.Destructive || add.OpenWorld {
		t.Errorf("add_composer_request = %+v", add)
	}
	// No annotation means the protocol defaults: may modify, reaches out.
	send := byName["send_composer_request"]
	if send.ReadOnly || !send.Destructive || !send.OpenWorld {
		t.Errorf("send_composer_request = %+v", send)
	}
}
