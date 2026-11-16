//go:build !windows

package main

// Other platforms keep the existing desktop lifecycle until a native tray is
// implemented for them. In particular, closing their window still exits.
func startDesktopTray(show, quit func(), startup *desktopStartupController) (func(), error) {
	return nil, nil
}

const startupSupported = false

func readStartupRegistration() (string, error)      { return "", nil }
func writeStartupRegistration(command string) error { return nil }
func currentStartupCommand() (string, error)        { return "", nil }

func acquireDesktopInstance(background bool) (func(), bool, error) {
	return func() {}, true, nil
}
