package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"httpStackLens/composer"
	"httpStackLens/configuration"
	"httpStackLens/mcp"
	"httpStackLens/proxy/middlewares"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// The MCP composer tools reach the destination through the real proxy
// pipeline, with the file variables substituted — the path the Web UI play
// button takes.
func TestMCPComposerRequestGoesThroughThePipeline(t *testing.T) {
	var (
		mu       sync.Mutex
		received string
		token    string
	)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		received, token = r.Method+" "+r.URL.RequestURI()+" "+string(body), r.Header.Get("X-Token")
		mu.Unlock()
		w.Header().Set("Set-Cookie", "sid=server-secret")
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"queued":true}`)
	}))
	defer target.Close()

	files := composer.NewHttpFileStore(t.TempDir())
	if _, err := files.Save("api.http", "@base = "+target.URL+"\n@token = s3cr3t\n"); err != nil {
		t.Fatal(err)
	}
	sender := newTestSender(&middlewares.TunnelServer{}, &recordingEventLogger{})

	probe, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	_ = probe.Close()
	server, err := mcp.Start(configuration.MCPConfig{Enabled: true, Port: port}, mcp.Dependencies{HttpFiles: files, SendComposer: sender.Send})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = server.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := sdk.NewClient(&sdk.Implementation{Name: "integration", Version: "1"}, nil)
	session, err := client.Connect(ctx, &sdk.StreamableClientTransport{Endpoint: "http://" + server.Addr + "/"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	call := func(name string, args map[string]any) map[string]any {
		t.Helper()
		result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		all, _ := json.Marshal(result)
		if result.IsError {
			t.Fatalf("%s failed: %s", name, all)
		}
		var out map[string]any
		data, _ := json.Marshal(result.StructuredContent)
		_ = json.Unmarshal(data, &out)
		return out
	}

	call("add_composer_request", map[string]any{
		"file": "api", "title": "Enqueue", "method": "POST", "url": "{{base}}/jobs?x=1",
		"headers": []map[string]any{{"name": "X-Token", "value": "{{token}}"}},
		"body":    `{"job":"build"}`,
	})
	sent := call("send_composer_request", map[string]any{"file": "api", "index": 0})

	mu.Lock()
	defer mu.Unlock()
	if received != `POST /jobs?x=1 {"job":"build"}` || token != "s3cr3t" {
		t.Fatalf("the destination received %q with token %q", received, token)
	}
	if sent["outcome"] != "response" || sent["status"] != float64(http.StatusAccepted) || sent["body"] != `{"queued":true}` {
		t.Fatalf("unexpected result: %+v", sent)
	}
	data, _ := json.Marshal(sent)
	if strings.Contains(string(data), "server-secret") || strings.Contains(string(data), "s3cr3t") {
		t.Fatalf("the result leaked a secret: %s", data)
	}
}
