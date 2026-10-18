//go:build js && wasm

package main

import (
	"syscall/js"

	"httpStackLens/webui/wasm/components/composer"
	"httpStackLens/webui/wasm/components/mcppanel"
	"httpStackLens/webui/wasm/components/modeswitch"
	"httpStackLens/webui/wasm/dom"
)

const modeKey = "hsl-mode"

// composerMounted is set on the first switch to the composer: the traffic view
// is what the app opens on, so its component tree is built only if asked for.
var composerMounted bool

// composerView is the mounted composer, nil until the first switch to it. The
// SSE stream hands it the .http files an MCP client writes.
var composerView *composer.Composer

// mcpView is the mounted MCP tab, nil until the first switch to it. The SSE
// stream hands it the listener state and the calls it traces; until then it
// has nothing to keep up to date, since mounting it reads both.
var mcpView *mcppanel.Panel

// initModeSwitch mounts the title-bar switch and applies the mode the window
// was left in. The switch itself knows nothing of the views — it hands the mode
// back here, where the page regions are shown and hidden.
func initModeSwitch() {
	host := js.Global().Get("document").Call("getElementById", "mode-switch")
	if !host.Truthy() {
		return
	}
	mode := dom.LocalGet(modeKey)
	if !modeswitch.Known(mode) {
		mode = modeswitch.Traffic
	}
	applyMode(mode)
	dom.Mount(host, modeswitch.New(mode, applyMode))
}

func applyMode(mode string) {
	dom.LocalSet(modeKey, mode)
	capture := mode == modeswitch.Traffic
	showRegion("capture-toolbar", capture)
	showRegion("capture-view", capture)
	showRegion("compose-view", mode == modeswitch.Composer)
	showRegion("mcp-view", mode == modeswitch.MCP)

	switch {
	case mode == modeswitch.Composer && !composerMounted:
		composerMounted = true
		view := js.Global().Get("document").Call("getElementById", "compose-view")
		composerView = composer.New()
		dom.Mount(view, composerView)
	case mode == modeswitch.MCP && mcpView == nil:
		view := js.Global().Get("document").Call("getElementById", "mcp-view")
		mcpView = mcppanel.New()
		dom.Mount(view, mcpView)
	}
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
