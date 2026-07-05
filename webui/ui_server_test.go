package webui

import (
	"encoding/json"
	"httpStackLens/storage"
	"httpStackLens/webui/wasm/shared"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRequestDetailDtoIncludesMetadataAndHeaders(t *testing.T) {
	createdAt := time.Date(2026, 7, 5, 12, 30, 15, 123, time.FixedZone("test", 2*60*60))
	exchange := storage.CapturedExchange{
		CorrelationID: "req-1",
		CreatedAt:     createdAt,
		Request: &storage.RequestRecord{
			Method:      "GET",
			URL:         "https://example.com/api",
			HttpVersion: storage.HttpVersion11,
			Headers: []storage.Header{
				{Name: "Accept", Value: "application/json"},
				{Name: "X-Trace", Value: "abc"},
			},
			Body: []byte("hello"),
		},
		Response: &storage.ResponseRecord{
			StatusCode:    200,
			StatusMessage: "OK",
			HttpVersion:   storage.HttpVersion20,
			Headers: []storage.Header{
				{Name: "Content-Type", Value: "application/json"},
			},
			BodySkipped: true,
		},
	}

	got := requestDetailDto(exchange)

	if got.CorrelationID != "req-1" {
		t.Fatalf("CorrelationID = %q, want req-1", got.CorrelationID)
	}
	if got.CreatedAt != createdAt.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("CreatedAt = %q, want UTC RFC3339Nano", got.CreatedAt)
	}
	if got.Request == nil {
		t.Fatal("Request is nil")
	}
	if got.Request.Method != "GET" || got.Request.URL != "https://example.com/api" || got.Request.HttpVersion != "HTTP/1.1" {
		t.Fatalf("request metadata = %+v", got.Request)
	}
	if !got.Request.BodyAvailable || got.Request.BodySkipped || got.Request.BodySize != 5 {
		t.Fatalf("request body metadata = available %v skipped %v size %d", got.Request.BodyAvailable, got.Request.BodySkipped, got.Request.BodySize)
	}
	if len(got.Request.Headers) != 2 || got.Request.Headers[0].Name != "Accept" || got.Request.Headers[1].Value != "abc" {
		t.Fatalf("request headers = %+v", got.Request.Headers)
	}
	if got.Response == nil {
		t.Fatal("Response is nil")
	}
	if got.Response.Status != 200 || got.Response.StatusText != "OK" || got.Response.HttpVersion != "HTTP/2.0" {
		t.Fatalf("response metadata = %+v", got.Response)
	}
	if got.Response.BodyAvailable || !got.Response.BodySkipped || got.Response.BodySize != 0 {
		t.Fatalf("response body metadata = available %v skipped %v size %d", got.Response.BodyAvailable, got.Response.BodySkipped, got.Response.BodySize)
	}
	if len(got.Response.Headers) != 1 || got.Response.Headers[0].Name != "Content-Type" {
		t.Fatalf("response headers = %+v", got.Response.Headers)
	}
}

func TestRequestDetailHandlerReturnsStoredExchange(t *testing.T) {
	store := storage.NewRequestStore(10)
	store.PutRequest("req-1", storage.RequestRecord{
		Method:      "POST",
		URL:         "http://example.com/upload",
		HttpVersion: storage.HttpVersion11,
		Headers: []storage.Header{
			{Name: "Content-Type", Value: "text/plain"},
		},
		Body: []byte("abc"),
	})
	store.PutResponse("req-1", storage.ResponseRecord{
		StatusCode:    201,
		StatusMessage: "Created",
		HttpVersion:   storage.HttpVersion11,
		Headers: []storage.Header{
			{Name: "Location", Value: "/upload/1"},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/requests/req-1", nil)
	rr := httptest.NewRecorder()
	requestDetailHandler(store).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %q", rr.Code, http.StatusOK, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	var dto shared.RequestDetailDto
	if err := json.Unmarshal(rr.Body.Bytes(), &dto); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if dto.CorrelationID != "req-1" || dto.Request == nil || dto.Response == nil {
		t.Fatalf("dto = %+v", dto)
	}
	if dto.Request.Method != "POST" || dto.Request.Headers[0].Value != "text/plain" {
		t.Fatalf("request dto = %+v", dto.Request)
	}
	if dto.Response.Status != 201 || dto.Response.Headers[0].Value != "/upload/1" {
		t.Fatalf("response dto = %+v", dto.Response)
	}
}

func TestRequestDetailHandlerNotFound(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/requests/missing", nil)
	rr := httptest.NewRecorder()
	requestDetailHandler(storage.NewRequestStore(10)).ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestRequestDetailHandlerRejectsNonGet(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/requests/req-1", nil)
	rr := httptest.NewRecorder()
	requestDetailHandler(storage.NewRequestStore(10)).ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
	if got := rr.Header().Get("Allow"); got != http.MethodGet {
		t.Fatalf("Allow = %q, want GET", got)
	}
}
