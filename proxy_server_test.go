package main

import (
	"httpStackLens/configuration"
	"httpStackLens/storage"
	"net"
	"testing"
)

func TestProxyServerAllowsClientUsesAccessControl(t *testing.T) {
	server := ProxyServer{
		accessControl: configuration.NewAccessControlSettingsStore(configuration.AccessControlSettings{
			Proxy: configuration.AccessControlConfig{Mode: configuration.AccessControlLan},
		}),
	}

	if !server.allowsClient(&net.TCPAddr{IP: net.ParseIP("192.168.1.42"), Port: 50000}) {
		t.Fatal("LAN access should allow private proxy client")
	}
	if server.allowsClient(&net.TCPAddr{IP: net.ParseIP("8.8.8.8"), Port: 50000}) {
		t.Fatal("LAN access should reject public proxy client")
	}
}

func TestProxyServerStopAcceptingKeepsTrackedConnectionOpen(t *testing.T) {
	server, err := CreateProxyServer(
		AppContext{port: 0}, nil, configuration.ProxyConfig{}, nil, nil, nil,
		storage.NewCaptureController(true),
	)
	if err != nil {
		t.Fatalf("CreateProxyServer: %v", err)
	}
	defer server.Close()

	client, peer := net.Pipe()
	defer client.Close()
	defer peer.Close()
	if !server.trackConnection(client) {
		t.Fatal("could not track active connection")
	}

	server.StopAccepting()
	writeErr := make(chan error, 1)
	go func() {
		_, err := peer.Write([]byte("x"))
		writeErr <- err
	}()
	buf := make([]byte, 1)
	if _, err := client.Read(buf); err != nil {
		t.Fatalf("read active connection after StopAccepting: %v", err)
	}
	if err := <-writeErr; err != nil {
		t.Fatalf("active connection was closed by StopAccepting: %v", err)
	}
}
