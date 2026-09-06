//go:build js && wasm

package composer

import (
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"httpStackLens/webui/wasm/dom"
	"httpStackLens/webui/wasm/shared"
)

// The composer's collection lives on disk, in the folder config.yaml names
// under `http_files.folder`. The backend owns it; these calls are the whole of
// what the UI knows about where the files are.
const (
	httpFilesPath  = "/api/http-files"
	openFolderPath = "/api/http-files/open-folder"
)

// legacyStoreKey held the collection before it moved to disk. It is read once,
// to carry an existing collection over, and then dropped.
const legacyStoreKey = "hsl-http-files"

// fetchHttpFiles reads the whole collection: the folder it came from, and every
// file with its text. The files are small and the sidebar lists the requests
// inside each of them, so one round trip beats one per file.
func fetchHttpFiles() (shared.HttpFilesDto, error) {
	res, err := dom.Fetch("GET", httpFilesPath, nil, "")
	if err != nil {
		return shared.HttpFilesDto{}, errors.New("could not reach HttpStackLens: " + err.Error())
	}
	if res.Status != 200 {
		return shared.HttpFilesDto{}, backendError(res)
	}
	var dto shared.HttpFilesDto
	if err := json.Unmarshal([]byte(res.Body), &dto); err != nil {
		return shared.HttpFilesDto{}, errors.New("unreadable answer from HttpStackLens: " + err.Error())
	}
	return dto, nil
}

// putHttpFile writes one file back, creating it when it is new. The whole file
// goes over as plain text: the composer is its only writer, and a partial
// update would need the two sides to agree on a diff neither of them keeps.
func putHttpFile(name, content string) error {
	res, err := dom.Fetch("PUT", httpFilesPath+"/"+url.PathEscape(name),
		map[string]string{"Content-Type": "text/plain; charset=utf-8"}, content)
	if err != nil {
		return errors.New("could not reach HttpStackLens: " + err.Error())
	}
	if res.Status != 200 {
		return backendError(res)
	}
	return nil
}

func renameHttpFile(oldName, newName string) error {
	payload, err := json.Marshal(shared.HttpFileRenameDto{Name: newName})
	if err != nil {
		return err
	}
	res, err := dom.Fetch("POST", httpFilesPath+"/"+url.PathEscape(oldName)+"/rename",
		map[string]string{"Content-Type": "application/json"}, string(payload))
	if err != nil {
		return errors.New("could not reach HttpStackLens: " + err.Error())
	}
	if res.Status != 200 {
		return backendError(res)
	}
	return nil
}

func deleteHttpFile(name string) error {
	res, err := dom.Fetch("DELETE", httpFilesPath+"/"+url.PathEscape(name), nil, "")
	if err != nil {
		return errors.New("could not reach HttpStackLens: " + err.Error())
	}
	if res.Status != 204 {
		return backendError(res)
	}
	return nil
}

// openHttpFolder asks the backend to show the folder in the file manager of the
// machine it runs on — which is the developer's own machine: HttpStackLens is a
// local tool, and the Web UI is served from localhost.
func openHttpFolder() error {
	res, err := dom.Fetch("POST", openFolderPath, nil, "")
	if err != nil {
		return errors.New("could not reach HttpStackLens: " + err.Error())
	}
	if res.Status != 204 {
		return backendError(res)
	}
	return nil
}

// backendError turns a failed response into the message the UI shows. The API
// answers errors in plain text, so the body is the message.
func backendError(res dom.HTTPResponse) error {
	message := strings.TrimSpace(res.Body)
	if message == "" {
		message = res.StatusText
	}
	if message == "" {
		message = "HttpStackLens answered " + strconv.Itoa(res.Status)
	}
	return errors.New(message)
}

// migrateLegacyFiles carries a collection kept in the browser over to the
// folder, once. Before the files were stored on disk they lived in
// localStorage, and a user upgrading would otherwise open the composer to an
// empty sidebar with their requests nowhere in sight.
func migrateLegacyFiles() []shared.HttpFileDto {
	raw := dom.LocalGet(legacyStoreKey)
	if raw == "" {
		return nil
	}
	defer dom.LocalRemove(legacyStoreKey)

	var files []*File
	if err := json.Unmarshal([]byte(raw), &files); err != nil {
		return nil
	}

	migrated := make([]shared.HttpFileDto, 0, len(files))
	for _, f := range files {
		name, err := FileName(f.Name)
		if err != nil {
			continue
		}
		content := ToHTTP(f)
		if err := putHttpFile(name, content); err != nil {
			continue
		}
		migrated = append(migrated, shared.HttpFileDto{Name: name, Content: content})
	}
	return migrated
}
