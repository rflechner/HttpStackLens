//go:build js && wasm

package dom

import (
	"reflect"
	"strconv"
	"strings"
	"syscall/js"
)

// applyBindings pushes field values into the DOM after a render. The template
// could emit them, but doing it here keeps inputs, selects, textareas and
// checkboxes uniform, and avoids re-setting a value that has not changed —
// which would move the caret.
func applyBindings(c Component) {
	b := c.base()
	nodes := b.root.Call("querySelectorAll", "[data-bind]")
	for i := range nodes.Length() {
		node := nodes.Index(i)
		if owner := node.Call("closest", "[data-child]"); owner.Truthy() {
			continue // belongs to a nested component
		}
		v, ok := fieldByPath(c, node.Get("dataset").Get("bind").String())
		if !ok {
			continue
		}
		if strings.EqualFold(node.Get("type").String(), "checkbox") {
			want := v.Kind() == reflect.Bool && v.Bool()
			if node.Get("checked").Bool() != want {
				node.Set("checked", want)
			}
			continue
		}
		want := formatValue(v)
		if node.Get("value").String() != want {
			node.Set("value", want)
		}
	}
}

// writeBinding is the other half of the two-way binding: DOM back into the
// struct field. It deliberately does not re-render — that would destroy the
// input being typed into. Add data-bind-render to opt in.
func writeBinding(c Component, node js.Value, path string) {
	v, ok := fieldByPath(c, path)
	if !ok || !v.CanSet() {
		console("cannot bind " + typeName(c) + "." + path)
		return
	}
	if strings.EqualFold(node.Get("type").String(), "checkbox") && v.Kind() == reflect.Bool {
		v.SetBool(node.Get("checked").Bool())
		return
	}
	raw := node.Get("value").String()
	switch v.Kind() {
	case reflect.String:
		v.SetString(raw)
	case reflect.Bool:
		v.SetBool(raw == "true")
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
			v.SetInt(n)
		}
	case reflect.Float32, reflect.Float64:
		if f, err := strconv.ParseFloat(raw, 64); err == nil {
			v.SetFloat(f)
		}
	default:
		console("unsupported bind target " + path + " (" + v.Kind().String() + ")")
	}
}

// fieldByPath walks a dotted path from the component to an addressable field.
// Slice indexes are path elements, so a repeated row can bind straight into
// the model: data-bind="Req.Headers.{{$i}}.Key".
func fieldByPath(c Component, path string) (reflect.Value, bool) {
	v := reflect.ValueOf(c)
	for _, part := range strings.Split(path, ".") {
		for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
			if v.IsNil() {
				return reflect.Value{}, false
			}
			v = v.Elem()
		}
		switch v.Kind() {
		case reflect.Slice, reflect.Array:
			i, err := strconv.Atoi(part)
			if err != nil || i < 0 || i >= v.Len() {
				return reflect.Value{}, false
			}
			v = v.Index(i)
		case reflect.Struct:
			f := v.FieldByName(part)
			if !f.IsValid() {
				return reflect.Value{}, false
			}
			v = f
		default:
			return reflect.Value{}, false
		}
	}
	return v, true
}

func formatValue(v reflect.Value) string {
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Bool:
		return strconv.FormatBool(v.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'f', -1, 64)
	default:
		return ""
	}
}
