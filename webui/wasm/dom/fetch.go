//go:build js && wasm

package dom

import (
	"errors"
	"syscall/js"
)

// HTTPResponse is a fetch() result flattened into plain Go values.
type HTTPResponse struct {
	Status     int
	StatusText string
	Headers    [][2]string
	Body       string
}

// Fetch performs a request and blocks until it completes. It must be called
// from a goroutine — blocking inside an event handler would deadlock the
// single JS thread the callbacks need to run on:
//
//	func (c *Composer) Send(e dom.Event) {
//	    c.Sending = true
//	    c.StateHasChanged()
//	    go c.send()
//	}
func Fetch(method, url string, headers map[string]string, body string) (HTTPResponse, error) {
	type outcome struct {
		res HTTPResponse
		err error
	}
	done := make(chan outcome, 1)

	var res HTTPResponse
	var onResp, onText, onErr js.Func

	onText = js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			res.Body = args[0].String()
		}
		done <- outcome{res: res}
		return nil
	})
	onErr = js.FuncOf(func(_ js.Value, args []js.Value) any {
		msg := "network error"
		if len(args) > 0 && args[0].Truthy() {
			msg = args[0].Call("toString").String()
		}
		done <- outcome{err: errors.New(msg)}
		return nil
	})
	onResp = js.FuncOf(func(_ js.Value, args []js.Value) any {
		r := args[0]
		res.Status = r.Get("status").Int()
		res.StatusText = r.Get("statusText").String()
		entries := js.Global().Get("Array").Call("from", r.Get("headers").Call("entries"))
		for i := range entries.Length() {
			e := entries.Index(i)
			res.Headers = append(res.Headers, [2]string{e.Index(0).String(), e.Index(1).String()})
		}
		return r.Call("text").Call("then", onText).Call("catch", onErr)
	})

	hdrs := map[string]any{}
	for k, v := range headers {
		hdrs[k] = v
	}
	opts := map[string]any{"method": method, "headers": hdrs, "mode": "cors"}
	if body != "" {
		opts["body"] = body
	}
	js.Global().Call("fetch", url, opts).Call("then", onResp).Call("catch", onErr)

	out := <-done
	onResp.Release()
	onText.Release()
	onErr.Release()
	return out.res, out.err
}

// LocalGet reads a localStorage key, returning "" when absent.
func LocalGet(key string) string {
	v := js.Global().Get("localStorage").Call("getItem", key)
	if v.Type() != js.TypeString {
		return ""
	}
	return v.String()
}

// LocalSet writes a localStorage key, ignoring quota errors.
func LocalSet(key, value string) {
	defer func() { recover() }()
	js.Global().Get("localStorage").Call("setItem", key, value)
}
