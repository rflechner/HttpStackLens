package webui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"httpStackLens/webui/wasm/shared"
)

func TestHttpFileNameAddsExtension(t *testing.T) {
	name, err := httpFileName("  github  ")
	if err != nil {
		t.Fatalf("expected a valid name, got %v", err)
	}
	if name != "github.http" {
		t.Fatalf("expected github.http, got %q", name)
	}
}

func TestHttpFileNameKeepsExistingExtension(t *testing.T) {
	name, err := httpFileName("corp-auth.http")
	if err != nil {
		t.Fatalf("expected a valid name, got %v", err)
	}
	if name != "corp-auth.http" {
		t.Fatalf("expected corp-auth.http, got %q", name)
	}
}

// A name is the only part of a path a client controls, so the traversal cases
// are the ones worth pinning down.
func TestHttpFileNameRefusesPathsAndReservedCharacters(t *testing.T) {
	refused := []string{
		"",
		"   ",
		"..",
		"../escape.http",
		"..\\escape.http",
		"nested/file.http",
		"nested\\file.http",
		"C:/absolute.http",
		".hidden.http",
		"pipe|name.http",
		"star*.http",
		"quote\"name.http",
		strings.Repeat("x", 200) + ".http",
	}
	for _, name := range refused {
		if got, err := httpFileName(name); err == nil {
			t.Errorf("expected %q to be refused, got %q", name, got)
		}
	}
}

func TestStoreSaveListRenameDelete(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "http_files")
	store := newHttpFileStore(folder)

	// The folder does not exist yet: that is an empty collection, not a failure.
	files, err := store.List()
	if err != nil {
		t.Fatalf("List on a missing folder: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected no file, got %d", len(files))
	}

	if _, err := store.Save("github", "### Current user\nGET https://api.github.com/user\n"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := store.Save("corp-auth.http", "### Token\nPOST https://auth.corp.local/oauth2/token\n"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	files, err = store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	// Sorted by name, so the sidebar does not reshuffle between two reads.
	if files[0].Name != "corp-auth.http" || files[1].Name != "github.http" {
		t.Fatalf("unexpected order: %s, %s", files[0].Name, files[1].Name)
	}
	if !strings.Contains(files[1].Content, "api.github.com") {
		t.Fatalf("the content did not come back: %q", files[1].Content)
	}

	renamed, err := store.Rename("github.http", "github-api")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if renamed.Name != "github-api.http" {
		t.Fatalf("expected github-api.http, got %q", renamed.Name)
	}
	if !strings.Contains(renamed.Content, "api.github.com") {
		t.Fatalf("rename lost the content: %q", renamed.Content)
	}
	if _, err := os.Stat(filepath.Join(folder, "github.http")); !os.IsNotExist(err) {
		t.Fatalf("the old name is still on disk")
	}

	if err := store.Delete("github-api.http"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	// Deleting twice is the outcome the caller wanted either way.
	if err := store.Delete("github-api.http"); err != nil {
		t.Fatalf("Delete on a missing file: %v", err)
	}
}

func TestStoreRenameRefusesAnExistingTarget(t *testing.T) {
	store := newHttpFileStore(t.TempDir())
	if _, err := store.Save("a.http", "### A\nGET https://example.com/a\n"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := store.Save("b.http", "### B\nGET https://example.com/b\n"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, err := store.Rename("a.http", "b.http"); err == nil {
		t.Fatal("expected the rename to be refused")
	}
	content, err := os.ReadFile(filepath.Join(store.Folder(), "b.http"))
	if err != nil {
		t.Fatalf("reading b.http: %v", err)
	}
	if !strings.Contains(string(content), "example.com/b") {
		t.Fatalf("b.http was overwritten: %q", content)
	}
}

func TestStoreListIgnoresOtherFiles(t *testing.T) {
	folder := t.TempDir()
	store := newHttpFileStore(folder)
	if err := os.WriteFile(filepath.Join(folder, "README.md"), []byte("notes"), 0o644); err != nil {
		t.Fatalf("writing README.md: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(folder, "archive.http"), 0o755); err != nil {
		t.Fatalf("creating the folder: %v", err)
	}
	if _, err := store.Save("real.http", "### One\nGET https://example.com\n"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	files, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 1 || files[0].Name != "real.http" {
		t.Fatalf("expected only real.http, got %+v", files)
	}
}

func TestHttpFilesListHandlerReportsTheFolder(t *testing.T) {
	store := newHttpFileStore(t.TempDir())
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
	store := newHttpFileStore(t.TempDir())

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
	store := newHttpFileStore(filepath.Join(folder, "http_files"))

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
	store := newHttpFileStore(t.TempDir())
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
	store := newHttpFileStore(t.TempDir())

	req := httptest.NewRequest(http.MethodPost, "/api/http-files/gone.http/rename", strings.NewReader(`{"name":"other.http"}`))
	req.SetPathValue("name", "gone.http")
	res := httptest.NewRecorder()
	httpFileRenameHandler(store)(res, req)

	if res.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", res.Code, res.Body.String())
	}
}

func TestHttpFileDeleteHandler(t *testing.T) {
	store := newHttpFileStore(t.TempDir())
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
	registerHttpFileRoutes(mux, newHttpFileStore(folder))

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
