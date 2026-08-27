package middlewares

import (
	"net"
	"testing"

	"httpStackLens/http"
	"httpStackLens/http/models"
)

// delegateRecorder stands in for the rest of the pipeline.
type delegateRecorder struct {
	called bool
}

func (m *delegateRecorder) HandleProxyRequest(net.Conn, models.ProxyRequest) error {
	m.called = true
	return nil
}

func TestIsLocalSeesThroughTheBufferedStream(t *testing.T) {
	client, server := net.Pipe()
	defer func() { _ = client.Close() }()
	defer func() { _ = server.Close() }()

	if IsLocal(server) {
		t.Error("a plain connection must not be reported as local")
	}
	// The pipeline receives the buffered stream, not the connection itself, so
	// the marker has to survive the wrapping.
	if !IsLocal(http.NewNetworkStream(LocalConn{Conn: server})) {
		t.Error("a local connection must stay local through the network stream")
	}
	if IsLocal(http.NewNetworkStream(server)) {
		t.Error("a plain connection must not become local through the network stream")
	}
}
