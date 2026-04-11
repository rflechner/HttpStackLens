//go:build js && wasm

package main

import (
	"fmt"
	"syscall/js"
)

type StateModel struct {
	RequestCount int
	Lines        []string
}

func consoleLog(message string) {
	js.Global().Get("console").Call("log", message)
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
		event := args[0]
		data := event.Get("data").String()
		if data == "" {
			consoleLog("Empty event received")
			return nil
		}
		consoleLog("Received event data: " + data)
		m.Lines = append(m.Lines, data)
		m.RequestCount++
		m.render()
		return nil
	}))
}

func (m *StateModel) render() {
	doc := js.Global().Get("document")

	html := fmt.Sprintf("request count: <strong>%d</strong>", m.RequestCount)
	html += "<table>"

	for _, line := range m.Lines {
		html += "<tr><td>" + line + "</td></tr>"
	}

	html += "</table>"

	doc.Call("getElementById", "summary").Set("innerHTML", html)
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
