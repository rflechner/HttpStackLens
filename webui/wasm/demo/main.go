//go:build js && wasm

// Command demo mounts the composer on its own, so the component can be
// exercised without booting the whole UI:
//
//	GOOS=js GOARCH=wasm go build -o demo.wasm ./webui/wasm/demo
package main

import (
	"syscall/js"

	"httpStackLens/webui/wasm/components/composer"
	"httpStackLens/webui/wasm/dom"
)

func main() {
	doc := js.Global().Get("document")
	dom.Mount(doc.Call("getElementById", "app"), composer.New())
	if loader := doc.Call("getElementById", "loader"); loader.Truthy() {
		loader.Set("hidden", true)
	}
	select {} // keep the Go runtime alive for the callbacks
}
