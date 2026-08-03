package main

import (
	"context"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"httpStackLens/http/models"
	"httpStackLens/proxy/middlewares"
	"httpStackLens/webui/wasm/shared"
)

// recordingEventLogger captures what the composer reports to the Web UI.
type recordingEventLogger struct {
	mu       sync.Mutex
	requests []models.ProxyRequest
}

func (l *recordingEventLogger) LogEvent(string) {}

func (l *recordingEventLogger) LogRequest(_ int, _ string, request models.ProxyRequest) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.requests = append(l.requests, request)
}

func (l *recordingEventLogger) logged() []models.ProxyRequest {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]models.ProxyRequest(nil), l.requests...)
}

// newTestSender wires a sender onto a listener-less proxy server running the
// given pipeline — the same shape as the one built in main.
func newTestSender(pipeline middlewares.Middleware, logger ProxyEventLogger) *composerSender {
	sender := newComposerSender(nil, nil)
	sender.attach(&ProxyServer{
		appContext:  AppContext{pipeline: pipeline},
		EventLogger: logger,
	})
	return sender
}

func TestComposerSenderSendsThroughThePipeline(t *testing.T) {
	var (
		mu      sync.Mutex
		method  string
		path    string
		header  string
		payload string
	)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		method, path, header, payload = r.Method, r.URL.RequestURI(), r.Header.Get("X-Token"), string(body)
		mu.Unlock()
		w.Header().Set("X-Answer", "42")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer target.Close()

	logger := &recordingEventLogger{}
	sender := newTestSender(&middlewares.TunnelServer{}, logger)

	response, err := sender.Send(context.Background(), shared.ComposerRequestDto{
		Method:  "POST",
		Url:     target.URL + "/items?page=2",
		Headers: []shared.HeaderDto{{Name: "X-Token", Value: "secret"}},
		Body:    `{"name":"composer"}`,
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if response.Error != "" {
		t.Fatalf("Expected a completed exchange, got error %q", response.Error)
	}
	if response.Status != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", response.Status)
	}
	if response.StatusText != "Created" {
		t.Errorf("Expected status text 'Created', got %q", response.StatusText)
	}
	if response.Body != `{"ok":true}` {
		t.Errorf("Unexpected body %q", response.Body)
	}
	if !hasHeader(response.Headers, "X-Answer", "42") {
		t.Errorf("Expected the X-Answer response header, got %v", response.Headers)
	}

	mu.Lock()
	defer mu.Unlock()
	if method != "POST" {
		t.Errorf("Expected the target to receive a POST, got %s", method)
	}
	if path != "/items?page=2" {
		t.Errorf("Expected the path and query to be forwarded, got %q", path)
	}
	if header != "secret" {
		t.Errorf("Expected the X-Token header to be forwarded, got %q", header)
	}
	if payload != `{"name":"composer"}` {
		t.Errorf("Expected the body to be forwarded, got %q", payload)
	}

	// The exchange must surface in the traffic view like any proxied request.
	logged := logger.logged()
	if len(logged) != 1 {
		t.Fatalf("Expected 1 request reported to the UI, got %d", len(logged))
	}
	if logged[0].HttpRequestLine.HttpMethod != models.POST {
		t.Errorf("Expected a POST to be reported, got %q", logged[0].HttpRequestLine.HttpMethod)
	}
}

func TestComposerSenderSendsThroughHttps(t *testing.T) {
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "secure")
	}))
	defer target.Close()

	sender := newTestSender(&middlewares.TunnelServer{}, &recordingEventLogger{})
	// The composer trusts the system roots plus, when decrypting, this
	// application's CA. Here it must trust the test server's own certificate.
	pool := x509.NewCertPool()
	pool.AddCert(target.Certificate())
	sender.trustedRoots = func() *x509.CertPool { return pool }

	response, err := sender.Send(context.Background(), shared.ComposerRequestDto{
		Method: "GET",
		Url:    target.URL + "/",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if response.Error != "" {
		t.Fatalf("Expected a completed exchange, got error %q", response.Error)
	}
	if response.Status != http.StatusOK || response.Body != "secure" {
		t.Errorf("Unexpected response %d %q", response.Status, response.Body)
	}
}

func TestComposerSenderReportsTransportFailureAsAResult(t *testing.T) {
	// Nothing listens on this port: the pipeline answers 502, the tunnel dial
	// fails, and the composer shows the failure instead of erroring out.
	sender := newTestSender(&middlewares.TunnelServer{}, &recordingEventLogger{})

	response, err := sender.Send(context.Background(), shared.ComposerRequestDto{
		Method: "GET",
		Url:    "http://127.0.0.1:1/unreachable",
	})
	if err != nil {
		t.Fatalf("Expected a result, got an API error: %v", err)
	}
	if response.Status != http.StatusBadGateway && response.Error == "" {
		t.Errorf("Expected a failure to be reported, got %+v", response)
	}
}

func TestComposerSenderRejectsUnusableRequests(t *testing.T) {
	sender := newTestSender(&middlewares.TunnelServer{}, &recordingEventLogger{})

	cases := []struct {
		name    string
		request shared.ComposerRequestDto
	}{
		{"no scheme", shared.ComposerRequestDto{Method: "GET", Url: "api.ipify.org"}},
		{"unsupported scheme", shared.ComposerRequestDto{Method: "GET", Url: "ftp://example.com/a"}},
		{"no host", shared.ComposerRequestDto{Method: "GET", Url: "http:///a"}},
		{"invalid method", shared.ComposerRequestDto{Method: "G E T", Url: "http://example.com/a"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := sender.Send(context.Background(), c.request); err == nil {
				t.Errorf("Expected an error for %+v, got success", c.request)
			}
		})
	}
}

func TestComposerSenderIsUnavailableBeforeAttach(t *testing.T) {
	sender := newComposerSender(nil, nil)
	if _, err := sender.Send(context.Background(), shared.ComposerRequestDto{Url: "http://example.com"}); err == nil {
		t.Fatal("Expected an error while the pipeline is not attached")
	}
}

func TestComposerSenderTruncatesLargeBodies(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("x", 32))
	}))
	defer target.Close()

	sender := newTestSender(&middlewares.TunnelServer{}, &recordingEventLogger{})
	sender.maxBody = 8

	response, err := sender.Send(context.Background(), shared.ComposerRequestDto{Method: "GET", Url: target.URL})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !response.Truncated {
		t.Error("Expected the body to be reported as truncated")
	}
	if len(response.Body) != 8 {
		t.Errorf("Expected 8 bytes of body, got %d", len(response.Body))
	}
}

func hasHeader(headers []shared.HeaderDto, name, value string) bool {
	for _, header := range headers {
		if strings.EqualFold(header.Name, name) && header.Value == value {
			return true
		}
	}
	return false
}
