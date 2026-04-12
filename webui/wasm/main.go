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

// ── Theme ─────────────────────────────────────────────────────────────────

func initTheme() {
	localStorage := js.Global().Get("localStorage")
	theme := localStorage.Call("getItem", "hsl-theme")
	themeStr := "slate"
	if !theme.IsNull() && !theme.IsUndefined() && theme.String() != "" {
		themeStr = theme.String()
	}
	js.Global().Get("document").Get("documentElement").Get("dataset").Set("theme", themeStr)
}

// ── Tabs ──────────────────────────────────────────────────────────────────

func setTab(id string) {
	doc := js.Global().Get("document")
	js.Global().Get("localStorage").Call("setItem", "hsl-tab", id)

	btns := doc.Call("querySelectorAll", ".tab-btn")
	for i := 0; i < btns.Length(); i++ {
		btn := btns.Index(i)
		cl := btn.Get("classList")
		if btn.Get("dataset").Get("tab").String() == id {
			cl.Call("add", "text-foreground", "border-b-2", "border-foreground")
			cl.Call("remove", "text-dim")
		} else {
			cl.Call("remove", "text-foreground", "border-b-2", "border-foreground")
			cl.Call("add", "text-dim")
		}
	}

	panels := doc.Call("querySelectorAll", ".tab-panel")
	for i := 0; i < panels.Length(); i++ {
		panel := panels.Index(i)
		cl := panel.Get("classList")
		if panel.Get("id").String() == "tab-"+id {
			cl.Call("remove", "hidden")
			cl.Call("add", "flex")
		} else {
			cl.Call("add", "hidden")
			cl.Call("remove", "flex")
		}
	}

	if id == "configuration" {
		DisplayConfig(js.Undefined(), nil)
	}
}

func initTabs() {
	doc := js.Global().Get("document")

	btns := doc.Call("querySelectorAll", ".tab-btn")
	for i := 0; i < btns.Length(); i++ {
		btn := btns.Index(i)
		tabID := btn.Get("dataset").Get("tab").String()
		btn.Call("addEventListener", "click", js.FuncOf(func(this js.Value, args []js.Value) any {
			setTab(tabID)
			return nil
		}))
	}

	saved := js.Global().Get("localStorage").Call("getItem", "hsl-tab")
	initial := "traffic"
	if !saved.IsNull() && !saved.IsUndefined() && saved.String() != "" {
		initial = saved.String()
	}
	setTab(initial)
}

// ── Config ────────────────────────────────────────────────────────────────

func fetchText(url string, callback js.Func) {
	js.Global().Call("fetch", url).
		Call("then", js.FuncOf(func(this js.Value, args []js.Value) any {
			return args[0].Call("text")
		})).
		Call("then", callback).
		Call("catch", js.FuncOf(func(this js.Value, args []js.Value) any {
			consoleLog("Error fetching " + url + ": " + args[0].String())
			return nil
		}))
}

func DisplayConfig(this js.Value, args []js.Value) any {
	fetchText("/config", js.FuncOf(func(this js.Value, args []js.Value) any {
		var config shared.AppConfigDto
		if err := json.Unmarshal([]byte(args[0].String()), &config); err != nil {
			consoleLog("Error parsing config JSON: " + err.Error())
			return nil
		}

		doc := js.Global().Get("document")
		doc.Call("getElementById", "config-proxy-port").Set("textContent",
			fmt.Sprintf("%d", config.Proxy.Port))
		doc.Call("getElementById", "config-proxy-remote").Set("textContent",
			fmt.Sprintf("%t", config.Proxy.EnableRemoteConnection))
		doc.Call("getElementById", "config-webui-port").Set("textContent",
			fmt.Sprintf("%d", config.WebUi.Port))
		doc.Call("getElementById", "config-webui-remote").Set("textContent",
			fmt.Sprintf("%t", config.WebUi.EnableRemoteConnection))

		return nil
	}))
	return nil
}

// ── SSE ───────────────────────────────────────────────────────────────────

func (m *StateModel) connectSSE() {
	es := js.Global().Get("EventSource").New("/events")

	es.Set("onopen", js.FuncOf(func(this js.Value, args []js.Value) any {
		js.Global().Get("document").
			Call("getElementById", "sse-status").
			Set("innerHTML", "🟢 Connected to server stream")
		return nil
	}))

	es.Set("onerror", js.FuncOf(func(this js.Value, args []js.Value) any {
		js.Global().Get("document").
			Call("getElementById", "sse-status").
			Set("innerHTML", "🔴 Disconnected from server stream")
		return nil
	}))

	es.Call("addEventListener", "request_occurred", js.FuncOf(func(this js.Value, args []js.Value) any {
		data := args[0].Get("data").String()
		if data == "" {
			return nil
		}
		var req shared.RequestEventDto
		if err := json.Unmarshal([]byte(data), &req); err != nil {
			consoleLog("Error parsing request JSON: " + err.Error())
			return nil
		}
		m.Lines = append(m.Lines, req)
		m.RequestCount++
		m.appendRow(req)
		return nil
	}))
}

// ── Render ────────────────────────────────────────────────────────────────

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

// ── Main ──────────────────────────────────────────────────────────────────

func main() {
	initTheme()
	initTabs()

	js.Global().Set("DisplayConfig", js.FuncOf(DisplayConfig))

	model := &StateModel{}
	model.connectSSE()

	select {}
}
