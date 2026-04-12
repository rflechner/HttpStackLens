//go:build !dev

package webui

import (
	"embed"
	"io/fs"
)

//go:embed wwwroot
var embeddedFiles embed.FS

func getFS() fs.FS {
	return embeddedFiles
}
