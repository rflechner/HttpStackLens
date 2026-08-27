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
// data-bind to write and a data-on-<event> to call. Events raised inside a
// nested component are left to it: its own listener saw them first, on the way
// up.
func dispatch(c Component, ev string, evt js.Value) {
	b := c.base()
	key := "on" + strings.ToUpper(ev[:1]) + ev[1:]
	bindable := ev == "input" || ev == "change"

	if insideChild(evt.Get("target"), b.root) {
		return
	}

	for node := evt.Get("target"); node.Truthy() && !node.Equal(b.root); node = node.Get("parentElement") {
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

// insideChild reports whether target sits inside a nested component hosted
// under root. The whole chain has to be checked before any handler runs: the
// element carrying data-on-<event> is a descendant of the data-child node, so
// looking for the boundary along the way would always find the handler first
// and fire the parent's method as well as the child's.
//
// This is what keeps two components from answering one click when they happen
// to name a handler the same — a response pane and its composer both having a
// SetTab, say.
func insideChild(target, root js.Value) bool {
	for node := target; node.Truthy() && !node.Equal(root); node = node.Get("parentElement") {
		if node.Get("dataset").Get("child").Truthy() {
			return true
		}
	}
	return false
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
