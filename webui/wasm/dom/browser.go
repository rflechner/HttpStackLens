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

// Download offers content to the user as a file.
func Download(name, mime, content string) {
	doc := js.Global().Get("document")
	blob := js.Global().Get("Blob").New(
		[]any{content},
		map[string]any{"type": mime},
	)
	url := js.Global().Get("URL").Call("createObjectURL", blob)
	a := doc.Call("createElement", "a")
	a.Set("href", url)
	a.Set("download", name)
	a.Call("click")
	js.Global().Get("URL").Call("revokeObjectURL", url)
}
