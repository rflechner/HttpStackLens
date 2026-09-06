//go:build js && wasm

package dom

import "syscall/js"

// Clipboard copies text, silently ignoring a denied permission.
func Clipboard(text string) {
	nav := js.Global().Get("navigator")
	if cb := nav.Get("clipboard"); cb.Truthy() {
		cb.Call("writeText", text)
	}
}

// Confirm asks the user a yes/no question. It blocks the page, so keep it for
// what deserves it — deleting a file on disk, not a change that can be undone.
func Confirm(message string) bool {
	return js.Global().Call("confirm", message).Truthy()
}
