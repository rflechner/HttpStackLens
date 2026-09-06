//go:build js && wasm

package dom

import (
	"bytes"
	"fmt"
	"html/template"
	"reflect"
	"strings"
	"sync"
	"syscall/js"
)

// Mount attaches a component to a DOM element and renders it. Mounting an
// already-mounted component moves it: its listeners are released and re-bound
// on the new node, but its state and OnInit are preserved.
func Mount(root js.Value, c Component) {
	if !root.Truthy() {
		console("Mount on a missing element: " + typeName(c))
		return
	}
	b := c.base()
	releaseHandlers(b)
	b.self = c
	b.root = root
	b.tmpl = templateFor(c)
	injectStyles(c)
	bindListeners(c)
	if init, ok := c.(Initializer); ok && !b.inited {
		b.inited = true
		init.OnInit()
	}
	Render(c)
}

// Unmount releases the component and its children, then empties its node.
func Unmount(c Component) {
	b := c.base()
	for _, child := range b.children {
		Unmount(child)
	}
	releaseHandlers(b)
	if d, ok := c.(Disposer); ok {
		d.OnDispose()
	}
	if b.root.Truthy() {
		b.root.Set("innerHTML", "")
	}
	b.root = js.Undefined()
	b.rendered = false
}

// Render executes the template and replaces the component's subtree. Prefer
// StateHasChanged, which coalesces renders within a frame.
func Render(c Component) {
	b := c.base()
	if !b.root.Truthy() || b.tmpl == nil {
		return
	}
	var buf bytes.Buffer
	if err := b.tmpl.Execute(&buf, c); err != nil {
		console("render " + typeName(c) + ": " + err.Error())
		b.root.Set("innerHTML", `<pre style="color:#d1584f;white-space:pre-wrap">`+
			template.HTMLEscapeString(typeName(c)+": "+err.Error())+`</pre>`)
		return
	}
	b.root.Set("innerHTML", buf.String())
	applyBindings(c)
	mountChildren(c)
	first := !b.rendered
	b.rendered = true
	if r, ok := c.(Rendered); ok {
		r.OnAfterRender(first)
	}
}

// mountChildren wires every data-child placeholder to its registered
// component. Grandchildren are unreachable at this point — their host nodes
// are still empty — so the query cannot cross a component boundary.
func mountChildren(c Component) {
	b := c.base()
	if len(b.children) == 0 {
		return
	}
	slots := b.root.Call("querySelectorAll", "[data-child]")
	for i := range slots.Length() {
		node := slots.Index(i)
		name := node.Get("dataset").Get("child").String()
		if child, ok := b.children[name]; ok {
			Mount(node, child)
		}
	}
}

func releaseHandlers(b *Base) {
	for _, f := range b.handlers {
		f.Release()
	}
	b.handlers = nil
}

// ── templates ──────────────────────────────────────────────────────────────

var (
	tmplMu    sync.Mutex
	tmplCache = map[reflect.Type]*template.Template{}
)

var builtinFuncs = template.FuncMap{
	// css marks a value as a safe CSS token — html/template rejects
	// var(--mint) in a style attribute otherwise.
	"css": func(v any) template.CSS { return template.CSS(fmt.Sprint(v)) },
	// attr injects raw attributes, the only way to emit a bare `disabled`.
	"attr": func(v any) template.HTMLAttr { return template.HTMLAttr(fmt.Sprint(v)) },
	// html injects trusted markup (icons and the like).
	"html": func(v any) template.HTML { return template.HTML(fmt.Sprint(v)) },
	"add":  func(a, b int) int { return a + b },
	"join": strings.Join,
}

func templateFor(c Component) *template.Template {
	t := reflect.TypeOf(c)
	tmplMu.Lock()
	defer tmplMu.Unlock()
	if cached, ok := tmplCache[t]; ok {
		return cached
	}
	funcs := template.FuncMap{}
	for k, v := range builtinFuncs {
		funcs[k] = v
	}
	if f, ok := c.(Funcs); ok {
		for k, v := range f.TemplateFuncs() {
			funcs[k] = v
		}
	}
	parsed, err := template.New(typeName(c)).Funcs(funcs).Parse(c.Template())
	if err != nil {
		console("parse " + typeName(c) + ": " + err.Error())
		parsed = template.Must(template.New("err").Parse(
			`<pre style="color:#d1584f;white-space:pre-wrap">` +
				template.HTMLEscapeString(typeName(c)+": "+err.Error()) + `</pre>`))
	}
	tmplCache[t] = parsed
	return parsed
}

// ── styles ─────────────────────────────────────────────────────────────────

var injected = map[string]bool{}

func injectStyles(c Component) {
	s, ok := c.(Styler)
	if !ok {
		return
	}
	name := typeName(c)
	if injected[name] {
		return
	}
	injected[name] = true
	doc := js.Global().Get("document")
	el := doc.Call("createElement", "style")
	el.Set("textContent", s.Styles())
	el.Get("dataset").Set("component", name)
	doc.Get("head").Call("appendChild", el)
}

func typeName(c Component) string {
	t := reflect.TypeOf(c)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.PkgPath()[strings.LastIndexByte(t.PkgPath(), '/')+1:] + "." + t.Name()
}
