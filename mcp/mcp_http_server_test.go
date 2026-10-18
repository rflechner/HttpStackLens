package mcp

import (
	"context"
	"httpStackLens/configuration"
	"net"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestStartDisabled(t *testing.T) {
	// Even an invalid port is ignored when MCP is disabled.
	server, err := Start(configuration.MCPConfig{Port: -1}, Dependencies{})
	if err != nil || server != nil {
		t.Fatalf("Start disabled = (%v, %v)", server, err)
	}
}

func TestStartInvalidPort(t *testing.T) {
	for _, port := range []int{-1, 0, 65536} {
		server, err := Start(configuration.MCPConfig{Enabled: true, Port: port}, Dependencies{})
		if server != nil {
			_ = server.Close()
			t.Fatalf("unexpected listener for port %d", port)
		}
		if err == nil {
			t.Fatalf("expected error for port %d", port)
		}
	}
}

func TestStartOccupiedPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	server, err := Start(configuration.MCPConfig{Enabled: true, Port: port}, Dependencies{})
	if server != nil || err == nil {
		t.Fatalf("Start occupied port = (%v, %v)", server, err)
	}
}

func TestConfiguredListenerAndClose(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	server, err := Start(configuration.MCPConfig{Enabled: true, Port: port}, Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &sdk.StreamableClientTransport{Endpoint: "http://" + server.Addr + "/"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Tools) != 3 {
		t.Fatalf("unexpected tools: %+v", result.Tools)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	// Closing MCP must release the configured port.
	reopened, err := net.Listen("tcp", server.Addr)
	if err != nil {
		t.Fatalf("MCP listener still open: %v", err)
	}
	_ = reopened.Close()
}
