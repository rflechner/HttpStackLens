//go:build js && wasm

// Package modeswitch is the title-bar segmented control that flips the window
// between the traffic capture view and the .http composer.
//
// It owns nothing but itself: switching views is the host's business, so the
// component only reports the new mode through OnChange — the equivalent of an
// EventCallback<string>.
package modeswitch

import (
	_ "embed"

	"httpStackLens/webui/wasm/dom"
)

//go:embed modeswitch.html
var switchHTML string

//go:embed modeswitch.css
var switchCSS string

// The two modes the switch offers.
const (
	Traffic  = "capture"
	Composer = "compose"
)

// Switch is the segmented control. Mount it on the element carrying the
// mode-seg class — that element belongs to the page, its content to the switch.
type Switch struct {
	dom.Base

	Mode     string
	OnChange func(mode string)
}

// New returns an unmounted switch showing mode as the current one. onChange
// runs on every actual change, never on the initial mode.
func New(mode string, onChange func(mode string)) *Switch {
	if mode != Composer {
		mode = Traffic
	}
	return &Switch{Mode: mode, OnChange: onChange}
}

func (s *Switch) Template() string { return switchHTML }

func (s *Switch) Styles() string { return switchCSS }

// IsOn reports whether a mode is the current one, for the template.
func (s *Switch) IsOn(mode string) bool { return s.Mode == mode }

// Select is the click handler: data-arg carries the mode.
func (s *Switch) Select(e dom.Event) {
	mode := e.Arg()
	if mode == "" || mode == s.Mode {
		return
	}
	s.Mode = mode
	s.StateHasChanged()
	if s.OnChange != nil {
		s.OnChange(mode)
	}
}
