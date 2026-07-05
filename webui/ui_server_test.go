package webui

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"httpStackLens/storage"
	"httpStackLens/webui/wasm/shared"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestRequestDetailHandlerExposesTiming(t *testing.T) {
	store := storage.NewRequestStore(10)
	store.PutRequest("req-1", storage.RequestRecord{Method: "GET", URL: "https://example.com/"})
	store.PutTiming("req-1", storage.Timing{
		Dns:      10 * time.Millisecond,
		Connect:  20 * time.Millisecond,
		Tls:      30 * time.Millisecond,
		Ttfb:     40 * time.Millisecond,
		Download: 50 * time.Millisecond,
		Total:    150 * time.Millisecond,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/requests/req-1", nil)
	rr := httptest.NewRecorder()
	requestDetailHandler(store).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %q", rr.Code, http.StatusOK, rr.Body.String())
	}

	var dto shared.RequestDetailDto
	if err := json.Unmarshal(rr.Body.Bytes(), &dto); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if dto.Timing == nil {
		t.Fatalf("timing missing in dto = %+v", dto)
	}
	if dto.Timing.DnsMs != 10 || dto.Timing.ConnectMs != 20 || dto.Timing.TlsMs != 30 ||
		dto.Timing.TtfbMs != 40 || dto.Timing.DownloadMs != 50 || dto.Timing.TotalMs != 150 {
		t.Fatalf("timing dto = %+v", dto.Timing)
	}
}

func TestRequestDetailHandlerOmitsTimingWhenAbsent(t *testing.T) {
	store := storage.NewRequestStore(10)
	store.PutRequest("req-1", storage.RequestRecord{Method: "GET", URL: "https://example.com/"})

	req := httptest.NewRequest(http.MethodGet, "/api/requests/req-1", nil)
	rr := httptest.NewRecorder()
	requestDetailHandler(store).ServeHTTP(rr, req)

	var dto shared.RequestDetailDto
	if err := json.Unmarshal(rr.Body.Bytes(), &dto); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if dto.Timing != nil {
		t.Fatalf("timing should be nil, got %+v", dto.Timing)
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

func TestRequestBodyHandlerReturnsRequestBodyWithCapturedContentType(t *testing.T) {
	store := storage.NewRequestStore(10)
	store.PutRequest("req-1", storage.RequestRecord{
		Headers: []storage.Header{
			{Name: "Content-Type", Value: "application/json"},
		},
		Body: []byte(`{"ok":true}`),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/requests/req-1/body?side=request", nil)
	rr := httptest.NewRecorder()
	requestBodyHandler(store).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %q", rr.Code, http.StatusOK, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if got := rr.Body.String(); got != `{"ok":true}` {
		t.Fatalf("body = %q", got)
	}
}

func TestRequestBodyHandlerReturnsResponseBodyWithCapturedContentType(t *testing.T) {
	store := storage.NewRequestStore(10)
	store.PutResponse("req-1", storage.ResponseRecord{
		Headers: []storage.Header{
			{Name: "Content-Type", Value: "text/html; charset=utf-8"},
		},
		Body: []byte("<h1>ok</h1>"),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/requests/req-1/body?side=response", nil)
	rr := httptest.NewRecorder()
	requestBodyHandler(store).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %q", rr.Code, http.StatusOK, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want text/html; charset=utf-8", got)
	}
	if got := rr.Body.String(); got != "<h1>ok</h1>" {
		t.Fatalf("body = %q", got)
	}
}

func TestRequestBodyHandlerRejectsInvalidSide(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/requests/req-1/body?side=other", nil)
	rr := httptest.NewRecorder()
	requestBodyHandler(storage.NewRequestStore(10)).ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestRequestBodyHandlerReturnsNotFoundWhenBodyAbsent(t *testing.T) {
	store := storage.NewRequestStore(10)
	store.PutRequest("req-1", storage.RequestRecord{Body: nil})

	req := httptest.NewRequest(http.MethodGet, "/api/requests/req-1/body?side=request", nil)
	rr := httptest.NewRecorder()
	requestBodyHandler(store).ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestRequestBodyHandlerHonorsBodySkipped(t *testing.T) {
	store := storage.NewRequestStore(10)
	store.PutResponse("req-1", storage.ResponseRecord{BodySkipped: true})

	req := httptest.NewRequest(http.MethodGet, "/api/requests/req-1/body?side=response", nil)
	rr := httptest.NewRecorder()
	requestBodyHandler(store).ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusRequestEntityTooLarge)
	}
	if got := rr.Header().Get("X-Body-Skipped"); got != "true" {
		t.Fatalf("X-Body-Skipped = %q, want true", got)
	}
}

func TestCaptureStateHandlerReturnsState(t *testing.T) {
	store := storage.NewRequestStore(10)
	store.PutRequest("req-1", storage.RequestRecord{Method: "GET"})
	ctl := storage.NewCaptureController(true)

	req := httptest.NewRequest(http.MethodGet, "/api/capture/state", nil)
	rr := httptest.NewRecorder()
	captureStateHandler(ctl, store).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %q", rr.Code, http.StatusOK, rr.Body.String())
	}
	var got shared.CaptureStateDto
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !got.Capturing || got.BufferSize != 1 {
		t.Fatalf("state = %+v, want capturing true and buffer size 1", got)
	}
}

func TestCapturePauseResumeHandlersToggleState(t *testing.T) {
	hub := newHub()
	defer hub.Close()
	store := storage.NewRequestStore(10)
	ctl := storage.NewCaptureController(true)
	var persisted []bool
	persist := func(enabled bool) error {
		persisted = append(persisted, enabled)
		return nil
	}

	req := httptest.NewRequest(http.MethodPost, "/api/capture/pause", nil)
	rr := httptest.NewRecorder()
	capturePauseHandler(hub, ctl, store, persist).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("pause status = %d, want %d; body %q", rr.Code, http.StatusOK, rr.Body.String())
	}
	if ctl.IsCapturing() {
		t.Fatal("controller still capturing after pause")
	}
	if len(persisted) != 1 || persisted[0] {
		t.Fatalf("persisted after pause = %v, want [false]", persisted)
	}

	var paused shared.CaptureStateDto
	if err := json.Unmarshal(rr.Body.Bytes(), &paused); err != nil {
		t.Fatalf("decode pause response: %v", err)
	}
	if paused.Capturing {
		t.Fatalf("pause state = %+v, want capturing false", paused)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/capture/resume", nil)
	rr = httptest.NewRecorder()
	captureResumeHandler(hub, ctl, store, persist).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("resume status = %d, want %d; body %q", rr.Code, http.StatusOK, rr.Body.String())
	}
	if !ctl.IsCapturing() {
		t.Fatal("controller not capturing after resume")
	}
	if len(persisted) != 2 || !persisted[1] {
		t.Fatalf("persisted after resume = %v, want [false true]", persisted)
	}
}

func TestCapturePauseHandlerDoesNotChangeStateWhenPersistenceFails(t *testing.T) {
	ctl := storage.NewCaptureController(true)
	persistErr := errors.New("disk full")
	req := httptest.NewRequest(http.MethodPost, "/api/capture/pause", nil)
	rr := httptest.NewRecorder()

	capturePauseHandler(nil, ctl, storage.NewRequestStore(10), func(bool) error {
		return persistErr
	}).ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
	if !ctl.IsCapturing() {
		t.Fatal("capture state changed despite persistence failure")
	}
}

func TestCaptureClearHandlerClearsServerBuffer(t *testing.T) {
	hub := newHub()
	defer hub.Close()
	store := storage.NewRequestStore(10)
	store.PutRequest("req-1", storage.RequestRecord{Method: "GET"})
	ctl := storage.NewCaptureController(false)

	req := httptest.NewRequest(http.MethodPost, "/api/capture/clear", nil)
	rr := httptest.NewRecorder()
	captureClearHandler(hub, ctl, store).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %q", rr.Code, http.StatusOK, rr.Body.String())
	}
	if got := store.Len(); got != 0 {
		t.Fatalf("store Len = %d, want 0", got)
	}
	var got shared.CaptureStateDto
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Capturing || got.BufferSize != 0 {
		t.Fatalf("state = %+v, want capturing false and buffer size 0", got)
	}
}

func TestCapturePauseHandlerRejectsNonPost(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/capture/pause", nil)
	rr := httptest.NewRecorder()
	capturePauseHandler(nil, storage.NewCaptureController(true), storage.NewRequestStore(10), nil).ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
	if got := rr.Header().Get("Allow"); got != http.MethodPost {
		t.Fatalf("Allow = %q, want POST", got)
	}
}

func TestCaptureListHandlerReturnsCaptureFilesNewestFirst(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.capture")
	second := filepath.Join(dir, "second.capture")
	writeTestCapture(t, first, false)
	writeTestCapture(t, second, true)
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignore"), 0o644); err != nil {
		t.Fatalf("write notes: %v", err)
	}

	oldTime := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	newTime := oldTime.Add(time.Hour)
	if err := os.Chtimes(first, oldTime, oldTime); err != nil {
		t.Fatalf("chtime first: %v", err)
	}
	if err := os.Chtimes(second, newTime, newTime); err != nil {
		t.Fatalf("chtime second: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/captures", nil)
	rr := httptest.NewRecorder()
	captureListHandler(dir).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %q", rr.Code, http.StatusOK, rr.Body.String())
	}

	var got []shared.CaptureFileDto
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("captures = %+v, want 2 entries", got)
	}
	if got[0].Name != "second.capture" || got[1].Name != "first.capture" {
		t.Fatalf("capture order = %+v", got)
	}
}

func TestCaptureMetadataHandlerReturnsResolvedRecordCount(t *testing.T) {
	dir := t.TempDir()
	writeTestCapture(t, filepath.Join(dir, "session.capture"), true)

	req := httptest.NewRequest(http.MethodGet, "/api/captures/session.capture/metadata", nil)
	rr := httptest.NewRecorder()
	capturesAPIHandler(dir).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %q", rr.Code, http.StatusOK, rr.Body.String())
	}

	var got shared.CaptureMetadataDto
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Name != "session.capture" || !got.HttpsDecrypted || got.Version != storage.CaptureFormatVersion {
		t.Fatalf("metadata = %+v", got)
	}
	if got.RecordsCount != 2 {
		t.Fatalf("RecordsCount = %d, want 2", got.RecordsCount)
	}
}

func TestCaptureRecordsHandlerReturnsRecordsPage(t *testing.T) {
	dir := t.TempDir()
	writeTestCapture(t, filepath.Join(dir, "session.capture"), true)

	req := httptest.NewRequest(http.MethodGet, "/api/captures/session.capture/records?offset=0&limit=1", nil)
	rr := httptest.NewRecorder()
	capturesAPIHandler(dir).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %q", rr.Code, http.StatusOK, rr.Body.String())
	}

	var got shared.CaptureRecordsDto
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Name != "session.capture" || got.Offset != 0 || got.Limit != 1 || !got.HasMore || got.NextOffset != 1 {
		t.Fatalf("page metadata = %+v", got)
	}
	if len(got.Records) != 1 {
		t.Fatalf("records = %+v, want 1", got.Records)
	}
	rec := got.Records[0]
	if rec.Index != 0 || rec.Type != "request" || rec.Request == nil {
		t.Fatalf("record = %+v", rec)
	}
	if rec.Request.Method != "POST" || rec.Request.URL != "https://example.com/api" {
		t.Fatalf("request record = %+v", rec.Request)
	}
	if rec.Request.BodyBase64 != base64.StdEncoding.EncodeToString([]byte("request body")) {
		t.Fatalf("request body base64 = %q", rec.Request.BodyBase64)
	}
}

func TestCaptureRecordsHandlerReturnsLaterPage(t *testing.T) {
	dir := t.TempDir()
	writeTestCapture(t, filepath.Join(dir, "session.capture"), true)

	req := httptest.NewRequest(http.MethodGet, "/api/captures/session.capture/records?offset=1&limit=10", nil)
	rr := httptest.NewRecorder()
	capturesAPIHandler(dir).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %q", rr.Code, http.StatusOK, rr.Body.String())
	}

	var got shared.CaptureRecordsDto
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.HasMore || got.NextOffset != 2 {
		t.Fatalf("page metadata = %+v", got)
	}
	if len(got.Records) != 1 || got.Records[0].Type != "response" || got.Records[0].Response == nil {
		t.Fatalf("records = %+v", got.Records)
	}
	if got.Records[0].Response.Status != 201 {
		t.Fatalf("response record = %+v", got.Records[0].Response)
	}
}

func TestCapturesAPIHandlerRejectsPathTraversal(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/captures/../secret.capture/metadata", nil)
	rr := httptest.NewRecorder()
	capturesAPIHandler(t.TempDir()).ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestCaptureRecordsHandlerRejectsBadPagination(t *testing.T) {
	dir := t.TempDir()
	writeTestCapture(t, filepath.Join(dir, "session.capture"), true)

	req := httptest.NewRequest(http.MethodGet, "/api/captures/session.capture/records?offset=-1", nil)
	rr := httptest.NewRecorder()
	capturesAPIHandler(dir).ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func writeTestCapture(t *testing.T, path string, httpsDecrypted bool) {
	t.Helper()
	writer, err := storage.NewFileCaptureSessionWriter(path, httpsDecrypted)
	if err != nil {
		t.Fatalf("create capture: %v", err)
	}
	id := storage.UUID{0, 1, 2, 3, 4, 5, 0x46, 7, 0x88, 9, 10, 11, 12, 13, 14, 15}
	if err := writer.WriteRequest(storage.RequestRecord{
		RequestID:   id,
		Method:      "POST",
		URL:         "https://example.com/api",
		HttpVersion: storage.HttpVersion11,
		Headers: []storage.Header{
			{Name: "Content-Type", Value: "text/plain"},
		},
		Body: []byte("request body"),
	}); err != nil {
		t.Fatalf("write request: %v", err)
	}
	if err := writer.WriteResponse(storage.ResponseRecord{
		RequestID:     id,
		HttpVersion:   storage.HttpVersion11,
		StatusCode:    201,
		StatusMessage: "Created",
		Headers: []storage.Header{
			{Name: "Content-Type", Value: "application/json"},
		},
		Body: []byte(`{"created":true}`),
	}); err != nil {
		t.Fatalf("write response: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close capture: %v", err)
	}
}
