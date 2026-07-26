package helpers

import (
	"log"
	"os"
	"path/filepath"
)

func ResolveRelativePath(relativeOrAbsolutePath string) string {
	// An empty path means "not configured"; keep it empty so callers can still
	// detect it, instead of resolving to the executable's directory.
	if relativeOrAbsolutePath == "" {
		return ""
	}

	if filepath.IsAbs(relativeOrAbsolutePath) {
		return relativeOrAbsolutePath
	}

	exePath, err := os.Executable()
	if err != nil {
		log.Printf("Failed to resolve executable path: %v\n", err)
		return relativeOrAbsolutePath
	}

	// os.Executable can return a symlink on some platforms; follow it so the
	// config is resolved next to the real binary, not the link.
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = resolved
	}

	return filepath.Join(filepath.Dir(exePath), relativeOrAbsolutePath)
}
