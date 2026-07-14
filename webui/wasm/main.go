//go:build js && wasm

package main

import (
	"encoding/base64"
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
	if id == "certificates" {
		DisplayCertificates(js.Undefined(), nil)
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

func sendAjaxRequest[T any](url string, method string, payload T, callback js.Func) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("serialize request payload: %w", err)
	}
	opts := map[string]any{
		"method":  method,
		"headers": map[string]any{"Content-Type": "application/json"},
		"body":    string(body),
	}
	js.Global().Call("fetch", url, opts).
		Call("then", callback).
		Call("catch", js.FuncOf(func(this js.Value, args []js.Value) any {
			consoleLog(method + " " + url + " failed: " + args[0].String())
			return nil
		}))
	return nil
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

		doc.Call("getElementById", "config-cert-ca-file").Set("textContent", config.DecryptHttps.CertManager.CaCertFile)
		doc.Call("getElementById", "config-cert-ca-key-file").Set("textContent", config.DecryptHttps.CertManager.CaKeyFile)

		return nil
	}))
	return nil
}

func DisplayCertificates(this js.Value, args []js.Value) any {
	fetchText("/certificates-infos", js.FuncOf(func(this js.Value, args []js.Value) any {
		var certsInfos shared.CertificatesInfosDto
		if err := json.Unmarshal([]byte(args[0].String()), &certsInfos); err != nil {
			consoleLog("Error parsing config JSON: " + err.Error())
			return nil
		}

		doc := js.Global().Get("document")

		doc.Call("getElementById", "cert-ca-common-name").Set("textContent", certsInfos.CaCertSubject)
		installed := "not installed"
		if certsInfos.Installed {
			installed = "installed"
		} else if !certsInfos.InstallSupported {
			installed = "manual install"
		} else if certsInfos.InstallCheckError != "" {
			installed = "unknown"
		}
		doc.Call("getElementById", "cert-ca-installed").Set("textContent", installed)

		return nil
	}))
	return nil
}

// ── SSE ───────────────────────────────────────────────────────────────────

// setSSEStatus updates the connection indicator when present. The v2 (mockup)
// UI has no #sse-status element, so this is a no-op there rather than a crash.
func setSSEStatus(html string) {
	el := js.Global().Get("document").Call("getElementById", "sse-status")
	if el.IsNull() || el.IsUndefined() {
		return
	}
	el.Set("innerHTML", html)
}

func (m *StateModel) connectSSE() {
	es := js.Global().Get("EventSource").New("/events")

	es.Set("onopen", js.FuncOf(func(this js.Value, args []js.Value) any {
		setSSEStatus("🟢 Connected to server stream")
		return nil
	}))

	es.Set("onerror", js.FuncOf(func(this js.Value, args []js.Value) any {
		setSSEStatus("🔴 Disconnected from server stream")
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

		if len(m.Lines) > 100 {
			m.Lines = m.Lines[50:]
		}

		m.Lines = append(m.Lines, req)
		m.RequestCount++
		m.appendRow(req)
		return nil
	}))

	es.Call("addEventListener", "response_occurred", js.FuncOf(func(this js.Value, args []js.Value) any {
		data := args[0].Get("data").String()
		if data == "" {
			return nil
		}
		var resp shared.ResponseEventDto
		if err := json.Unmarshal([]byte(data), &resp); err != nil {
			consoleLog("Error parsing response JSON: " + err.Error())
			return nil
		}
		m.updateRow(resp)
		return nil
	}))

	es.Call("addEventListener", "capture_state", js.FuncOf(func(this js.Value, args []js.Value) any {
		data := args[0].Get("data").String()
		if data == "" {
			return nil
		}
		var st shared.CaptureStateDto
		if err := json.Unmarshal([]byte(data), &st); err != nil {
			return nil
		}
		callMockup("setCaptureState", captureStateToJS(st))
		return nil
	}))
}

// captureStateToJS flattens the capture_state DTO into the shape the JS status
// bar consumes (capturing + live decrypt/upstream/access states).
func captureStateToJS(st shared.CaptureStateDto) map[string]any {
	return map[string]any{
		"recording":  st.Recording,
		"capturing":  st.Capturing,
		"bufferSize": st.BufferSize,
		"proxy":      map[string]any{"running": st.Proxy.Running},
		"decrypt":    map[string]any{"enabled": st.Decrypt.Enabled},
		"upstream":   map[string]any{"enabled": st.Upstream.Enabled, "ntlm": st.Upstream.Ntlm},
		"access":     map[string]any{"mode": st.Access.Mode},
	}
}

// loadCaptureState fetches the current capture state once at boot so the status
// bar reflects the real backend state immediately, before any mutation event.
func (m *StateModel) loadCaptureState() {
	fetchText("/api/capture/state", js.FuncOf(func(this js.Value, args []js.Value) any {
		var st shared.CaptureStateDto
		if err := json.Unmarshal([]byte(args[0].String()), &st); err != nil {
			return nil
		}
		callMockup("setCaptureState", captureStateToJS(st))
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
	if m.appendMockupRow(line) {
		return
	}

	doc := js.Global().Get("document")
	tbody := doc.Call("getElementById", "request-rows")
	if tbody.IsNull() || tbody.IsUndefined() {
		return
	}

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

// appendMockupRow pushes a new request into the v2 (mockup) UI. It returns
// false when that UI isn't present (e.g. the legacy index.html table), so the
// caller can fall back to the plain DOM table.
func (m *StateModel) appendMockupRow(line shared.RequestEventDto) bool {
	return callMockup("appendRow", map[string]any{
		"id":            line.ID,
		"method":        line.Method,
		"scheme":        line.Scheme,
		"host":          line.Host,
		"path":          line.Path,
		"version":       line.Version,
		"tls":           line.Tls,
		"decrypted":     line.Decrypted,
		"correlationId": line.CorrelationID,
	})
}

// updateRow completes an existing row when its response arrives.
func (m *StateModel) updateRow(resp shared.ResponseEventDto) {
	callMockup("updateRow", map[string]any{
		"correlationId": resp.CorrelationID,
		"status":        resp.Status,
		"statusText":    resp.StatusText,
		"mime":          resp.ContentType,
		"size":          resp.Size,
		"ms":            resp.DurationMs,
		"stream":        resp.Stream,
		"bodyAvailable": resp.BodyAvailable,
		"bodySkipped":   resp.BodySkipped,
	})
}

// callMockup invokes window.HttpStackLensMockup[method](args...) when the v2 UI
// is loaded. Returns false when the API (or that method) isn't available.
func callMockup(method string, args ...any) bool {
	api := js.Global().Get("HttpStackLensMockup")
	if api.IsUndefined() || api.IsNull() {
		return false
	}
	fn := api.Get(method)
	if fn.Type() != js.TypeFunction {
		return false
	}
	fn.Invoke(args...)
	return true
}

// ── Detail / body / capture bridges ─────────────────────────────────────────

// registerBridges exposes the functions the JS render layer calls back into:
// lazy detail/body fetches and capture control.
func (m *StateModel) registerBridges() {
	js.Global().Set("hslLoadDetail", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) >= 1 {
			m.loadDetail(args[0].String())
		}
		return nil
	}))
	js.Global().Set("hslLoadBody", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) >= 2 {
			m.loadBody(args[0].String(), args[1].String())
		}
		return nil
	}))
	js.Global().Set("hslCapture", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) >= 1 {
			capture(args[0].String())
		}
		return nil
	}))
	js.Global().Set("hslProxy", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) >= 1 {
			proxyAction(args[0].String())
		}
		return nil
	}))
	js.Global().Set("hslDecryptHttps", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) >= 1 && args[0].Type() == js.TypeBoolean {
			m.decryptHttps(args[0].Bool())
		}
		return nil
	}))
	js.Global().Set("hslLoadBodyCapture", js.FuncOf(func(this js.Value, args []js.Value) any {
		m.loadBodyCapture()
		return nil
	}))
	js.Global().Set("hslSaveBodyCapture", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) >= 1 {
			var payload shared.BodyCaptureSettingsDto
			if err := json.Unmarshal([]byte(args[0].String()), &payload); err != nil {
				callMockup("setBodyCapture", map[string]any{"error": "Invalid body capture settings."})
				return nil
			}
			m.saveBodyCapture(payload)
		}
		return nil
	}))
	js.Global().Set("hslLoadUpstream", js.FuncOf(func(this js.Value, args []js.Value) any {
		m.loadUpstream()
		return nil
	}))
	js.Global().Set("hslSaveUpstream", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) >= 1 {
			var payload shared.UpstreamSettingsDto
			if err := json.Unmarshal([]byte(args[0].String()), &payload); err != nil {
				callMockup("setUpstream", map[string]any{"error": "Invalid upstream proxy settings."})
				return nil
			}
			m.saveUpstream(payload)
		}
		return nil
	}))
	js.Global().Set("hslCertificateAction", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) >= 1 {
			m.certificateAction(args[0].String())
		}
		return nil
	}))

	js.Global().Set("hslLoadAccessControl", js.FuncOf(func(this js.Value, args []js.Value) any {
		m.loadAccessControl()
		return nil
	}))
	js.Global().Set("hslSaveAccessControl", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) >= 1 {
			var payload shared.AccessControlConfigDto
			if err := json.Unmarshal([]byte(args[0].String()), &payload); err != nil {
				consoleLog("Invalid access control settings: " + err.Error())
				return nil
			}
			m.saveAccessMode(payload)
		}
		return nil
	}))
}

func headersToJS(headers []shared.HeaderDto) []any {
	arr := make([]any, len(headers))
	for i, h := range headers {
		arr[i] = []any{h.Name, h.Value}
	}
	return arr
}

func detailToJS(d shared.RequestDetailDto) map[string]any {
	out := map[string]any{"createdAt": d.CreatedAt}
	if d.Request != nil {
		out["request"] = map[string]any{
			"method":        d.Request.Method,
			"url":           d.Request.URL,
			"httpVersion":   d.Request.HttpVersion,
			"headers":       headersToJS(d.Request.Headers),
			"bodyAvailable": d.Request.BodyAvailable,
			"bodySkipped":   d.Request.BodySkipped,
			"bodySize":      d.Request.BodySize,
		}
	}
	if d.Response != nil {
		out["response"] = map[string]any{
			"status":        d.Response.Status,
			"statusText":    d.Response.StatusText,
			"httpVersion":   d.Response.HttpVersion,
			"headers":       headersToJS(d.Response.Headers),
			"bodyAvailable": d.Response.BodyAvailable,
			"bodySkipped":   d.Response.BodySkipped,
			"bodySize":      d.Response.BodySize,
		}
	}
	if d.Timing != nil {
		out["timing"] = map[string]any{
			"dnsMs":      d.Timing.DnsMs,
			"connectMs":  d.Timing.ConnectMs,
			"tlsMs":      d.Timing.TlsMs,
			"ttfbMs":     d.Timing.TtfbMs,
			"downloadMs": d.Timing.DownloadMs,
			"totalMs":    d.Timing.TotalMs,
		}
	}
	return out
}

func (m *StateModel) loadDetail(correlationID string) {
	if correlationID == "" {
		return
	}
	fetchText("/api/requests/"+correlationID, js.FuncOf(func(this js.Value, args []js.Value) any {
		var d shared.RequestDetailDto
		if err := json.Unmarshal([]byte(args[0].String()), &d); err != nil {
			callMockup("setDetail", correlationID, map[string]any{"error": "Could not load request detail."})
			return nil
		}
		callMockup("setDetail", correlationID, detailToJS(d))
		return nil
	}))
}

func (m *StateModel) loadBody(correlationID, side string) {
	if correlationID == "" {
		callMockup("setBody", correlationID, side, map[string]any{"available": false, "error": "No correlation id."})
		return
	}
	url := "/api/requests/" + correlationID + "/body?side=" + side
	js.Global().Call("fetch", url).
		Call("then", js.FuncOf(func(this js.Value, args []js.Value) any {
			res := args[0]
			status := res.Get("status").Int()
			if status == 413 { // body was captured-skipped
				callMockup("setBody", correlationID, side, map[string]any{"available": false, "skipped": true})
				return nil
			}
			if status < 200 || status >= 300 {
				callMockup("setBody", correlationID, side, map[string]any{"available": false})
				return nil
			}
			contentType := ""
			if ct := res.Get("headers").Call("get", "content-type"); ct.Type() == js.TypeString {
				contentType = ct.String()
			}
			res.Call("arrayBuffer").Call("then", js.FuncOf(func(this js.Value, targs []js.Value) any {
				bytes := make([]byte, targs[0].Get("byteLength").Int())
				js.CopyBytesToGo(bytes, js.Global().Get("Uint8Array").New(targs[0]))
				callMockup("setBody", correlationID, side, map[string]any{
					"available":   true,
					"bodyBase64":  base64.StdEncoding.EncodeToString(bytes),
					"contentType": contentType,
				})
				return nil
			}))
			return nil
		})).
		Call("catch", js.FuncOf(func(this js.Value, args []js.Value) any {
			callMockup("setBody", correlationID, side, map[string]any{"available": false, "error": "Could not fetch body."})
			return nil
		}))
}

// bodyCaptureToJS flattens the body-capture DTO into the shape the JS settings
// panel consumes. Kb/Mb limits are normalized to bytes so the editor works in a
// single unit; the default cap is omitted when unset.
func bodyCaptureToJS(dto shared.BodyCaptureSettingsDto) map[string]any {
	rules := make([]any, 0, len(dto.MimeTypes))
	for _, r := range dto.MimeTypes {
		rule := map[string]any{"name": r.Name}
		switch {
		case r.MaxSizeBytes != nil:
			rule["maxSizeBytes"] = float64(*r.MaxSizeBytes)
		case r.MaxSizeKb != nil:
			rule["maxSizeBytes"] = *r.MaxSizeKb * 1024
		case r.MaxSizeMb != nil:
			rule["maxSizeBytes"] = *r.MaxSizeMb * 1024 * 1024
		}
		rules = append(rules, rule)
	}
	out := map[string]any{"loaded": true, "mimeTypes": rules}
	if dto.DefaultMaxBytes != nil {
		out["defaultMaxBytes"] = float64(*dto.DefaultMaxBytes)
	}
	return out
}

func certificateInfosToJS(dto shared.CertificatesInfosDto) map[string]any {
	return map[string]any{
		"available":             dto.Available,
		"ca_cert_subject":       dto.CaCertSubject,
		"ca_cert_issuer":        dto.CaCertIssuer,
		"ca_cert_serial_number": dto.CaCertSerialNumber,
		"fingerprint_sha256":    dto.FingerprintSha256,
		"not_before":            dto.NotBefore,
		"not_after":             dto.NotAfter,
		"expired":               dto.Expired,
		"install_supported":     dto.InstallSupported,
		"installed":             dto.Installed,
		"install_check_error":   dto.InstallCheckError,
		"error":                 dto.Error,
	}
}

func (m *StateModel) loadBodyCapture() {
	fetchText("/api/settings/body-capture", js.FuncOf(func(this js.Value, args []js.Value) any {
		var dto shared.BodyCaptureSettingsDto
		if err := json.Unmarshal([]byte(args[0].String()), &dto); err != nil {
			callMockup("setBodyCapture", map[string]any{"error": "Could not load body capture settings."})
			return nil
		}
		callMockup("setBodyCapture", bodyCaptureToJS(dto))
		return nil
	}))
}

// saveBodyCapture PUTs the JSON payload built by the JS panel and echoes the
// server's normalized result back (with a saved flag), or surfaces the error.
func (m *StateModel) saveBodyCapture(payload shared.BodyCaptureSettingsDto) {
	err := sendAjaxRequest("/api/settings/body-capture", "PUT", payload,
		js.FuncOf(func(this js.Value, args []js.Value) any {
			res := args[0]
			status := res.Get("status").Int()
			res.Call("text").Call("then", js.FuncOf(func(this js.Value, targs []js.Value) any {
				text := targs[0].String()
				if status < 200 || status >= 300 {
					msg := text
					if msg == "" {
						msg = "Could not save body capture settings."
					}
					callMockup("setBodyCapture", map[string]any{"error": msg})
					return nil
				}
				var dto shared.BodyCaptureSettingsDto
				if err := json.Unmarshal([]byte(text), &dto); err != nil {
					callMockup("setBodyCapture", map[string]any{"saved": true})
					return nil
				}
				out := bodyCaptureToJS(dto)
				out["saved"] = true
				callMockup("setBodyCapture", out)
				return nil
			}))
			return nil
		}))
	if err != nil {
		callMockup("setBodyCapture", map[string]any{"error": "Could not serialize body capture settings."})
	}
}

func (m *StateModel) saveAccessMode(accessMode shared.AccessControlConfigDto) {
	payload := shared.AccessControlSettingsDto{
		Proxy: accessMode,
		WebUi: accessMode,
	}
	if err := sendAjaxRequest("/api/settings/access-control", "PUT", payload,
		js.FuncOf(func(this js.Value, args []js.Value) any {
			res := args[0]
			status := res.Get("status").Int()
			res.Call("text").Call("then", js.FuncOf(func(this js.Value, textArgs []js.Value) any {
				body := textArgs[0].String()
				if status < 200 || status >= 300 {
					if body == "" {
						body = "Could not save access control settings."
					}
					callMockup("setAccessControl", map[string]any{"error": body})
					return nil
				}
				var dto shared.AccessControlSettingsDto
				if err := json.Unmarshal([]byte(body), &dto); err != nil {
					callMockup("setAccessControl", map[string]any{"error": "Could not read saved access control settings."})
					return nil
				}
				out := accessControlToJS(dto.Proxy)
				out["saved"] = true
				callMockup("setAccessControl", out)
				return nil
			}))
			return nil
		})); err != nil {
		callMockup("setAccessControl", map[string]any{"error": "Could not serialize access control settings."})
	}
}

func upstreamToJS(dto shared.UpstreamSettingsDto) map[string]any {
	noProxy := make([]any, len(dto.NoProxy))
	for i, host := range dto.NoProxy {
		noProxy[i] = host
	}
	return map[string]any{
		"outputProxyUri": dto.OutputProxyUri,
		"noProxy":        noProxy,
		"ntlm":           dto.AddWindowsAuthentication,
	}
}

func (m *StateModel) loadUpstream() {
	fetchText("/api/settings/upstream", js.FuncOf(func(this js.Value, args []js.Value) any {
		var dto shared.UpstreamSettingsDto
		if err := json.Unmarshal([]byte(args[0].String()), &dto); err != nil {
			callMockup("setUpstream", map[string]any{"error": "Could not load upstream proxy settings."})
			return nil
		}
		callMockup("setUpstream", upstreamToJS(dto))
		return nil
	}))
}

func (m *StateModel) saveUpstream(payload shared.UpstreamSettingsDto) {
	if err := sendAjaxRequest("/api/settings/upstream", "PUT", payload,
		js.FuncOf(func(this js.Value, args []js.Value) any {
			res := args[0]
			status := res.Get("status").Int()
			res.Call("text").Call("then", js.FuncOf(func(this js.Value, textArgs []js.Value) any {
				body := textArgs[0].String()
				if status < 200 || status >= 300 {
					if body == "" {
						body = "Could not save upstream proxy settings."
					}
					callMockup("setUpstream", map[string]any{"error": body})
					return nil
				}
				var dto shared.UpstreamSettingsDto
				if err := json.Unmarshal([]byte(body), &dto); err != nil {
					callMockup("setUpstream", map[string]any{"error": "Could not read saved upstream proxy settings."})
					return nil
				}
				out := upstreamToJS(dto)
				out["saved"] = true
				callMockup("setUpstream", out)
				return nil
			}))
			return nil
		})); err != nil {
		callMockup("setUpstream", map[string]any{"error": "Could not serialize upstream proxy settings."})
	}
}

func accessControlToJS(config shared.AccessControlConfigDto) map[string]any {
	networks := make([]any, len(config.Networks))
	for i, network := range config.Networks {
		networks[i] = network
	}
	return map[string]any{"mode": config.Mode, "networks": networks}
}

func (m *StateModel) loadAccessControl() {
	fetchText("/api/settings/access-control", js.FuncOf(func(this js.Value, args []js.Value) any {
		var dto shared.AccessControlSettingsDto
		if err := json.Unmarshal([]byte(args[0].String()), &dto); err != nil {
			callMockup("setAccessControl", map[string]any{"error": "Could not load access control settings."})
			return nil
		}
		callMockup("setAccessControl", accessControlToJS(dto.Proxy))
		return nil
	}))
}

func capture(action string) {
	var path string
	switch action {
	case "stop":
		path = "/api/recording/stop"
	case "start":
		path = "/api/recording/start"
	case "clear":
		path = "/api/recording/clear"
	default:
		return
	}
	js.Global().Call("fetch", path, map[string]any{"method": "POST"}).
		Call("catch", js.FuncOf(func(this js.Value, args []js.Value) any {
			consoleLog("capture " + action + " failed: " + args[0].String())
			return nil
		}))
}

func proxyAction(action string) {
	if action != "start" && action != "stop" {
		return
	}
	js.Global().Call("fetch", "/api/proxy/"+action, map[string]any{"method": "POST"}).
		Call("catch", js.FuncOf(func(this js.Value, args []js.Value) any {
			consoleLog("proxy " + action + " failed: " + args[0].String())
			return nil
		}))
}

func (m *StateModel) decryptHttps(enabled bool) {
	payload := shared.DecryptHttpsToggleSettingsDto{Enabled: enabled}
	if err := sendAjaxRequest("/api/settings/decrypt-https", "PUT", payload,
		js.FuncOf(func(this js.Value, args []js.Value) any {
			return nil
		})); err != nil {
		consoleLog("Could not serialize HTTPS decryption settings: " + err.Error())
	}
}

// certificateAction connects the TLS settings UI to the B6 certificate API.
// Key generation is a single synchronous server operation, so the UI exposes a
// simple busy state instead of pretending that byte-level progress is known.
func (m *StateModel) certificateAction(action string) {
	url := "/certificates-infos"
	method := "GET"
	var options any

	switch action {
	case "status":
		options = js.Undefined()
	case "generate", "regenerate":
		url = "/api/certificates/ca/generate"
		method = "POST"
		payload, err := json.Marshal(shared.CertificateGenerateRequestDto{Replace: action == "regenerate"})
		if err != nil {
			callMockup("setCertificate", map[string]any{"requestError": "Could not prepare certificate generation."})
			return
		}
		options = map[string]any{
			"method":  method,
			"headers": map[string]any{"Content-Type": "application/json"},
			"body":    string(payload),
		}
	case "install":
		url = "/api/certificates/ca/install"
		method = "POST"
		options = map[string]any{"method": method}
	default:
		callMockup("setCertificate", map[string]any{"requestError": "Unknown certificate action."})
		return
	}

	var request js.Value
	if action == "status" {
		request = js.Global().Call("fetch", url)
	} else {
		request = js.Global().Call("fetch", url, options)
	}
	request.Call("then", js.FuncOf(func(this js.Value, args []js.Value) any {
		response := args[0]
		status := response.Get("status").Int()
		return response.Call("text").Call("then", js.FuncOf(func(this js.Value, textArgs []js.Value) any {
			body := textArgs[0].String()
			if status < 200 || status >= 300 {
				if body == "" {
					body = fmt.Sprintf("Certificate request failed (HTTP %d).", status)
				}
				callMockup("setCertificate", map[string]any{"requestError": body})
				return nil
			}
			var dto shared.CertificatesInfosDto
			if err := json.Unmarshal([]byte(body), &dto); err != nil {
				callMockup("setCertificate", map[string]any{"requestError": "Could not read certificate status."})
				return nil
			}
			callMockup("setCertificate", certificateInfosToJS(dto))
			return nil
		}))
	})).Call("catch", js.FuncOf(func(this js.Value, args []js.Value) any {
		message := "Certificate request failed."
		if len(args) > 0 {
			message += " " + args[0].String()
		}
		callMockup("setCertificate", map[string]any{"requestError": message})
		return nil
	}))
}

// ── Main ──────────────────────────────────────────────────────────────────

func dismissLoader() {
	loader := js.Global().Get("document").Call("getElementById", "loader")
	loader.Get("classList").Call("add", "fade-out")
}

func main() {
	//initTheme()
	//initTabs()

	js.Global().Set("DisplayConfig", js.FuncOf(DisplayConfig))
	js.Global().Set("DisplayCertificates", js.FuncOf(DisplayCertificates))

	model := &StateModel{}
	model.registerBridges()
	model.connectSSE()
	model.loadCaptureState()

	dismissLoader()

	select {}
}
