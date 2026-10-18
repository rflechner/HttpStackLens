package composer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHttpFileNameAddsExtension(t *testing.T) {
	name, err := HttpFileName("  github  ")
	if err != nil {
		t.Fatalf("expected a valid name, got %v", err)
	}
	if name != "github.http" {
		t.Fatalf("expected github.http, got %q", name)
	}
}

func TestHttpFileNameKeepsExistingExtension(t *testing.T) {
	name, err := HttpFileName("corp-auth.http")
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
		if got, err := HttpFileName(name); err == nil {
			t.Errorf("expected %q to be refused, got %q", name, got)
		}
	}
}

func TestStoreSaveListRenameDelete(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "http_files")
	store := NewHttpFileStore(folder)

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
	store := NewHttpFileStore(t.TempDir())
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
	store := NewHttpFileStore(folder)
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
