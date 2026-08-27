//go:build dev

package webui

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

// In dev mode, we use the local filesystem.
// We can modify HTML/CSS files directly without rebuilding.
func getFS() fs.FS {
	// Resolve from this source file first so IDE run configurations do not
	// depend on a particular working directory. The relative fallbacks keep
	// trimmed-path builds and package tests convenient as well.
	if _, sourceFile, _, ok := runtime.Caller(0); ok {
		if root := filepath.Dir(sourceFile); hasDevIndex(root) {
			return os.DirFS(root)
		}
	}
	for _, root := range []string{"webui", "."} {
		if hasDevIndex(root) {
			return os.DirFS(root)
		}
	}
	return os.DirFS("webui")
}

func hasDevIndex(root string) bool {
	_, err := os.Stat(filepath.Join(root, "wwwroot", "index.html"))
	return err == nil
}
