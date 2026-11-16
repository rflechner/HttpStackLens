package main

import (
	"errors"
	"strings"
	"testing"
)

func TestDesktopStartupPersistsAndReadsRegistration(t *testing.T) {
	command := ""
	var saved, changed bool
	c := &desktopStartupController{
		supported: true, background: true,
		read:    func() (string, error) { return command, nil },
		write:   func(value string) error { command = value; return nil },
		command: func() (string, error) { return `"C:\My App\app.exe" --background`, nil },
		persist: func(value bool) error { saved = value; return nil },
		changed: func(value bool) { changed = value },
	}
	state, err := c.SetEnabled(true)
	if err != nil || !state.StartAtLogin || !state.Registered || !state.StartedInBackground || !saved || !changed {
		t.Fatalf("enable = %+v, %v; saved=%v changed=%v", state, err, saved, changed)
	}
	// An external registry change must appear separately from the saved choice.
	command = ""
	state, err = c.Get()
	if err != nil || !state.StartAtLogin || state.Registered {
		t.Fatalf("external removal = %+v, %v", state, err)
	}
	state, err = c.SetEnabled(false)
	if err != nil || state.StartAtLogin || state.Registered || saved || changed {
		t.Fatalf("disable = %+v, %v", state, err)
	}
}

func TestDesktopStartupRestoresExactCommandWhenPersistenceFails(t *testing.T) {
	previous := `"C:\Old App\app.exe" --background`
	command := previous
	c := &desktopStartupController{
		supported: true,
		read:      func() (string, error) { return command, nil },
		write:     func(value string) error { command = value; return nil },
		command:   func() (string, error) { return "new-command", nil },
		persist:   func(bool) error { return errors.New("disk full") },
	}
	if _, err := c.SetEnabled(true); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("error = %v", err)
	}
	if command != previous || c.preference != nil {
		t.Fatalf("rollback: command=%q preference=%v", command, c.preference)
	}
}

func TestDesktopStartupRegistrationFailureDoesNotPersist(t *testing.T) {
	c := &desktopStartupController{
		supported: true,
		read:      func() (string, error) { return "", nil },
		write:     func(string) error { return errors.New("access denied") },
		command:   func() (string, error) { return "command", nil },
		persist:   func(bool) error { t.Fatal("must not persist after registry failure"); return nil },
	}
	if _, err := c.SetEnabled(true); err == nil {
		t.Fatal("expected error")
	}
}

func TestDesktopStartupInitialPreferenceAndLegacyRegistration(t *testing.T) {
	for _, value := range []bool{false, true} {
		command := "legacy-command"
		c := &desktopStartupController{
			supported: true,
			read:      func() (string, error) { return command, nil },
			write:     func(value string) error { command = value; return nil },
			command:   func() (string, error) { return "current-command", nil },
		}
		if err := c.ApplyInitial(); err != nil || command != "legacy-command" {
			t.Fatalf("legacy registration changed: %q, %v", command, err)
		}
		state, err := c.Get()
		if err != nil || !state.StartAtLogin || !state.Registered {
			t.Fatalf("legacy state = %+v, %v", state, err)
		}
		c.preference = &value
		if err := c.ApplyInitial(); err != nil {
			t.Fatal(err)
		}
		if value && command != "current-command" || !value && command != "" {
			t.Fatalf("initial %v: command=%q", value, command)
		}
	}
}

func TestDesktopStartupUnsupported(t *testing.T) {
	c := &desktopStartupController{}
	state, err := c.Get()
	if err != nil || state.Supported {
		t.Fatalf("unsupported state = %+v, %v", state, err)
	}
	if _, err := c.SetEnabled(true); err == nil {
		t.Fatal("expected unsupported error")
	}
}
