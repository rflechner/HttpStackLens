//go:build js && wasm

// Package dom is a small component runtime for Go/WASM front-ends.
//
// A component is a struct embedding [Base], paired with an HTML template that
// lives next to it — the equivalent of a Foo.razor / Foo.razor.cs pair:
//
//	//go:embed toolbar.html
//	var toolbarHTML string
//
//	type Toolbar struct {
//	    dom.Base
//	    Title string
//	}
//
//	func (t *Toolbar) Template() string  { return toolbarHTML }
//	func (t *Toolbar) Save(e dom.Event)  { t.saved = true; t.StateHasChanged() }
//
// The template carries the wiring, resolved at mount time:
//
//	data-on-click="Save"     calls the Save method            (≈ @onclick)
//	data-bind="Title"        two-way binds a field            (≈ @bind)
//	data-ref="input"         exposes the node as c.Ref(...)   (≈ @ref)
//	data-child="response"    hosts a nested component         (≈ <Response />)
//
// Rendering replaces the component's whole subtree, so keep components small:
// a re-render loses focus, selection and scroll position inside it. Event
// handlers survive because they are delegated to the component root, not
// attached to individual nodes.
package dom

import (
	"html/template"
	"strconv"
	"syscall/js"
)

// Component is implemented by any struct embedding [Base].
type Component interface {
	// Template returns the html/template source for this component.
	Template() string

	// base is unexported on purpose: embedding Base is the only way in.
	base() *Base
}

// Optional lifecycle hooks. Implement the ones you need.
type (
	// Initializer runs once, before the first render. ≈ OnInitialized.
	Initializer interface{ OnInit() }

	// Rendered runs after every render, with first=true on the first one.
	// ≈ OnAfterRender.
	Rendered interface{ OnAfterRender(first bool) }

	// Disposer runs when the component is unmounted. ≈ IDisposable.
	Disposer interface{ OnDispose() }

	// Styler returns CSS injected once per component type. ≈ Foo.razor.css,
	// minus the scoping — prefix your selectors.
	Styler interface{ Styles() string }

	// Funcs adds template functions on top of the built-ins.
	Funcs interface{ TemplateFuncs() template.FuncMap }
)

// Base holds the runtime state of a component. Embed it by value.
type Base struct {
	self     Component
	root     js.Value
	tmpl     *template.Template
	children map[string]Component
	handlers []js.Func
	inited   bool
	rendered bool
	pending  bool
}

func (b *Base) base() *Base { return b }

// Root is the element the component is mounted on. Its inner HTML belongs to
// the component; the element itself belongs to the parent.
func (b *Base) Root() js.Value { return b.root }

// IsMounted reports whether the component currently owns a DOM node.
func (b *Base) IsMounted() bool { return b.root.Truthy() }

// StateHasChanged schedules a re-render on the next animation frame. Calling it
// several times in one event handler renders once.
func (b *Base) StateHasChanged() {
	if !b.root.Truthy() || b.pending {
		return
	}
	b.pending = true
	var cb js.Func
	cb = js.FuncOf(func(js.Value, []js.Value) any {
		cb.Release()
		b.pending = false
		if b.self != nil && b.root.Truthy() {
			Render(b.self)
		}
		return nil
	})
	// A hidden tab never runs animation frames, which would stall updates
	// arriving in the background (an SSE burst, a response to a request sent
	// before switching away).
	if js.Global().Get("document").Get("hidden").Truthy() {
		js.Global().Call("setTimeout", cb, 0)
	} else {
		js.Global().Call("requestAnimationFrame", cb)
	}
}

// Ref returns the node marked data-ref="name" in this component's template,
// or undefined. Valid only after a render.
func (b *Base) Ref(name string) js.Value {
	if !b.root.Truthy() {
		return js.Undefined()
	}
	return b.root.Call("querySelector", "[data-ref="+strconv.Quote(name)+"]")
}

// SetChild registers a component for the data-child="slot" placeholder. It is
// mounted on the next render and kept across re-renders of the parent: the
// parent's innerHTML replacement destroys the child's DOM, so the child is
// remounted onto the new node with its Go state intact.
func (b *Base) SetChild(slot string, c Component) {
	if b.children == nil {
		b.children = make(map[string]Component)
	}
	b.children[slot] = c
}

// Child returns the component registered for a slot, or nil.
func (b *Base) Child(slot string) Component { return b.children[slot] }

func console(msg string) {
	js.Global().Get("console").Call("warn", "[dom] "+msg)
}
