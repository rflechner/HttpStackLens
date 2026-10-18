//go:build js && wasm

package mcppanel

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"syscall/js"

	"httpStackLens/webui/wasm/dom"
	"httpStackLens/webui/wasm/shared"
)

//go:embed toolbar.html
var toolbarHTML string

//go:embed calls.html
var callsHTML string

//go:embed detail.html
var detailHTML string

// rowHeight is the height of a trace row, in pixels, as set by .mp-tr. A call
// arriving on top of a list scrolled down pushes it by that much.
const rowHeight = 27

// ── toolbar ────────────────────────────────────────────────────────────────

// CallsToolbar holds the filter. It re-renders only on its own toggles: the
// count, which changes with every call, belongs to the list below.
type CallsToolbar struct {
	dom.Base
	owner *Panel
}

func (t *CallsToolbar) Template() string { return toolbarHTML }

func (t *CallsToolbar) ErrorsOnly() bool { return t.owner.ErrorsOnly }
func (t *CallsToolbar) Filter() string   { return t.owner.Filter }

// FilterChanged narrows the list as the filter is typed, without re-rendering
// the toolbar holding the input.
func (t *CallsToolbar) FilterChanged(e dom.Event) {
	t.owner.Filter = e.Value()
	t.owner.list.refresh(0)
}

func (t *CallsToolbar) ToggleErrors(e dom.Event) {
	t.owner.ErrorsOnly = e.Checked()
	t.owner.list.refresh(0)
}

func (t *CallsToolbar) Clear() {
	go func() {
		if err := apiCall("DELETE", callsPath, nil, nil); err != nil {
			js.Global().Get("console").Call("warn", "[mcp] clear trace: "+err.Error())
			return
		}
		t.owner.clearLocal()
	}()
}

// ── list ───────────────────────────────────────────────────────────────────

// CallsList is the trace table, newest call first.
type CallsList struct {
	dom.Base
	owner *Panel

	// scrollTop is restored after a render, which replaces the scrolled node.
	scrollTop int
}

func (l *CallsList) Template() string { return callsHTML }

// refresh re-renders the list, keeping the reader where they were. shifted
// is the number of rows inserted on top, which push the content down: a list
// left at the very top stays there to show them.
func (l *CallsList) refresh(shifted int) {
	if rows := l.Ref("rows"); rows.Truthy() {
		l.scrollTop = rows.Get("scrollTop").Int()
		if l.scrollTop > 0 {
			l.scrollTop += shifted * rowHeight
		}
	}
	l.StateHasChanged()
}

func (l *CallsList) OnAfterRender(bool) {
	if rows := l.Ref("rows"); rows.Truthy() && l.scrollTop > 0 {
		rows.Set("scrollTop", l.scrollTop)
	}
}

func (l *CallsList) Rows() []row {
	calls := l.owner.Visible()
	out := make([]row, len(calls))
	for i, call := range calls {
		out[i] = row{McpCallDto: call, Selected: call.ID == l.owner.Selected}
	}
	return out
}

func (l *CallsList) Loaded() bool  { return l.owner.Loaded }
func (l *CallsList) Total() int    { return len(l.owner.Calls) }
func (l *CallsList) Capacity() int { return l.owner.Capacity }
func (l *CallsList) Running() bool { return l.owner.State.Running }
func (l *CallsList) URL() string   { return l.owner.State.URL }
func (l *CallsList) Filtered() bool {
	return l.owner.ErrorsOnly || strings.TrimSpace(l.owner.Filter) != ""
}

func (l *CallsList) Select(e dom.Event) {
	l.owner.selectCall(int64(e.ArgInt()))
}

// row is a call as the table shows it.
type row struct {
	shared.McpCallDto
	Selected bool
}

func (r row) Time() string     { return localTime(r.At, true) }
func (r row) Duration() string { return formatDuration(r.DurationMs, r.Method) }
func (r row) What() string     { return what(r.McpCallDto) }

// ── detail ─────────────────────────────────────────────────────────────────

// CallDetail shows the selected call: what was sent and what came back.
type CallDetail struct {
	dom.Base
	owner *Panel
}

func (d *CallDetail) Template() string { return detailHTML }

func (d *CallDetail) Call() *shared.McpCallDto { return d.owner.selectedCall() }

func (d *CallDetail) Title() string {
	if call := d.Call(); call != nil {
		return what(*call)
	}
	return ""
}

func (d *CallDetail) When() string {
	if call := d.Call(); call != nil {
		return localTime(call.At, true)
	}
	return ""
}

func (d *CallDetail) Duration() string {
	if call := d.Call(); call != nil {
		return formatDuration(call.DurationMs, call.Method)
	}
	return ""
}

func (d *CallDetail) Arguments() string {
	if call := d.Call(); call != nil {
		return prettyJSON(call.Arguments)
	}
	return ""
}

func (d *CallDetail) Result() string {
	if call := d.Call(); call != nil {
		return prettyJSON(call.Result)
	}
	return ""
}

func (d *CallDetail) Close() { d.owner.selectCall(0) }

func (d *CallDetail) CopyArguments() {
	if call := d.Call(); call != nil {
		dom.Clipboard(call.Arguments)
	}
}

func (d *CallDetail) CopyResult() {
	if call := d.Call(); call != nil {
		dom.Clipboard(call.Result)
	}
}

// ── helpers ────────────────────────────────────────────────────────────────

// what names a call by what it did: the tool for a tool call, the method
// otherwise.
func what(call shared.McpCallDto) string {
	switch {
	case call.Tool != "":
		return call.Tool
	case call.Method == "http":
		return "rejected request"
	}
	return call.Method
}

// matches is a case-insensitive search across what a developer would type:
// tool, method, status, client and error.
func matches(call shared.McpCallDto, filter string) bool {
	filter = strings.ToLower(strings.TrimSpace(filter))
	if filter == "" {
		return true
	}
	for _, field := range []string{call.Tool, call.Method, call.Status, call.Client, call.Error, call.Remote} {
		if strings.Contains(strings.ToLower(field), filter) {
			return true
		}
	}
	return false
}

func formatDuration(ms float64, method string) string {
	if method == "http" {
		return "—"
	}
	switch {
	case ms < 1:
		return fmt.Sprintf("%.2f ms", ms)
	case ms < 1000:
		return fmt.Sprintf("%.0f ms", ms)
	}
	return fmt.Sprintf("%.2f s", ms/1000)
}

// localTime formats an RFC3339 instant in the viewer's time zone, which the
// Go runtime does not know under WASM — the browser does.
func localTime(at string, millis bool) string {
	d := js.Global().Get("Date").New(at)
	if v := d.Call("getTime"); v.IsNaN() {
		return at
	}
	text := fmt.Sprintf("%02d:%02d:%02d", d.Call("getHours").Int(), d.Call("getMinutes").Int(), d.Call("getSeconds").Int())
	if millis {
		text += fmt.Sprintf(".%03d", d.Call("getMilliseconds").Int())
	}
	return text
}

// prettyJSON indents a traced payload. A truncated one is not valid JSON any
// more and is shown as it is.
func prettyJSON(raw string) string {
	if raw == "" {
		return ""
	}
	var out bytes.Buffer
	if err := json.Indent(&out, []byte(raw), "", "  "); err != nil {
		return raw
	}
	return out.String()
}
