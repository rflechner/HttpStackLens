//go:build dev

package webui

import (
	"io/fs"
	"os"
)

// In dev mode, we use the local filesystem.
// We can modify HTML/CSS files directly without rebuilding.
func getFS() fs.FS {
	return os.DirFS(".")
}
