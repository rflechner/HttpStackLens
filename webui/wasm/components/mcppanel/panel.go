//go:build js && wasm

// Package mcppanel is the MCP tab: the state and settings of the local MCP
// listener on the left, the trace of the calls it received on the right.
//
// The main column has two views: the trace of the calls, and the catalog of
// the tools MCP clients are offered.
//
// The root component renders once and never again. Everything that changes
// lives in a child — server pane, toolbar, call list, call detail, tool
// catalog — so that a call arriving while a filter or a port is being typed
// re-renders the list only, and the input keeps its focus.
package mcppanel

import (
	_ "embed"
	"syscall/js"

	"httpStackLens/webui/wasm/dom"
	"httpStackLens/webui/wasm/shared"
)

//go:embed panel.html
var panelHTML string

//go:embed mcp.css
var panelCSS string

// Panel is the root component and the model its children read.
type Panel struct {
	dom.Base

	State    shared.McpStateDto
	Settings shared.McpSettingsDto
	Loaded   bool
	LoadErr  string

	// Calls is newest first, capped at Capacity like the backend trace.
	Calls    []shared.McpCallDto
	Capacity int

	Filter     string
	ErrorsOnly bool
	Selected   int64

	// View is the main column: viewCalls or viewTools.
	View       string
	ToolFilter string

	server  *ServerPane
	toolbar *CallsToolbar
	list    *CallsList
	detail  *CallDetail
	tools   *ToolsList
}

// viewKey remembers the view the main column was left on.
const viewKey = "hsl-mcp-view"

const (
	viewCalls = "calls"
	viewTools = "tools"
)

// New returns an unmounted panel. Pass it to dom.Mount.
func New() *Panel { return &Panel{} }

func (p *Panel) Template() string { return panelHTML }

func (p *Panel) Styles() string { return panelCSS }

func (p *Panel) OnInit() {
	p.server = &ServerPane{owner: p}
	p.toolbar = &CallsToolbar{owner: p}
	p.list = &CallsList{owner: p}
	p.detail = &CallDetail{owner: p}
	p.tools = &ToolsList{owner: p}
	p.SetChild("server", p.server)
	p.SetChild("toolbar", p.toolbar)
	p.SetChild("calls", p.list)
	p.SetChild("detail", p.detail)
	p.SetChild("tools", p.tools)
	p.View = viewCalls
	if dom.LocalGet(viewKey) == viewTools {
		p.View = viewTools
	}
	// Fetching blocks: it has to leave the JS thread its callbacks run on.
	go p.load()
}

func (p *Panel) OnAfterRender(first bool) {
	if first {
		p.showView()
	}
}

// setView switches the main column. The hosts are shown and hidden rather
// than re-rendered, so that each view keeps its scroll and selection.
func (p *Panel) setView(view string) {
	if view != viewCalls && view != viewTools {
		return
	}
	p.View = view
	dom.LocalSet(viewKey, view)
	p.showView()
	p.toolbar.StateHasChanged()
}

func (p *Panel) showView() {
	calls := p.View == viewCalls
	setDisplay(p.Ref("calls"), calls)
	setDisplay(p.Ref("detail"), calls)
	setDisplay(p.Ref("tools"), !calls)
	if !calls {
		p.tools.ensureLoaded()
	}
}

func setDisplay(node js.Value, visible bool) {
	if !node.Truthy() {
		return
	}
	display := "none"
	if visible {
		display = ""
	}
	node.Get("style").Set("display", display)
}

func (p *Panel) load() {
	state, err := fetchState()
	if err == nil {
		var settings shared.McpSettingsDto
		settings, err = fetchSettings()
		p.Settings = settings
	}
	var calls shared.McpCallsDto
	if err == nil {
		calls, err = fetchCalls()
	}
	if err != nil {
		p.LoadErr = err.Error()
	} else {
		p.State, p.Calls, p.Capacity = state, calls.Calls, calls.Capacity
		p.LoadErr = ""
	}
	p.Loaded = true
	p.server.syncDraft()
	p.refreshAll()
}

func (p *Panel) refreshAll() {
	p.server.StateHasChanged()
	p.toolbar.StateHasChanged()
	p.list.refresh(0)
	p.detail.StateHasChanged()
}

// ApplyState takes an mcp_state event. The state never carries the token, so
// the settings are read again: the change may have come from another window.
func (p *Panel) ApplyState(state shared.McpStateDto) {
	p.State = state
	p.server.StateHasChanged()
	if state.CallCount == 0 && len(p.Calls) > 0 {
		p.clearLocal()
	}
	go func() {
		if settings, err := fetchSettings(); err == nil {
			p.Settings = settings
			p.server.StateHasChanged()
		}
	}()
}

// AddCall takes an mcp_call event.
func (p *Panel) AddCall(call shared.McpCallDto) {
	if len(p.Calls) > 0 && p.Calls[0].ID >= call.ID {
		return // already listed by the initial fetch
	}
	limit := p.Capacity
	if limit <= 0 {
		limit = 200
	}
	p.Calls = append([]shared.McpCallDto{call}, p.Calls...)
	if len(p.Calls) > limit {
		p.Calls = p.Calls[:limit]
	}
	p.State.CallCount = len(p.Calls)
	if call.Tool != "" && p.View == viewTools {
		p.tools.refresh() // the per-tool call counts
	}
	shifted := 0
	if p.visible(call) {
		shifted = 1
	}
	p.list.refresh(shifted)
}

func (p *Panel) clearLocal() {
	p.Calls = nil
	p.Selected = 0
	p.list.refresh(0)
	p.detail.StateHasChanged()
}

// Visible is the trace as filtered by the toolbar.
func (p *Panel) Visible() []shared.McpCallDto {
	out := make([]shared.McpCallDto, 0, len(p.Calls))
	for _, call := range p.Calls {
		if p.visible(call) {
			out = append(out, call)
		}
	}
	return out
}

func (p *Panel) visible(call shared.McpCallDto) bool {
	if p.ErrorsOnly && call.Status == "ok" {
		return false
	}
	return matches(call, p.Filter)
}

func (p *Panel) selectedCall() *shared.McpCallDto {
	for i := range p.Calls {
		if p.Calls[i].ID == p.Selected {
			return &p.Calls[i]
		}
	}
	return nil
}

func (p *Panel) selectCall(id int64) {
	if p.Selected == id {
		id = 0
	}
	p.Selected = id
	p.list.refresh(0)
	p.detail.StateHasChanged()
}
