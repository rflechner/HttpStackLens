package webui

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"

	"httpStackLens/composer"
	"httpStackLens/webui/wasm/shared"
)

// The composer's collection of `.http` files lives in composer.HttpFileStore,
// shared with the MCP server; this file only puts it on the wire.

// ── handlers ───────────────────────────────────────────────────────────────

// registerHttpFileRoutes wires the collection onto a mux. The routes spell the
// method out because the last segment is a file name: a PUT that fell through
// to the list handler would be a silent no-op rather than an error.
func registerHttpFileRoutes(mux *http.ServeMux, store *composer.HttpFileStore) {
	mux.HandleFunc("GET /api/http-files", httpFilesListHandler(store))
	mux.HandleFunc("POST /api/http-files/open-folder", httpFilesOpenFolderHandler(store))
	mux.HandleFunc("PUT /api/http-files/{name}", httpFileSaveHandler(store))
	mux.HandleFunc("DELETE /api/http-files/{name}", httpFileDeleteHandler(store))
	mux.HandleFunc("POST /api/http-files/{name}/rename", httpFileRenameHandler(store))
}

func httpFilesListHandler(store *composer.HttpFileStore) http.HandlerFunc {
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
func httpFileSaveHandler(store *composer.HttpFileStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, composer.MaxHttpFileBytes+1))
		if err != nil {
			http.Error(w, "could not read the request body", http.StatusBadRequest)
			return
		}
		if len(body) > composer.MaxHttpFileBytes {
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

func httpFileRenameHandler(store *composer.HttpFileStore) http.HandlerFunc {
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

func httpFileDeleteHandler(store *composer.HttpFileStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := store.Delete(r.PathValue("name")); err != nil {
			writeHttpFileError(w, err, "could not delete the .http file")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func httpFilesOpenFolderHandler(store *composer.HttpFileStore) http.HandlerFunc {
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
	case errors.Is(err, composer.ErrInvalidHttpFileName):
		http.Error(w, "invalid .http file name", http.StatusBadRequest)
	case errors.Is(err, composer.ErrHttpFileExists):
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
