//go:build js && wasm

package dom

import (
	"reflect"
	"regexp"
	"strings"
	"syscall/js"
)

var onAttrRe = regexp.MustCompile(`data-on-([a-z]+)\s*=`)

// bindListeners registers one delegated listener per event type used in the
// template. Handlers therefore survive re-renders, dynamically inserted rows
// are live without extra work, and the number of js.Func per component stays
// small and bounded.
func bindListeners(c Component) {
	b := c.base()
	events := map[string]bool{}
	src := c.Template()
	for _, m := range onAttrRe.FindAllStringSubmatch(src, -1) {
		events[m[1]] = true
	}
	if strings.Contains(src, "data-bind") {
		events["input"] = true
		events["change"] = true
	}
	for ev := range events {
		fn := js.FuncOf(func(_ js.Value, args []js.Value) any {
			if len(args) > 0 {
				dispatch(c, ev, args[0])
			}
			return nil
		})
		b.handlers = append(b.handlers, fn)
		b.root.Call("addEventListener", ev, fn)
	}
}

// dispatch walks from the event target up to the component root, looking for a
// data-bind to write and a data-on-<event> to call. It stops at any nested
// component root: that subtree has its own listener and handled the event
// first, on the way up.
func dispatch(c Component, ev string, evt js.Value) {
	b := c.base()
	key := "on" + strings.ToUpper(ev[:1]) + ev[1:]
	bindable := ev == "input" || ev == "change"

	for node := evt.Get("target"); node.Truthy() && !node.Equal(b.root); node = node.Get("parentElement") {
		if node.Get("dataset").Get("child").Truthy() {
			return // belongs to a child component
		}
		if bindable {
			if path := node.Get("dataset").Get("bind"); path.Truthy() {
				writeBinding(c, node, path.String())
				if node.Get("dataset").Get("bindRender").Truthy() {
					b.StateHasChanged()
				}
			}
		}
		if name := node.Get("dataset").Get(key); name.Truthy() {
			invoke(c, name.String(), Event{Raw: evt, Node: node, Name: ev})
			return
		}
	}
}

var eventType = reflect.TypeOf(Event{})

// invoke calls the named method. Handlers take either no argument or a single
// Event — this is where data-on-click="Save" becomes c.Save(e).
func invoke(c Component, name string, e Event) {
	m := reflect.ValueOf(c).MethodByName(name)
	if !m.IsValid() {
		console(typeName(c) + " has no handler " + name)
		return
	}
	t := m.Type()
	switch {
	case t.NumIn() == 0:
		m.Call(nil)
	case t.NumIn() == 1 && t.In(0) == eventType:
		m.Call([]reflect.Value{reflect.ValueOf(e)})
	default:
		console(typeName(c) + "." + name + " must take no argument or a dom.Event")
	}
}
