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
		m.appendRow(req)
		return nil
	}))
}

func methodClass(method string) string {
	switch method {
	case "CONNECT":
		return "method-connect"
	case "GET":
		return "method-get"
	case "POST":
		return "method-post"
	default:
		return ""
	}
}

func methodIcon(method string) string {
	switch method {
	case "CONNECT":
		return "🔐"
	case "GET":
		return "🔵"
	case "POST":
		return "🟢"
	default:
		return "👀"
	}
}

func (m *StateModel) appendRow(line shared.RequestEventDto) {
	doc := js.Global().Get("document")
	tbody := doc.Call("getElementById", "request-rows")

	tr := doc.Call("createElement", "tr")
	tr.Set("className", "border-b border-border-subtle hover:bg-surface-alt/50 transition-colors "+methodClass(line.Method))

	tdID := doc.Call("createElement", "td")
	tdID.Set("className", "px-3 py-2 text-dim tabular-nums")
	tdID.Set("textContent", fmt.Sprintf("%d", line.ID))

	tdMethod := doc.Call("createElement", "td")
	tdMethod.Set("className", "px-3 py-2 font-mono font-semibold")
	tdMethod.Set("textContent", methodIcon(line.Method)+" "+line.Method)

	tdHost := doc.Call("createElement", "td")
	tdHost.Set("className", "px-3 py-2 text-muted")
	tdHost.Set("textContent", line.Host)

	tdPath := doc.Call("createElement", "td")
	tdPath.Set("className", "px-3 py-2 text-dim truncate max-w-xs")
	tdPath.Set("textContent", line.Path)

	tr.Call("appendChild", tdID)
	tr.Call("appendChild", tdMethod)
	tr.Call("appendChild", tdHost)
	tr.Call("appendChild", tdPath)
	tbody.Call("prepend", tr)

	doc.Call("getElementById", "request-count").Set("textContent",
		fmt.Sprintf("%d requests", m.RequestCount))
}

func main() {

	model := &StateModel{
		RequestCount: 0,
	}
	model.connectSSE()

	// block forever
	select {}
}
