//go:build !(js && wasm)

package composer

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"httpStackLens/helpers"
	"httpStackLens/webui/wasm/shared"
)

// httpFileExt is the only extension the composer's collection holds. Anything
// else in the folder is left alone: the folder is a normal one a developer may
// keep notes or a README in.
const httpFileExt = ".http"

// MaxHttpFileBytes caps what a single save may write. A .http file is hand
// written text; a megabyte of it is already far past anything reasonable, and
// the limit keeps a runaway client from filling the disk.
const MaxHttpFileBytes = 1 << 20

var (
	ErrInvalidHttpFileName = errors.New("invalid .http file name")
	ErrHttpFileExists      = errors.New(".http file already exists")
)

// HttpFileStore is the composer's collection of `.http` files on disk. The
// folder comes from config.yaml (`http_files.folder`), and every path handed to
// the filesystem is rebuilt from a validated base name, so a name coming off
// the wire can never point outside it.
//
// The Web UI and the MCP server share one store. Its lock makes each operation
// whole — an Edit reads and writes back without another writer slipping in
// between — but it does not merge: the Web UI still overwrites a file with the
// copy it holds when it saves.
type HttpFileStore struct {
	folder string
	mu     sync.Mutex
}

func NewHttpFileStore(folder string) *HttpFileStore {
	return &HttpFileStore{folder: folder}
}

// Folder is the location shown in the Web UI and opened in the file manager.
func (s *HttpFileStore) Folder() string { return s.folder }

// List reads every .http file in the folder. A folder that does not exist yet
// is not an error: it simply holds no file, which is what the composer shows
// before the first one is created.
func (s *HttpFileStore) List() ([]shared.HttpFileDto, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

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
		if info.Size() > MaxHttpFileBytes {
			log.Printf("Skipping %s: larger than the %d byte .http limit", entry.Name(), MaxHttpFileBytes)
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

// Read loads one file. A file that is not there is reported with
// os.ErrNotExist so that callers can tell it from a failure.
func (s *HttpFileStore) Read(name string) (shared.HttpFileDto, error) {
	safe, err := HttpFileName(name)
	if err != nil {
		return shared.HttpFileDto{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(safe)
}

// Save writes a file, creating the folder on the way. It is a plain overwrite:
// the Web UI composer sends the whole file every time.
func (s *HttpFileStore) Save(name, content string) (shared.HttpFileDto, error) {
	safe, err := HttpFileName(name)
	if err != nil {
		return shared.HttpFileDto{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.write(safe, content)
}

// Edit reads a file, hands its content to change and writes back what change
// returns, with no other write of the store in between. A missing file reaches
// change as ("", false), so the caller decides whether it may be created.
// Returning an error from change leaves the file untouched.
func (s *HttpFileStore) Edit(name string, change func(current shared.HttpFileDto, exists bool) (string, error)) (shared.HttpFileDto, error) {
	safe, err := HttpFileName(name)
	if err != nil {
		return shared.HttpFileDto{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := s.read(safe)
	exists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return shared.HttpFileDto{}, err
	}
	content, err := change(current, exists)
	if err != nil {
		return shared.HttpFileDto{}, err
	}
	return s.write(safe, content)
}

// Rename moves a file inside the folder. An existing target is refused rather
// than overwritten: renaming onto another collection would silently destroy it.
func (s *HttpFileStore) Rename(oldName, newName string) (shared.HttpFileDto, error) {
	from, err := HttpFileName(oldName)
	if err != nil {
		return shared.HttpFileDto{}, err
	}
	to, err := HttpFileName(newName)
	if err != nil {
		return shared.HttpFileDto{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if from != to {
		// A case-only rename on Windows and macOS would read as "the target
		// exists", because the filesystem matches the two names. os.Rename
		// handles that one itself, so only a genuinely different name is
		// checked.
		if !strings.EqualFold(from, to) {
			if _, err := os.Stat(filepath.Join(s.folder, to)); err == nil {
				return shared.HttpFileDto{}, ErrHttpFileExists
			}
		}
		if err := os.Rename(filepath.Join(s.folder, from), filepath.Join(s.folder, to)); err != nil {
			return shared.HttpFileDto{}, err
		}
	}
	return s.read(to)
}

// Delete removes a file. Deleting one that is already gone succeeds: the
// composer only asks to close what it believes is there, and the outcome the
// caller wants — no such file — is the one it gets.
func (s *HttpFileStore) Delete(name string) error {
	safe, err := HttpFileName(name)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(filepath.Join(s.folder, safe)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// OpenFolder shows the folder in the file manager of the machine running
// HttpStackLens.
func (s *HttpFileStore) OpenFolder() error {
	return helpers.OpenFolderInFileManager(s.folder)
}

// read and write expect the lock to be held and the name to be validated.
func (s *HttpFileStore) read(safe string) (shared.HttpFileDto, error) {
	path := filepath.Join(s.folder, safe)
	info, err := os.Stat(path)
	if err != nil {
		return shared.HttpFileDto{}, err
	}
	if info.IsDir() {
		return shared.HttpFileDto{}, fmt.Errorf("%s: %w", safe, os.ErrNotExist)
	}
	if info.Size() > MaxHttpFileBytes {
		return shared.HttpFileDto{}, fmt.Errorf("%s is larger than the %d byte limit", safe, MaxHttpFileBytes)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return shared.HttpFileDto{}, err
	}
	return shared.HttpFileDto{
		Name:       safe,
		Content:    string(content),
		ModifiedAt: info.ModTime().UTC().Format(time.RFC3339Nano),
	}, nil
}

func (s *HttpFileStore) write(safe, content string) (shared.HttpFileDto, error) {
	if len(content) > MaxHttpFileBytes {
		return shared.HttpFileDto{}, fmt.Errorf("%s is larger than the %d byte limit", safe, MaxHttpFileBytes)
	}
	if err := os.MkdirAll(s.folder, 0o755); err != nil {
		return shared.HttpFileDto{}, err
	}
	if err := os.WriteFile(filepath.Join(s.folder, safe), []byte(content), 0o644); err != nil {
		return shared.HttpFileDto{}, err
	}
	return s.read(safe)
}

// reservedHttpFileChars is Windows' reserved set, applied on every platform so
// a collection stays portable between the machines one developer works on.
const reservedHttpFileChars = `<>:"|?*`

// HttpFileName turns what a client sent into a base name inside the folder, or
// refuses it. The extension is added when it is missing so that a name typed in
// the Web UI, or by hand against the API, does not have to carry it.
func HttpFileName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", ErrInvalidHttpFileName
	}
	if !strings.EqualFold(filepath.Ext(trimmed), httpFileExt) {
		trimmed += httpFileExt
	}
	// Base name only: a separator, a drive letter or a `..` segment would point
	// somewhere other than the collection.
	if trimmed != filepath.Base(trimmed) || strings.ContainsAny(trimmed, `/\`) {
		return "", ErrInvalidHttpFileName
	}
	if strings.HasPrefix(trimmed, ".") || len(trimmed) > 128 {
		return "", ErrInvalidHttpFileName
	}
	for _, r := range trimmed {
		if r < 0x20 || strings.ContainsRune(reservedHttpFileChars, r) {
			return "", ErrInvalidHttpFileName
		}
	}
	return trimmed, nil
}
