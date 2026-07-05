package middlewares

import (
	"httpStackLens/storage"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"httpStackLens/webui/wasm/shared"
)

// fakeSink records the events published to it so tests can assert on them.
type fakeSink struct {
	requests  []shared.RequestEventDto
	responses []shared.ResponseEventDto
}

func (f *fakeSink) PublishRequestEvent(e shared.RequestEventDto) { f.requests = append(f.requests, e) }
func (f *fakeSink) PublishResponseEvent(e shared.ResponseEventDto) {
	f.responses = append(f.responses, e)
}

func TestPublishRequestEvent(t *testing.T) {
	sink := &fakeSink{}
	m := &HttpsInterceptor{Events: sink}

	req := httptest.NewRequest(http.MethodGet, "https://api.github.com/user?state=open", nil)
	m.publishRequestEvent("corr-1", req, "api.github.com", "api.github.com:443")

	if len(sink.requests) != 1 {
		t.Fatalf("expected 1 request event, got %d", len(sink.requests))
	}
	got := sink.requests[0]
	if got.CorrelationID != "corr-1" {
		t.Errorf("CorrelationID = %q, want %q", got.CorrelationID, "corr-1")
	}
	if got.Method != "GET" || got.Host != "api.github.com" || got.Port != 443 {
		t.Errorf("method/host/port = %q/%q/%d", got.Method, got.Host, got.Port)
	}
	if got.Path != "/user?state=open" {
		t.Errorf("Path = %q, want %q", got.Path, "/user?state=open")
	}
	if got.Scheme != "https" || !got.Tls || !got.Decrypted {
		t.Errorf("scheme/tls/decrypted = %q/%v/%v, want https/true/true", got.Scheme, got.Tls, got.Decrypted)
	}
	if got.ID != 1 {
		t.Errorf("ID = %d, want 1 (first decrypted request)", got.ID)
	}

	// The display sequence increments across requests.
	m.publishRequestEvent("corr-2", req, "api.github.com", "api.github.com:443")
	if id := sink.requests[1].ID; id != 2 {
		t.Errorf("second event ID = %d, want 2", id)
	}
}

func TestPublishRequestEventDefaultPort(t *testing.T) {
	sink := &fakeSink{}
	m := &HttpsInterceptor{Events: sink}
	req := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)

	// authority without an explicit port falls back to 443.
	m.publishRequestEvent("c", req, "example.com", "example.com")
	if got := sink.requests[0].Port; got != 443 {
		t.Errorf("Port = %d, want 443", got)
	}
}

func TestPublishResponseEvent(t *testing.T) {
	sink := &fakeSink{}
	m := &HttpsInterceptor{Events: sink}
	resp := &http.Response{StatusCode: 200, Status: "200 OK"}

	m.publishResponseEvent("corr-1", resp, "application/json", 1234, false, 512, 42*time.Millisecond)

	if len(sink.responses) != 1 {
		t.Fatalf("expected 1 response event, got %d", len(sink.responses))
	}
	got := sink.responses[0]
	if got.CorrelationID != "corr-1" || got.Status != 200 || got.StatusText != "OK" {
		t.Errorf("corr/status/text = %q/%d/%q", got.CorrelationID, got.Status, got.StatusText)
	}
	if got.ContentType != "application/json" || got.Size != 1234 || got.DurationMs != 42 {
		t.Errorf("ct/size/dur = %q/%d/%d", got.ContentType, got.Size, got.DurationMs)
	}
	if !got.BodyAvailable || got.BodySkipped || got.Stream {
		t.Errorf("bodyAvailable/skipped/stream = %v/%v/%v, want true/false/false", got.BodyAvailable, got.BodySkipped, got.Stream)
	}
}

func TestPublishResponseEventSkippedBodyNotAvailable(t *testing.T) {
	sink := &fakeSink{}
	m := &HttpsInterceptor{Events: sink}
	resp := &http.Response{StatusCode: 200, Status: "200 OK"}

	// A skipped body (too large) is not available for the detail view.
	m.publishResponseEvent("c", resp, "application/octet-stream", 9_000_000, true, 0, time.Millisecond)
	got := sink.responses[0]
	if got.BodyAvailable || !got.BodySkipped {
		t.Errorf("bodyAvailable/skipped = %v/%v, want false/true", got.BodyAvailable, got.BodySkipped)
	}
	if got.Size != 9_000_000 {
		t.Errorf("Size = %d, want the full transferred size even when skipped", got.Size)
	}
}

func TestPublishEventsNilSinkIsNoop(t *testing.T) {
	m := &HttpsInterceptor{} // no Events
	req := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
	resp := &http.Response{StatusCode: 200, Status: "200 OK"}
	// Must not panic.
	m.publishRequestEvent("c", req, "example.com", "example.com:443")
	m.publishResponseEvent("c", resp, "text/html", 0, false, 0, 0)
}

func TestPublishEventsPausedCaptureIsNoop(t *testing.T) {
	sink := &fakeSink{}
	captureCtl := storage.NewCaptureController(false)
	m := &HttpsInterceptor{Events: sink, CaptureCtl: captureCtl}
	req := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
	resp := &http.Response{StatusCode: 200, Status: "200 OK"}

	m.publishRequestEvent("c", req, "example.com", "example.com:443")
	m.publishResponseEvent("c", resp, "text/html", 0, false, 0, 0)

	if len(sink.requests) != 0 || len(sink.responses) != 0 {
		t.Fatalf("paused capture published events: requests=%d responses=%d", len(sink.requests), len(sink.responses))
	}
}

func TestIsStreaming(t *testing.T) {
	cases := []struct {
		name        string
		contentType string
		status      int
		want        bool
	}{
		{"sse", "text/event-stream", 200, true},
		{"sse with charset", "text/event-stream; charset=utf-8", 200, true},
		{"websocket upgrade", "", http.StatusSwitchingProtocols, true},
		{"plain json", "application/json", 200, false},
		{"html", "text/html", 200, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := &http.Response{StatusCode: tc.status}
			if got := isStreaming(tc.contentType, resp); got != tc.want {
				t.Errorf("isStreaming(%q, %d) = %v, want %v", tc.contentType, tc.status, got, tc.want)
			}
		})
	}
}

func TestCaptureLimitWriterTotalCountsSkipped(t *testing.T) {
	w := &captureLimitWriter{limit: 4}
	_, _ = w.Write([]byte("hello world")) // 11 bytes, exceeds the 4-byte limit
	if _, skipped := w.captured(); !skipped {
		t.Fatal("expected body to be skipped")
	}
	if w.total() != 11 {
		t.Errorf("total() = %d, want 11 (full size even when skipped)", w.total())
	}
}
