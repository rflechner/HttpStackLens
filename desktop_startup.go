package main

import (
	"fmt"
	"sync"

	"httpStackLens/webui/wasm/shared"
)

// The tray and HTTP settings use the same controller. Read back Windows' Run
// entry on each request, rather than reporting only the saved YAML preference.
type desktopStartupController struct {
	mu                    sync.Mutex
	supported, background bool
	preference            *bool
	read                  func() (string, error)
	write                 func(string) error
	command               func() (string, error)
	persist               func(bool) error
	changed               func(bool)
}

func (c *desktopStartupController) Get() (shared.DesktopSettingsDto, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.get()
}

func (c *desktopStartupController) get() (shared.DesktopSettingsDto, error) {
	state := shared.DesktopSettingsDto{Supported: c.supported, StartedInBackground: c.background}
	if !c.supported {
		return state, nil
	}
	command, err := c.read()
	if err != nil {
		return state, err
	}
	state.Registered = command != ""
	state.StartAtLogin = state.Registered
	if c.preference != nil {
		state.StartAtLogin = *c.preference
	}
	return state, nil
}

func (c *desktopStartupController) SetEnabled(enabled bool) (shared.DesktopSettingsDto, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.supported {
		return shared.DesktopSettingsDto{}, fmt.Errorf("start at sign-in is only supported on Windows")
	}
	previous, err := c.read()
	if err != nil {
		return shared.DesktopSettingsDto{}, err
	}
	next := ""
	if enabled {
		next, err = c.command()
		if err != nil {
			return shared.DesktopSettingsDto{}, err
		}
	}
	if err := c.write(next); err != nil {
		return shared.DesktopSettingsDto{}, err
	}
	if err := c.persist(enabled); err != nil {
		if rollbackErr := c.write(previous); rollbackErr != nil {
			return shared.DesktopSettingsDto{}, fmt.Errorf("save desktop settings: %w; restore startup entry: %v", err, rollbackErr)
		}
		return shared.DesktopSettingsDto{}, fmt.Errorf("save desktop settings: %w", err)
	}
	c.preference = &enabled
	if c.changed != nil {
		c.changed(enabled)
	}
	return c.get()
}

func (c *desktopStartupController) ApplyInitial() error {
	if !c.supported || c.preference == nil {
		return nil
	}
	next := ""
	if *c.preference {
		var err error
		next, err = c.command()
		if err != nil {
			return err
		}
	}
	return c.write(next)
}
