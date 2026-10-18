//go:build js && wasm

package mcppanel

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"httpStackLens/webui/wasm/dom"
	"httpStackLens/webui/wasm/shared"
)

//go:embed tools.html
var toolsHTML string

// ToolsList is the catalog of the tools MCP clients are offered: what each one
// says it does, the arguments a model can fill in, and the behaviour it
// declares. It is read when first shown: the catalog only changes with the
// application.
type ToolsList struct {
	dom.Base
	owner *Panel

	Tools   []shared.McpToolDto
	Loading bool
	LoadErr string
	// Schemas lists the tools whose raw JSON Schemas are unfolded.
	Schemas map[string]bool

	loaded    bool
	scrollTop int
}

func (l *ToolsList) Template() string { return toolsHTML }

func (l *ToolsList) OnInit() { l.Schemas = map[string]bool{} }

func (l *ToolsList) ensureLoaded() {
	if !l.loaded {
		l.reload()
	}
}

func (l *ToolsList) reload() {
	if l.Loading {
		return
	}
	l.loaded, l.Loading, l.LoadErr = true, true, ""
	l.StateHasChanged()
	go func() {
		tools, err := fetchTools()
		if err != nil {
			l.LoadErr = err.Error()
		} else {
			l.Tools = tools
		}
		l.Loading = false
		l.StateHasChanged()
	}()
}

// refresh re-renders the catalog where the reader left it.
func (l *ToolsList) refresh() {
	if l.IsMounted() {
		l.scrollTop = l.Root().Get("scrollTop").Int()
	}
	l.StateHasChanged()
}

func (l *ToolsList) OnAfterRender(bool) {
	if l.scrollTop > 0 {
		l.Root().Set("scrollTop", l.scrollTop)
	}
}

// ── template accessors ─────────────────────────────────────────────────────

func (l *ToolsList) Filtered() bool { return strings.TrimSpace(l.owner.ToolFilter) != "" }

func (l *ToolsList) Cards() []toolCard {
	filter := strings.ToLower(strings.TrimSpace(l.owner.ToolFilter))
	counts := map[string]int{}
	for _, call := range l.owner.Calls {
		if call.Tool != "" {
			counts[call.Tool]++
		}
	}
	var cards []toolCard
	for _, tool := range l.Tools {
		if filter != "" && !strings.Contains(strings.ToLower(tool.Name+" "+tool.Title+" "+tool.Description), filter) {
			continue
		}
		cards = append(cards, toolCard{
			McpToolDto: tool,
			Params:     schemaParams(tool.InputSchema),
			Calls:      counts[tool.Name],
			ShowSchema: l.Schemas[tool.Name],
		})
	}
	return cards
}

// Summary counts the tools by what they may do, the first thing to know
// before handing them to a model.
func (l *ToolsList) Summary() string {
	var readOnly, writes, network int
	for _, tool := range l.Tools {
		if tool.ReadOnly {
			readOnly++
		} else {
			writes++
		}
		if tool.OpenWorld {
			network++
		}
	}
	return fmt.Sprintf("%d tools · %d read-only · %d can change things · %d may reach the network",
		len(l.Tools), readOnly, writes, network)
}

func (l *ToolsList) ToggleSchema(e dom.Event) {
	name := e.Arg()
	l.Schemas[name] = !l.Schemas[name]
	l.refresh()
}

// ShowCalls opens the trace filtered on one tool.
func (l *ToolsList) ShowCalls(e dom.Event) {
	l.owner.Filter = e.Arg()
	l.owner.ErrorsOnly = false
	l.owner.setView(viewCalls)
	l.owner.list.refresh(0)
}

// ── cards ──────────────────────────────────────────────────────────────────

type toolCard struct {
	shared.McpToolDto
	Params     []param
	Calls      int
	ShowSchema bool
}

// Effect is the one-word answer to "what can this tool do to my machine": a
// read-only tool reads, a non-destructive one only adds, any other may change
// or delete. Tools without annotations fall in the last case, as the MCP
// defaults have clients assume.
func (c toolCard) Effect() string {
	switch {
	case c.ReadOnly:
		return "read-only"
	case !c.Destructive:
		return "adds"
	}
	return "may modify"
}

func (c toolCard) EffectClass() string {
	switch {
	case c.ReadOnly:
		return "ok"
	case !c.Destructive:
		return "tool_error"
	}
	return "error"
}

func (c toolCard) EffectHint() string {
	switch {
	case c.ReadOnly:
		return "readOnlyHint: the tool does not change anything"
	case !c.Destructive:
		return "destructiveHint false: the tool may add things but does not modify or delete existing ones"
	}
	return "The tool may modify or delete existing things (destructiveHint is true, or absent and true by default)"
}

func (c toolCard) InputJSON() string  { return prettyJSON(string(c.InputSchema)) }
func (c toolCard) OutputJSON() string { return prettyJSON(string(c.OutputSchema)) }

// param is one argument of a tool, read from its input schema.
type param struct {
	Name        string
	Type        string
	Required    bool
	Description string
}

// schemaProp is the part of a JSON Schema property the catalog shows. Type is
// a string or a list of them.
type schemaProp struct {
	Type        any          `json:"type"`
	Description string       `json:"description"`
	Enum        []any        `json:"enum"`
	Items       *schemaProp  `json:"items"`
	Default     any          `json:"default"`
	Minimum     *json.Number `json:"minimum"`
	Maximum     *json.Number `json:"maximum"`
}

// schemaParams lists the top-level arguments, required ones first. Nested
// objects are shown by their type only: the raw schema has the rest.
func schemaParams(raw json.RawMessage) []param {
	var schema struct {
		Properties map[string]schemaProp `json:"properties"`
		Required   []string              `json:"required"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &schema) != nil {
		return nil
	}
	required := map[string]bool{}
	for _, name := range schema.Required {
		required[name] = true
	}
	params := make([]param, 0, len(schema.Properties))
	for name, prop := range schema.Properties {
		params = append(params, param{Name: name, Type: typeName(prop), Required: required[name], Description: prop.Description})
	}
	sort.Slice(params, func(i, j int) bool {
		if params[i].Required != params[j].Required {
			return params[i].Required
		}
		return params[i].Name < params[j].Name
	})
	return params
}

func typeName(prop schemaProp) string {
	var types []string
	switch t := prop.Type.(type) {
	case string:
		types = []string{t}
	case []any:
		for _, v := range t {
			if s, ok := v.(string); ok && s != "null" {
				types = append(types, s)
			}
		}
	}
	name := strings.Join(types, " | ")
	if name == "array" && prop.Items != nil {
		name = "array of " + typeName(*prop.Items)
	}
	if name == "" {
		name = "any"
	}
	if len(prop.Enum) > 0 {
		values := make([]string, len(prop.Enum))
		for i, v := range prop.Enum {
			values[i] = fmt.Sprint(v)
		}
		name += " (" + strings.Join(values, ", ") + ")"
	}
	if prop.Minimum != nil && prop.Maximum != nil {
		name += fmt.Sprintf(" %s–%s", *prop.Minimum, *prop.Maximum)
	}
	if prop.Default != nil {
		name += fmt.Sprintf(", default %v", prop.Default)
	}
	return name
}
