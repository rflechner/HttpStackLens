//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

const startupRegistryPath = `Software\Microsoft\Windows\CurrentVersion\Run`
const startupRegistryName = "HttpStackLens"
const startupSupported = true

// Only the current user's Run entry is changed. Windows starts the same binary
// in that user's session, retaining SSPI's current-user credentials.
func startupCommand(executable string) (string, error) {
	if !filepath.IsAbs(executable) || strings.ContainsAny(executable, "\x00\r\n\"") {
		return "", fmt.Errorf("invalid executable path for startup")
	}
	return syscall.EscapeArg(executable) + " --background", nil
}

func readStartupRegistration() (string, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, startupRegistryPath, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer key.Close()
	command, _, err := key.GetStringValue(startupRegistryName)
	if errors.Is(err, registry.ErrNotExist) {
		return "", nil
	}
	return command, err
}

func writeStartupRegistration(command string) error {
	if command == "" {
		key, err := registry.OpenKey(registry.CURRENT_USER, startupRegistryPath, registry.SET_VALUE)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		defer key.Close()
		err = key.DeleteValue(startupRegistryName)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, startupRegistryPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetStringValue(startupRegistryName, command)
}

func currentStartupCommand() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	return startupCommand(executable)
}
