package helpers

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// OpenFolderInFileManager shows a folder in the desktop file manager of the
// machine running HttpStackLens — Explorer on Windows, Finder on macOS, the
// XDG default elsewhere.
//
// The command is started and not waited on: Explorer detaches and reports a
// non-zero exit code even when it opened the window, so its status says
// nothing useful. What we can check is the folder, which is created when it is
// missing rather than handing the file manager a path it would refuse.
func OpenFolderInFileManager(folder string) error {
	if folder == "" {
		return fmt.Errorf("no folder to open")
	}
	path := filepath.Clean(folder)
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("could not create %s: %w", path, err)
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not open %s: %w", path, err)
	}
	// Nothing reads the outcome, so reap the child instead of leaving a zombie.
	go func() { _ = cmd.Wait() }()
	return nil
}
