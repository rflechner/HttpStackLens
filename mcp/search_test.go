package mcp

import (
	"context"
	"encoding/json"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"httpStackLens/storage"
	"strings"
	"testing"
	"time"
)

func TestSearchRequestsOverMCP(t *testing.T) {
	store := storage.NewRequestStore(10)
	store.PutRequest("a", storage.RequestRecord{Method: "GET", URL: "https://user:password@example.com/api?token=secret#private", Headers: []storage.Header{{Name: "Authorization", Value: "Bearer hidden"}}, Body: []byte("body-secret")})
	store.PutResponse("a", storage.ResponseRecord{StatusCode: 503, Body: []byte("response-secret")})
	store.PutTiming("a", storage.Timing{Total: 1500 * time.Millisecond})
	store.PutRequest("b", storage.RequestRecord{Method: "CONNECT", URL: "example.com:443"})
	ctx, session := connectStatusClient(t, Dependencies{Requests: store})
	result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "search_requests", Arguments: map[string]any{"host": "EXAMPLE.COM", "method": "get", "path_contains": "/api", "status_code": 503, "has_response": true, "since": time.Now().Add(-time.Minute).Format(time.RFC3339), "until": time.Now().Add(time.Minute).Format(time.RFC3339)}})
	if err != nil || result.IsError {
		t.Fatalf("call: %v %+v", err, result)
	}
	all, _ := json.Marshal(result)
	for _, secret := range []string{"password", "secret", "private", "hidden", "Authorization"} {
		if strings.Contains(string(all), secret) {
			t.Fatalf("leaked %q: %s", secret, all)
		}
	}
	data, _ := json.Marshal(result.StructuredContent)
	var got SearchResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Requests) != 1 {
		t.Fatalf("unexpected results: %+v", got)
	}
	r := got.Requests[0]
	if r.ID != "a" || r.URL != "https://example.com/api?REDACTED" || !r.URLRedacted || r.StatusCode == nil || *r.StatusCode != 503 || r.DurationMS == nil || *r.DurationMS != 1500 || r.Visibility != "https_request" {
		t.Fatalf("incorrect summary: %+v", r)
	}
	original, _ := store.Get("a")
	if original.Request.URL != "https://user:password@example.com/api?token=secret#private" {
		t.Fatal("redaction mutated captured traffic")
	}
}

func TestSearchPaginationAndInvalidation(t *testing.T) {
	store := storage.NewRequestStore(4)
	deps := Dependencies{Requests: store}
	for _, id := range []string{"a", "b", "c"} {
		store.PutRequest(id, storage.RequestRecord{Method: "GET", URL: "http://example.com/"})
	}
	call := func(p SearchParams) SearchResult {
		t.Helper()
		_, r, err := deps.searchRequests(context.Background(), nil, p)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	first := call(SearchParams{Limit: 1})
	if len(first.Requests) != 1 || first.Requests[0].ID != "c" || first.NextCursor == "" {
		t.Fatalf("first page: %+v", first)
	}
	store.PutRequest("d", storage.RequestRecord{Method: "GET"})
	store.PutResponse("b", storage.ResponseRecord{StatusCode: 200})
	second := call(SearchParams{Limit: 1, Cursor: first.NextCursor})
	if second.Requests[0].ID != "b" || !second.Requests[0].HasResponse {
		t.Fatalf("second page: %+v", second)
	}
	third := call(SearchParams{Limit: 1, Cursor: second.NextCursor})
	if third.Requests[0].ID != "a" || third.NextCursor != "" {
		t.Fatalf("third page: %+v", third)
	}
	for _, p := range []SearchParams{{Cursor: "garbage"}, {Cursor: first.NextCursor, Host: "different"}, {Limit: 101}, {Limit: -1}, {StatusCode: 99}, {Since: "yesterday"}, {Until: "bad"}, {Since: "2026-10-11T00:00:00Z", Until: "2026-10-10T00:00:00Z"}} {
		if _, _, err := deps.searchRequests(context.Background(), nil, p); err == nil {
			t.Fatalf("expected validation error: %+v", p)
		}
	}
	store.PutRequest("e", storage.RequestRecord{})
	if _, _, err := deps.searchRequests(context.Background(), nil, SearchParams{Cursor: first.NextCursor}); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("eviction must expire cursor: %v", err)
	}
	fresh := call(SearchParams{Limit: 1})
	store.Clear()
	if _, _, err := deps.searchRequests(context.Background(), nil, SearchParams{Cursor: fresh.NextCursor}); err == nil {
		t.Fatal("clear must expire cursor")
	}
	empty := call(SearchParams{})
	if empty.Requests == nil || len(empty.Requests) != 0 {
		t.Fatal("empty results must be an array")
	}
}

func TestSearchRequestFormsAndMissingData(t *testing.T) {
	store := storage.NewRequestStore(10)
	store.PutRequest("origin", storage.RequestRecord{Method: "GET", URL: "/api?a=secret", Headers: []storage.Header{{Name: "Host", Value: "example.com:8080"}}})
	store.PutRequest("connect", storage.RequestRecord{Method: "CONNECT", URL: "[::1]:443"})
	store.PutRequest("invalid", storage.RequestRecord{Method: "GET", URL: "http://example.com/%invalid?secret"})
	store.PutResponse("orphan", storage.ResponseRecord{StatusCode: 502})
	deps := Dependencies{Requests: store}
	no := false
	_, r, err := deps.searchRequests(context.Background(), nil, SearchParams{Host: "example.com", HasResponse: &no})
	if err != nil || len(r.Requests) != 1 || r.Requests[0].ID != "origin" || r.Requests[0].URL != "//example.com:8080/api?REDACTED" || r.Requests[0].StatusCode != nil || r.Requests[0].DurationMS != nil {
		t.Fatalf("origin: %+v %v", r, err)
	}
	_, r, err = deps.searchRequests(context.Background(), nil, SearchParams{Host: "::1"})
	if err != nil || len(r.Requests) != 1 || r.Requests[0].Visibility != "connect" {
		t.Fatalf("CONNECT: %+v %v", r, err)
	}
	_, r, err = deps.searchRequests(context.Background(), nil, SearchParams{})
	if err != nil || r.Requests[0].HasRequest || r.Requests[1].URL != "" || !r.Requests[1].URLRedacted {
		t.Fatalf("missing/invalid: %+v %v", r, err)
	}
	if _, _, err := (Dependencies{}).searchRequests(context.Background(), nil, SearchParams{}); err == nil {
		t.Fatal("missing store must fail")
	}
}
