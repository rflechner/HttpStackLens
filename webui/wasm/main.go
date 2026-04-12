//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
	"httpStackLens/webui/wasm/shared"
	"syscall/js"
)

type StateModel struct {
	RequestCount int
	Lines        []shared.RequestEventDto
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

		var req shared.RequestEventDto
		if err := json.Unmarshal([]byte(data), &req); err != nil {
			consoleLog("Error parsing JSON: " + err.Error())
			return nil
		}

		consoleLog(fmt.Sprintf("Received request #%d: %s %s:%d%s",
			req.ID, req.Method, req.Host, req.Port, req.Path))

		m.Lines = append(m.Lines, req)
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
		html += "<tr>"
		html += "<td>" + fmt.Sprintf("%d", line.ID) + "</td>"

		if line.Method == "CONNECT" {
			html += "<td>🔐" + line.Method + "</td>"
		} else {
			html += "<td>👀" + line.Method + "</td>"
		}

		html += "<td>" + line.Host + "</td>"
		html += "<td>" + line.Path + "</td>"
		html += "</tr>"
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
