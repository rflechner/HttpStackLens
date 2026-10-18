package webui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"httpStackLens/composer"
	"httpStackLens/webui/wasm/shared"
)

func TestHttpFilesListHandlerReportsTheFolder(t *testing.T) {
	store := composer.NewHttpFileStore(t.TempDir())
	if _, err := store.Save("one.http", "### One\nGET https://example.com\n"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	res := httptest.NewRecorder()
	httpFilesListHandler(store)(res, httptest.NewRequest(http.MethodGet, "/api/http-files", nil))

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}
	var dto shared.HttpFilesDto
	if err := json.Unmarshal(res.Body.Bytes(), &dto); err != nil {
		t.Fatalf("unmarshaling the answer: %v", err)
	}
	if dto.Folder != store.Folder() {
		t.Fatalf("expected the folder %q, got %q", store.Folder(), dto.Folder)
	}
	if len(dto.Files) != 1 || dto.Files[0].Name != "one.http" {
		t.Fatalf("unexpected files: %+v", dto.Files)
	}
}

func TestHttpFileSaveHandlerWritesTheBody(t *testing.T) {
	store := composer.NewHttpFileStore(t.TempDir())

	body := strings.NewReader("### Health\nGET https://example.com/health\n")
	req := httptest.NewRequest(http.MethodPut, "/api/http-files/health.http", body)
	req.SetPathValue("name", "health.http")
	res := httptest.NewRecorder()
	httpFileSaveHandler(store)(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", res.Code, res.Body.String())
	}
	written, err := os.ReadFile(filepath.Join(store.Folder(), "health.http"))
	if err != nil {
		t.Fatalf("reading the saved file: %v", err)
	}
	if !strings.Contains(string(written), "example.com/health") {
		t.Fatalf("unexpected content: %q", written)
	}
}

func TestHttpFileSaveHandlerRefusesAnEscapingName(t *testing.T) {
	folder := t.TempDir()
	store := composer.NewHttpFileStore(filepath.Join(folder, "http_files"))

	req := httptest.NewRequest(http.MethodPut, "/api/http-files/x", strings.NewReader("### x\nGET https://example.com\n"))
	req.SetPathValue("name", "../escaped.http")
	res := httptest.NewRecorder()
	httpFileSaveHandler(store)(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.Code)
	}
	if _, err := os.Stat(filepath.Join(folder, "escaped.http")); !os.IsNotExist(err) {
		t.Fatal("a file was written outside the folder")
	}
}

func TestHttpFileRenameHandlerReportsAConflict(t *testing.T) {
	store := composer.NewHttpFileStore(t.TempDir())
	if _, err := store.Save("a.http", "### A\nGET https://example.com/a\n"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := store.Save("b.http", "### B\nGET https://example.com/b\n"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/http-files/a.http/rename", strings.NewReader(`{"name":"b.http"}`))
	req.SetPathValue("name", "a.http")
	res := httptest.NewRecorder()
	httpFileRenameHandler(store)(res, req)

	if res.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", res.Code, res.Body.String())
	}
}

func TestHttpFileRenameHandlerReportsAMissingFile(t *testing.T) {
	store := composer.NewHttpFileStore(t.TempDir())

	req := httptest.NewRequest(http.MethodPost, "/api/http-files/gone.http/rename", strings.NewReader(`{"name":"other.http"}`))
	req.SetPathValue("name", "gone.http")
	res := httptest.NewRecorder()
	httpFileRenameHandler(store)(res, req)

	if res.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", res.Code, res.Body.String())
	}
}

func TestHttpFileDeleteHandler(t *testing.T) {
	store := composer.NewHttpFileStore(t.TempDir())
	if _, err := store.Save("gone.http", "### Gone\nGET https://example.com\n"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/http-files/gone.http", nil)
	req.SetPathValue("name", "gone.http")
	res := httptest.NewRecorder()
	httpFileDeleteHandler(store)(res, req)

	if res.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", res.Code)
	}
	if _, err := os.Stat(filepath.Join(store.Folder(), "gone.http")); !os.IsNotExist(err) {
		t.Fatal("the file is still on disk")
	}
}

// The routes are worth a test of their own: ServeMux panics on a conflicting
// pattern at registration, and the name in the path must not swallow
// /open-folder.
func TestHttpFileRoutes(t *testing.T) {
	folder := t.TempDir()
	mux := http.NewServeMux()
	registerHttpFileRoutes(mux, composer.NewHttpFileStore(folder))

	put := httptest.NewRecorder()
	mux.ServeHTTP(put, httptest.NewRequest(http.MethodPut, "/api/http-files/orders.http",
		strings.NewReader("### Orders\nGET https://example.com/orders\n")))
	if put.Code != http.StatusOK {
		t.Fatalf("PUT: expected 200, got %d: %s", put.Code, put.Body.String())
	}

	list := httptest.NewRecorder()
	mux.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/http-files", nil))
	if list.Code != http.StatusOK {
		t.Fatalf("GET: expected 200, got %d", list.Code)
	}
	var dto shared.HttpFilesDto
	if err := json.Unmarshal(list.Body.Bytes(), &dto); err != nil {
		t.Fatalf("unmarshaling the list: %v", err)
	}
	if len(dto.Files) != 1 || dto.Files[0].Name != "orders.http" {
		t.Fatalf("unexpected files: %+v", dto.Files)
	}

	rename := httptest.NewRecorder()
	mux.ServeHTTP(rename, httptest.NewRequest(http.MethodPost, "/api/http-files/orders.http/rename",
		strings.NewReader(`{"name":"sales.http"}`)))
	if rename.Code != http.StatusOK {
		t.Fatalf("rename: expected 200, got %d: %s", rename.Code, rename.Body.String())
	}
	if _, err := os.Stat(filepath.Join(folder, "sales.http")); err != nil {
		t.Fatalf("sales.http is not on disk: %v", err)
	}

	del := httptest.NewRecorder()
	mux.ServeHTTP(del, httptest.NewRequest(http.MethodDelete, "/api/http-files/sales.http", nil))
	if del.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", del.Code)
	}
}
