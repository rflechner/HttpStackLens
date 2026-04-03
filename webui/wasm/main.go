//go:build js && wasm

package main

import (
	"fmt"
	"syscall/js"
)

type StateModel struct {
	RequestCount int
}

func (m *StateModel) connectSSE() {
	es := js.Global().Get("EventSource").New("/events")

	// Connection established
	es.Set("onopen", js.FuncOf(func(this js.Value, args []js.Value) any {
		js.Global().Get("document").
			Call("getElementById", "sse-status").
			Set("innerHTML", "🟢 Connected to server stream")
		return nil
	}))

	// Connection closed
	es.Set("onerror", js.FuncOf(func(this js.Value, args []js.Value) any {
		js.Global().Get("document").
			Call("getElementById", "sse-status").
			Set("innerHTML", "🔴 Disconnected from server stream")
		return nil
	}))

	es.Call("addEventListener", "request_occurred", js.FuncOf(func(this js.Value, args []js.Value) any {
		m.RequestCount++
		m.render()
		return nil
	}))
}

func (m *StateModel) render() {
	doc := js.Global().Get("document")

	doc.Call("getElementById", "summary").Set("innerHTML",
		fmt.Sprintf("request count: <strong>%d</strong>", m.RequestCount),
	)
}

func main() {

	js.Global().Call("alert", "Hello, World!")

	model := &StateModel{
		RequestCount: 0,
	}
	model.render()
	model.connectSSE()

	// block forever
	select {}
}
