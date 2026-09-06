package webui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"httpStackLens/helpers"
	"httpStackLens/webui/wasm/shared"
)

// httpFileExt is the only extension the composer's collection holds. Anything
// else in the folder is left alone: the folder is a normal one a developer may
// keep notes or a README in.
const httpFileExt = ".http"

// maxHttpFileBytes caps what a single save may write. A .http file is hand
// written text; a megabyte of it is already far past anything reasonable, and
// the limit keeps a runaway client from filling the disk.
const maxHttpFileBytes = 1 << 20

var (
	errInvalidHttpFileName = errors.New("invalid .http file name")
	errHttpFileExists      = errors.New(".http file already exists")
)

// httpFileStore is the composer's collection of `.http` files on disk. The
// folder comes from config.yaml (`http_files.folder`), and every path handed to
// the filesystem is rebuilt from a validated base name, so a name coming off
// the wire can never point outside it.
type httpFileStore struct {
	folder string
}

func newHttpFileStore(folder string) *httpFileStore {
	return &httpFileStore{folder: folder}
}

// Folder is the location shown in the Web UI and opened in the file manager.
func (s *httpFileStore) Folder() string { return s.folder }

// List reads every .http file in the folder. A folder that does not exist yet
// is not an error: it simply holds no file, which is what the composer shows
// before the first one is created.
func (s *httpFileStore) List() ([]shared.HttpFileDto, error) {
	entries, err := os.ReadDir(s.folder)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []shared.HttpFileDto{}, nil
		}
		return nil, err
	}

	files := make([]shared.HttpFileDto, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), httpFileExt) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		if info.Size() > maxHttpFileBytes {
			log.Printf("Skipping %s: larger than the %d byte .http limit", entry.Name(), maxHttpFileBytes)
			continue
		}
		content, err := os.ReadFile(filepath.Join(s.folder, entry.Name()))
		if err != nil {
			return nil, err
		}
		files = append(files, shared.HttpFileDto{
			Name:       entry.Name(),
			Content:    string(content),
			ModifiedAt: info.ModTime().UTC().Format(time.RFC3339Nano),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})
	return files, nil
}

// Save writes a file, creating the folder on the way. It is a plain overwrite:
// the composer is the only writer and it sends the whole file every time.
func (s *httpFileStore) Save(name, content string) (shared.HttpFileDto, error) {
	safe, err := httpFileName(name)
	if err != nil {
		return shared.HttpFileDto{}, err
	}
	if len(content) > maxHttpFileBytes {
		return shared.HttpFileDto{}, fmt.Errorf("%s is larger than the %d byte limit", safe, maxHttpFileBytes)
	}
	if err := os.MkdirAll(s.folder, 0o755); err != nil {
		return shared.HttpFileDto{}, err
	}
	if err := os.WriteFile(filepath.Join(s.folder, safe), []byte(content), 0o644); err != nil {
		return shared.HttpFileDto{}, err
	}
	return s.describe(safe, content), nil
}

// Rename moves a file inside the folder. An existing target is refused rather
// than overwritten: renaming onto another collection would silently destroy it.
func (s *httpFileStore) Rename(oldName, newName string) (shared.HttpFileDto, error) {
	from, err := httpFileName(oldName)
	if err != nil {
		return shared.HttpFileDto{}, err
	}
	to, err := httpFileName(newName)
	if err != nil {
		return shared.HttpFileDto{}, err
	}
	if from != to {
		// A case-only rename on Windows and macOS would read as "the target
		// exists", because the filesystem matches the two names. os.Rename
		// handles that one itself, so only a genuinely different name is
		// checked.
		if !strings.EqualFold(from, to) {
			if _, err := os.Stat(filepath.Join(s.folder, to)); err == nil {
				return shared.HttpFileDto{}, errHttpFileExists
			}
		}
		if err := os.Rename(filepath.Join(s.folder, from), filepath.Join(s.folder, to)); err != nil {
			return shared.HttpFileDto{}, err
		}
	}

	content, err := os.ReadFile(filepath.Join(s.folder, to))
	if err != nil {
		return shared.HttpFileDto{}, err
	}
	return s.describe(to, string(content)), nil
}

// Delete removes a file. Deleting one that is already gone succeeds: the
// composer only asks to close what it believes is there, and the outcome the
// caller wants — no such file — is the one it gets.
func (s *httpFileStore) Delete(name string) error {
	safe, err := httpFileName(name)
	if err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(s.folder, safe)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// OpenFolder shows the folder in the file manager of the machine running
// HttpStackLens.
func (s *httpFileStore) OpenFolder() error {
	return helpers.OpenFolderInFileManager(s.folder)
}

func (s *httpFileStore) describe(name, content string) shared.HttpFileDto {
	dto := shared.HttpFileDto{Name: name, Content: content}
	if info, err := os.Stat(filepath.Join(s.folder, name)); err == nil {
		dto.ModifiedAt = info.ModTime().UTC().Format(time.RFC3339Nano)
	}
	return dto
}

// reservedHttpFileChars is Windows' reserved set, applied on every platform so
// a collection stays portable between the machines one developer works on.
const reservedHttpFileChars = `<>:"|?*`

// httpFileName turns what a client sent into a base name inside the folder, or
// refuses it. The extension is added when it is missing so that a name typed in
// the Web UI, or by hand against the API, does not have to carry it.
func httpFileName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", errInvalidHttpFileName
	}
	if !strings.EqualFold(filepath.Ext(trimmed), httpFileExt) {
		trimmed += httpFileExt
	}
	// Base name only: a separator, a drive letter or a `..` segment would point
	// somewhere other than the collection.
	if trimmed != filepath.Base(trimmed) || strings.ContainsAny(trimmed, `/\`) {
		return "", errInvalidHttpFileName
	}
	if strings.HasPrefix(trimmed, ".") || len(trimmed) > 128 {
		return "", errInvalidHttpFileName
	}
	for _, r := range trimmed {
		if r < 0x20 || strings.ContainsRune(reservedHttpFileChars, r) {
			return "", errInvalidHttpFileName
		}
	}
	return trimmed, nil
}

// ── handlers ───────────────────────────────────────────────────────────────

// registerHttpFileRoutes wires the collection onto a mux. The routes spell the
// method out because the last segment is a file name: a PUT that fell through
// to the list handler would be a silent no-op rather than an error.
func registerHttpFileRoutes(mux *http.ServeMux, store *httpFileStore) {
	mux.HandleFunc("GET /api/http-files", httpFilesListHandler(store))
	mux.HandleFunc("POST /api/http-files/open-folder", httpFilesOpenFolderHandler(store))
	mux.HandleFunc("PUT /api/http-files/{name}", httpFileSaveHandler(store))
	mux.HandleFunc("DELETE /api/http-files/{name}", httpFileDeleteHandler(store))
	mux.HandleFunc("POST /api/http-files/{name}/rename", httpFileRenameHandler(store))
}

func httpFilesListHandler(store *httpFileStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		files, err := store.List()
		if err != nil {
			log.Printf("Error listing .http files: %v", err)
			http.Error(w, "could not list .http files", http.StatusInternalServerError)
			return
		}
		writeHttpFileJson(w, shared.HttpFilesDto{Folder: store.Folder(), Files: files})
	}
}

// httpFileSaveHandler takes the file as a plain text body rather than wrapped
// in JSON: the resource is the file, so `curl -T` or an editor's REST client
// can write one without knowing an envelope.
func httpFileSaveHandler(store *httpFileStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, maxHttpFileBytes+1))
		if err != nil {
			http.Error(w, "could not read the request body", http.StatusBadRequest)
			return
		}
		if len(body) > maxHttpFileBytes {
			http.Error(w, "the .http file is too large", http.StatusRequestEntityTooLarge)
			return
		}

		dto, err := store.Save(r.PathValue("name"), string(body))
		if err != nil {
			writeHttpFileError(w, err, "could not save the .http file")
			return
		}
		writeHttpFileJson(w, dto)
	}
}

func httpFileRenameHandler(store *httpFileStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload shared.HttpFileRenameDto
		if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&payload); err != nil {
			http.Error(w, "could not read the new name", http.StatusBadRequest)
			return
		}

		dto, err := store.Rename(r.PathValue("name"), payload.Name)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				http.Error(w, "no such .http file", http.StatusNotFound)
				return
			}
			writeHttpFileError(w, err, "could not rename the .http file")
			return
		}
		writeHttpFileJson(w, dto)
	}
}

func httpFileDeleteHandler(store *httpFileStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := store.Delete(r.PathValue("name")); err != nil {
			writeHttpFileError(w, err, "could not delete the .http file")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func httpFilesOpenFolderHandler(store *httpFileStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := store.OpenFolder(); err != nil {
			log.Printf("Error opening the .http folder: %v", err)
			http.Error(w, "could not open the folder", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// writeHttpFileError maps the store's failures onto status codes: a name the
// store refuses is the client's mistake, anything else is ours.
func writeHttpFileError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, errInvalidHttpFileName):
		http.Error(w, "invalid .http file name", http.StatusBadRequest)
	case errors.Is(err, errHttpFileExists):
		http.Error(w, "a .http file with that name already exists", http.StatusConflict)
	default:
		log.Printf("Error on .http file: %v", err)
		http.Error(w, fallback, http.StatusInternalServerError)
	}
}

func writeHttpFileJson(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("Error marshaling .http file response: %v", err)
	}
}
