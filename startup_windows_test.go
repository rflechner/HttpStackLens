//go:build windows

package main

import (
	"testing"
	"unsafe"
)

func TestStartupCommandQuotesExecutable(t *testing.T) {
	for _, tc := range []struct{ executable, command string }{
		{`C:\Program Files\HttpStackLens\HttpStackLens.exe`, `"C:\Program Files\HttpStackLens\HttpStackLens.exe" --background`},
		{`C:\Tools\HttpStackLens.exe`, `C:\Tools\HttpStackLens.exe --background`},
		{`\\server\tools\HttpStackLens.exe`, `\\server\tools\HttpStackLens.exe --background`},
	} {
		command, err := startupCommand(tc.executable)
		if err != nil || command != tc.command {
			t.Errorf("startupCommand(%q) = %q, %v; want %q", tc.executable, command, err, tc.command)
		}
	}
}

func TestStartupCommandRejectsInvalidPaths(t *testing.T) {
	for _, path := range []string{"", "HttpStackLens.exe", `C:HttpStackLens.exe`, "C:\\bad\x00.exe", "C:\\bad\r\n.exe", `C:\bad"name.exe`} {
		if _, err := startupCommand(path); err == nil {
			t.Errorf("accepted invalid startup path %q", path)
		}
	}
}

// These structs cross the Win32 ABI through unsafe.Pointer. A wrong field or
// padding would otherwise compile successfully but fail only at runtime.
func TestTrayWindowsABILayout(t *testing.T) {
	wantIcon, wantClass, wantMessage := uintptr(976), uintptr(80), uintptr(48)
	if unsafe.Sizeof(uintptr(0)) == 4 {
		wantIcon, wantClass, wantMessage = 956, 48, 32
	}
	for _, tc := range []struct {
		name      string
		got, want uintptr
	}{
		{"NOTIFYICONDATAW", unsafe.Sizeof(trayNotifyIconData{}), wantIcon},
		{"WNDCLASSEXW", unsafe.Sizeof(trayWindowClassEx{}), wantClass},
		{"MSG", unsafe.Sizeof(trayMessage{}), wantMessage},
	} {
		if tc.got != tc.want {
			t.Errorf("%s size = %d, want %d", tc.name, tc.got, tc.want)
		}
	}
}
