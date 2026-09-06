//go:build js && wasm

package main

import (
	"syscall/js"

	"httpStackLens/webui/wasm/components/composer"
	"httpStackLens/webui/wasm/components/modeswitch"
	"httpStackLens/webui/wasm/dom"
)

const modeKey = "hsl-mode"

// composerMounted is set on the first switch to the composer: the traffic view
// is what the app opens on, so its component tree is built only if asked for.
var composerMounted bool

// initModeSwitch mounts the title-bar switch and applies the mode the window
// was left in. The switch itself knows nothing of the views — it hands the mode
// back here, where the page regions are shown and hidden.
func initModeSwitch() {
	host := js.Global().Get("document").Call("getElementById", "mode-switch")
	if !host.Truthy() {
		return
	}
	mode := dom.LocalGet(modeKey)
	if mode != modeswitch.Composer {
		mode = modeswitch.Traffic
	}
	applyMode(mode)
	dom.Mount(host, modeswitch.New(mode, applyMode))
}

func applyMode(mode string) {
	dom.LocalSet(modeKey, mode)
	capture := mode != modeswitch.Composer
	showRegion("capture-toolbar", capture)
	showRegion("capture-view", capture)
	showRegion("compose-view", !capture)

	if capture || composerMounted {
		return
	}
	composerMounted = true
	view := js.Global().Get("document").Call("getElementById", "compose-view")
	dom.Mount(view, composer.New())
}

// showRegion toggles a top-level region. Every one of them is a flex row, so
// the visible value is always flex.
func showRegion(id string, visible bool) {
	el := js.Global().Get("document").Call("getElementById", id)
	if !el.Truthy() {
		return
	}
	display := "none"
	if visible {
		display = "flex"
	}
	el.Get("style").Set("display", display)
}
