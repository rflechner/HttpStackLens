//go:build js && wasm

package dom

import (
	"strconv"
	"strings"
	"syscall/js"
)

// Event is what a handler receives. Node is the element carrying the
// data-on-<event> attribute, not necessarily the one that was clicked.
type Event struct {
	Raw  js.Value // the native DOM event
	Node js.Value // the element the handler was declared on
	Name string   // "click", "input", …
}

// Value returns the value of the event target (input, select, textarea).
func (e Event) Value() string {
	t := e.Raw.Get("target")
	if !t.Truthy() {
		return ""
	}
	v := t.Get("value")
	if v.Type() != js.TypeString {
		return ""
	}
	return v.String()
}

// Checked returns the checked state of the event target.
func (e Event) Checked() bool { return e.Raw.Get("target").Get("checked").Truthy() }

// Key returns the pressed key for keyboard events.
func (e Event) Key() string { return e.Raw.Get("key").String() }

// Meta reports whether ⌘ or Ctrl was held.
func (e Event) Meta() bool {
	return e.Raw.Get("metaKey").Truthy() || e.Raw.Get("ctrlKey").Truthy()
}

// Arg returns data-arg on the handler's element — the parameter of the call,
// e.g. data-on-click="Select" data-arg="{{.ID}}".
func (e Event) Arg() string { return e.dataset(e.Node, "arg") }

// ArgOf returns data-arg-<name>, for handlers that need more than one value.
func (e Event) ArgOf(name string) string {
	return e.dataset(e.Node, "arg"+strings.ToUpper(name[:1])+name[1:])
}

// ArgInt is Arg parsed as an int, or -1.
func (e Event) ArgInt() int { return atoi(e.Arg()) }

// ArgIntOf is ArgOf parsed as an int, or -1.
func (e Event) ArgIntOf(name string) int { return atoi(e.ArgOf(name)) }

func (e Event) PreventDefault()  { e.Raw.Call("preventDefault") }
func (e Event) StopPropagation() { e.Raw.Call("stopPropagation") }

func (e Event) dataset(node js.Value, key string) string {
	if !node.Truthy() {
		return ""
	}
	v := node.Get("dataset").Get(key)
	if v.Type() != js.TypeString {
		return ""
	}
	return v.String()
}

func atoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return -1
	}
	return n
}
