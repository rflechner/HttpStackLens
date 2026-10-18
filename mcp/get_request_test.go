package mcp

import (
	"context"
	"encoding/json"
	"httpStackLens/storage"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGetRequestOverMCP(t *testing.T) {
	store := storage.NewRequestStore(10)
	store.PutRequest("a", storage.RequestRecord{
		Method: "POST", URL: "https://user:password@example.com/login?token=secret#private", HttpVersion: storage.HttpVersion11,
		Headers: []storage.Header{
			{Name: "Host", Value: "example.com"},
			{Name: "Authorization", Value: "Bearer hidden-bearer"},
			{Name: "Cookie", Value: "sid=cookie-secret; theme=dark;"},
			{Name: "X-Api-Key", Value: "key-secret"},
			{Name: "Referer", Value: "https://example.com/start?code=referer-secret"},
			{Name: "Content-Type", Value: "application/json"},
		},
		Body: []byte(`{"password":"body-secret"}`),
	})
	store.PutResponse("a", storage.ResponseRecord{
		StatusCode: 302, StatusMessage: "Found", HttpVersion: storage.HttpVersion11,
		Headers: []storage.Header{
			{Name: "Set-Cookie", Value: "sid=set-cookie-secret; Path=/; HttpOnly"},
			{Name: "Location", Value: "/home?ticket=location-secret"},
			{Name: "WWW-Authenticate", Value: "Negotiate"},
		},
		BodySkipped: true,
	})
	store.PutTiming("a", storage.Timing{Connect: 20 * time.Millisecond, Ttfb: 80 * time.Millisecond, Total: 120 * time.Millisecond})

	ctx, session := connectStatusClient(t, Dependencies{Requests: store})
	result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "get_request", Arguments: map[string]any{"id": "a"}})
	if err != nil || result.IsError {
		t.Fatalf("call: %v %+v", err, result)
	}
	all, _ := json.Marshal(result)
	for _, secret := range []string{"password", "secret", "private", "hidden"} {
		if strings.Contains(string(all), secret) {
			t.Fatalf("leaked %q: %s", secret, all)
		}
	}

	data, _ := json.Marshal(result.StructuredContent)
	var got RequestDetail
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != "a" || got.State != "complete" || got.Visibility != "https_request" {
		t.Fatalf("detail = %+v", got)
	}
	req := got.Request
	if req.Method != "POST" || req.URL != "https://example.com/login?REDACTED" || !req.URLRedacted || req.HTTPVersion != "HTTP/1.1" {
		t.Fatalf("request = %+v", req)
	}
	if req.Body.State != "captured" || req.Body.Size != 26 || req.Body.ContentType != "application/json" {
		t.Fatalf("request body = %+v", req.Body)
	}
	headers := map[string]HeaderSummary{}
	for _, h := range append(req.Headers, got.Response.Headers...) {
		headers[h.Name] = h
	}
	for name, want := range map[string]string{
		"Host":             "example.com",
		"Authorization":    "Bearer REDACTED",
		"Cookie":           "sid=REDACTED; theme=REDACTED",
		"X-Api-Key":        "REDACTED",
		"Referer":          "https://example.com/start?REDACTED",
		"Set-Cookie":       "sid=REDACTED; Path=/; HttpOnly",
		"Location":         "/home?REDACTED",
		"WWW-Authenticate": "Negotiate",
	} {
		if headers[name].Value != want {
			t.Errorf("%s = %q, want %q", name, headers[name].Value, want)
		}
	}
	if headers["Host"].Redacted || !headers["Authorization"].Redacted {
		t.Errorf("redacted flags: %+v", headers)
	}

	resp := got.Response
	if resp.StatusCode != 302 || resp.StatusText != "Found" || resp.Body.State != "skipped" {
		t.Fatalf("response = %+v", resp)
	}
	tm := got.Timing
	if tm == nil || tm.TotalMS != 120 || tm.ConnectMS == nil || *tm.ConnectMS != 20 || tm.DNSMS != nil || tm.TLSMS != nil {
		t.Fatalf("timing = %+v", tm)
	}

	original, _ := store.Get("a")
	if original.Request.Headers[1].Value != "Bearer hidden-bearer" {
		t.Fatal("redaction mutated captured traffic")
	}
}

func TestGetRequestStates(t *testing.T) {
	store := storage.NewRequestStore(2)
	deps := Dependencies{Requests: store}
	store.PutRequest("tunnel", storage.RequestRecord{Method: "CONNECT", URL: "example.com:443"})
	store.PutResponse("orphan", storage.ResponseRecord{StatusCode: 407, Headers: []storage.Header{{Name: "Proxy-Authenticate", Value: "NTLM TlRMTVNTUAACAAAA"}}})

	get := func(id string) (RequestDetail, error) {
		_, detail, err := deps.getRequest(context.Background(), nil, GetRequestParams{ID: id})
		return detail, err
	}
	tunnel, err := get("tunnel")
	if err != nil {
		t.Fatal(err)
	}
	if tunnel.State != "no_response" || tunnel.Visibility != "connect" || tunnel.Timing != nil || tunnel.Request.Body.State != "absent" {
		t.Fatalf("tunnel = %+v", tunnel)
	}
	if !hasLimitation(tunnel, "No response recorded") || !hasLimitation(tunnel, "CONNECT opens a tunnel") || !hasLimitation(tunnel, "No timing") {
		t.Fatalf("tunnel limitations = %v", tunnel.Limitations)
	}

	orphan, err := get("orphan")
	if err != nil {
		t.Fatal(err)
	}
	if orphan.State != "response_only" || orphan.Request != nil || orphan.Response.Headers[0].Value != "NTLM REDACTED" || !hasLimitation(orphan, "407") {
		t.Fatalf("orphan = %+v", orphan)
	}

	// A third exchange evicts the first one.
	store.PutRequest("new", storage.RequestRecord{Method: "GET", URL: "http://example.com/"})
	if _, err := get("tunnel"); err == nil || !strings.Contains(err.Error(), "evicted") {
		t.Fatalf("evicted exchange: %v", err)
	}
	if _, err := get(" "); err == nil {
		t.Fatal("expected an error without id")
	}
	if _, _, err := (Dependencies{}).getRequest(context.Background(), nil, GetRequestParams{ID: "a"}); err == nil {
		t.Fatal("expected an error without a store")
	}
}

func TestRedactTrafficHeaderBounds(t *testing.T) {
	long := redactTrafficHeader("X-Trace", strings.Repeat("é", maxHeaderValueLength))
	if !long.Truncated || len(long.Value) > maxHeaderValueLength+len("…") || strings.ContainsRune(long.Value, '�') {
		t.Fatalf("long header = %d bytes, truncated=%v", len(long.Value), long.Truncated)
	}
	for value, want := range map[string]string{
		"":                   "",
		"Negotiate":          "Negotiate",
		"Basic dXNlcjpwdw==": "Basic REDACTED",
		"raw-token-value":    "REDACTED",
		`realm="corp", x=1`:  "REDACTED",
	} {
		if got := redactTrafficHeader("Proxy-Authorization", value).Value; got != want {
			t.Errorf("Proxy-Authorization %q = %q, want %q", value, got, want)
		}
	}

	headers := make([]storage.Header, maxDetailHeaders+3)
	for i := range headers {
		headers[i] = storage.Header{Name: "X-H", Value: "v"}
	}
	kept, cut := summarizeHeaders(headers)
	if len(kept) != maxDetailHeaders || cut != 3 {
		t.Fatalf("kept %d, cut %d", len(kept), cut)
	}
}

func hasLimitation(d RequestDetail, prefix string) bool {
	for _, l := range d.Limitations {
		if strings.Contains(l, prefix) {
			return true
		}
	}
	return false
}
