package middlewares

import (
	"net"
	"strings"
	"testing"
	"time"

	"httpStackLens/http/models"
)

func composerRequest() models.ProxyRequest {
	return models.ProxyRequest{
		HttpRequestLine: models.HttpRequestLine{
			HttpMethod: models.GET,
			Endpoint:   models.ResourceEndpoint{Host: "example.com", Port: 80, PathAndQuery: "/"},
			Version:    models.Version{Major: 1, Minor: 1},
		},
	}
}

func TestWindowsAuthenticationLetsLocalConnectionsThrough(t *testing.T) {
	client, server := net.Pipe()
	defer func() { _ = client.Close() }()
	defer func() { _ = server.Close() }()

	next := &delegateRecorder{}
	middleware := &WindowsAuthenticationServerMiddleware{NextMiddleware: next}

	if err := middleware.HandleProxyRequest(LocalConn{Conn: server}, composerRequest()); err != nil {
		t.Fatalf("HandleProxyRequest: %v", err)
	}
	if !next.called {
		t.Error("a composer request must reach the rest of the pipeline unchallenged")
	}
}

func TestWindowsAuthenticationChallengesClientConnections(t *testing.T) {
	client, server := net.Pipe()
	defer func() { _ = client.Close() }()

	next := &delegateRecorder{}
	middleware := &WindowsAuthenticationServerMiddleware{NextMiddleware: next}

	done := make(chan struct{})
	go func() {
		_ = middleware.HandleProxyRequest(server, composerRequest())
		_ = server.Close()
		close(done)
	}()

	// Read the challenge only: the middleware then waits for the authenticated
	// request that a real client would send next.
	_ = client.SetReadDeadline(time.Now().Add(5 * time.Second))
	answer := make([]byte, 256)
	read, err := client.Read(answer)
	if err != nil {
		t.Fatalf("read the challenge: %v", err)
	}
	if !strings.Contains(string(answer[:read]), "407") {
		t.Errorf("expected a 407 challenge, got %q", string(answer[:read]))
	}

	_ = client.Close() // unblocks the challenge loop
	<-done
	if next.called {
		t.Error("an unauthenticated client must not reach the rest of the pipeline")
	}
}
