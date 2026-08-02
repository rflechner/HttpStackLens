//go:build js && wasm

// Package composer is the .http request composer, written as three components
// sharing one model: the shell and editor (Composer), the collections sidebar
// (FilesPane) and the result column (ResponsePane).
//
// They live in one package because the children hold a pointer back to the
// Composer — the direct equivalent of a cascading parameter. Across packages
// the children would take a small interface instead.
package composer

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"html/template"
	"strconv"
	"strings"
	"syscall/js"
	"time"

	httpfile "httpStackLens/composer"
	"httpStackLens/webui/wasm/dom"
)

//go:embed composer.html
var composerHTML string

//go:embed composer.css
var composerCSS string

const storeKey = "hsl-http-files"

// Composer is the root component: layout, request editor, and the model the
// two child panes read from.
type Composer struct {
	dom.Base

	Files   []*File
	CurFile *File
	Cur     *Request
	Params  []KV   // query rows, derived from Cur.URL while the Params tab is open
	Tab     string // body | headers | params | vars | raw
	Raw     string // .http draft, bound to the raw textarea
	Sending bool
	Res     *Result
	Dirty   map[string]bool

	files *FilesPane
	resp  *ResponsePane

	// rawScroll keeps the highlight overlay and the line numbers aligned with
	// the textarea. A scroll event does not bubble, so this one listener cannot
	// be delegated to the component root the way the others are.
	rawScroll js.Func
}

// New returns an unmounted composer. Pass it to dom.Mount.
func New() *Composer { return &Composer{} }

func (c *Composer) Template() string { return composerHTML }

// Styles ships the component's CSS, injected once on first mount.
func (c *Composer) Styles() string { return composerCSS }

func (c *Composer) OnInit() {
	c.Tab = "body"
	c.Dirty = map[string]bool{}
	c.load()
	c.files = &FilesPane{owner: c}
	c.resp = &ResponsePane{owner: c}
	c.SetChild("files", c.files)
	c.SetChild("response", c.resp)
	if len(c.Files) > 0 && len(c.Files[0].Reqs) > 0 {
		c.CurFile, c.Cur = c.Files[0], c.Files[0].Reqs[0]
	}
}

// ── template accessors ─────────────────────────────────────────────────────

func (c *Composer) MethodList() []string { return Methods }

func (c *Composer) TabIs(id string) bool { return c.Tab == id }

func (c *Composer) MethodColor() template.CSS { return methodColor(c.Cur.Method) }

func (c *Composer) FileDirty() bool { return c.CurFile != nil && c.Dirty[c.CurFile.ID] }

func (c *Composer) FileName() string {
	if c.CurFile == nil {
		return ""
	}
	return c.CurFile.Name
}

// SendAttrs shows the one place html/template needs help: a bare `disabled`
// cannot come from an action in attribute-name position, so it is emitted as a
// trusted attribute string instead.
func (c *Composer) SendAttrs() template.HTMLAttr {
	if c.Sending {
		return "disabled"
	}
	return ""
}

// RawText is the .http rendering of the current file, kept in Raw while the
// raw tab is open so that edits survive a re-render.
func (c *Composer) RawText() string {
	if c.Raw != "" {
		return c.Raw
	}
	if c.CurFile == nil {
		return ""
	}
	return ToHTTP(c.CurFile)
}

// RawHighlight is the coloured copy of the draft that sits behind the raw
// textarea. It comes out of the same parser the back end runs, so what the
// editor paints and what a request means can never drift apart.
func (c *Composer) RawHighlight() template.HTML {
	return template.HTML(httpfile.HighlightHTML(c.RawText()))
}

func (c *Composer) RawLines() []int {
	n := strings.Count(c.RawText(), "\n") + 1
	out := make([]int, n)
	for i := range out {
		out[i] = i + 1
	}
	return out
}

func (c *Composer) ReqCount() string {
	if c.CurFile == nil {
		return "0 requests"
	}
	if n := len(c.CurFile.Reqs); n == 1 {
		return "1 request"
	}
	return strconv.Itoa(len(c.CurFile.Reqs)) + " requests"
}

func methodColor(m string) template.CSS {
	switch m {
	case "GET":
		return "var(--mint)"
	case "POST":
		return "var(--warn)"
	case "PUT":
		return "var(--info)"
	case "PATCH":
		return "var(--pink)"
	case "DELETE":
		return "var(--danger)"
	default:
		return "var(--dim)"
	}
}

// ── handlers ───────────────────────────────────────────────────────────────

// SetTab switches editor tab, converting between the form and the raw text.
func (c *Composer) SetTab(e dom.Event) {
	next := e.Arg()
	if c.Tab == "raw" && next != "raw" {
		c.applyRaw()
	}
	if next == "raw" {
		c.Raw = ToHTTP(c.CurFile)
	} else {
		c.Raw = ""
	}
	if next == "params" && c.Cur != nil {
		_, c.Params = SplitURL(c.Cur.URL)
	}
	c.Tab = next
	c.StateHasChanged()
}

// MethodChanged re-renders: the verb colours the dropdown and the sidebar row.
func (c *Composer) MethodChanged() {
	c.touch()
	c.StateHasChanged()
	c.files.StateHasChanged()
}

// NameChanged only refreshes the sidebar — re-rendering the editor here would
// throw away the caret in the field being typed into.
func (c *Composer) NameChanged() {
	c.touch()
	c.files.StateHasChanged()
}

// Edited is the catch-all for fields whose value is already written by the
// two-way binding: nothing to render, just mark the file dirty.
func (c *Composer) Edited() { c.touch() }

// ParamChanged rebuilds the URL from the query rows and pushes it into the URL
// input directly — a render would move the caret in the row being edited.
func (c *Composer) ParamChanged() {
	if c.Cur == nil {
		return
	}
	base, _ := SplitURL(c.Cur.URL)
	c.Cur.URL = JoinURL(base, c.Params)
	if url := c.Ref("url"); url.Truthy() {
		url.Set("value", c.Cur.URL)
	}
	c.touch()
}

// RawChanged reparses the file on every keystroke and refreshes the sidebar
// only, so the textarea keeps focus, caret and scroll.
func (c *Composer) RawChanged() {
	c.applyRaw()
	c.files.StateHasChanged()
	if stat := c.Ref("rawStat"); stat.Truthy() {
		stat.Set("textContent", c.ReqCount())
	}
	if gutter := c.Ref("gutter"); gutter.Truthy() {
		gutter.Set("innerHTML", gutterHTML(c.RawLines()))
	}
	if highlight := c.Ref("highlight"); highlight.Truthy() {
		highlight.Set("innerHTML", string(c.RawHighlight()))
	}
	// A keystroke can scroll the textarea on its own — typing past the last
	// visible line — without firing a scroll event first.
	c.syncRawScroll()
}

// OnAfterRender rebinds the scroll listener: every render replaces the subtree,
// and with it the textarea the previous listener was attached to.
func (c *Composer) OnAfterRender(bool) {
	c.releaseRawScroll()
	area := c.Ref("rawArea")
	if !area.Truthy() {
		return
	}
	c.rawScroll = js.FuncOf(func(js.Value, []js.Value) any {
		c.syncRawScroll()
		return nil
	})
	area.Call("addEventListener", "scroll", c.rawScroll)
}

func (c *Composer) OnDispose() { c.releaseRawScroll() }

func (c *Composer) releaseRawScroll() {
	if c.rawScroll.Truthy() {
		c.rawScroll.Release()
		c.rawScroll = js.Func{}
	}
}

// syncRawScroll pins the overlay and the line numbers to the textarea. The
// overlay scrolls both ways, the gutter only down: it holds one column.
func (c *Composer) syncRawScroll() {
	area := c.Ref("rawArea")
	if !area.Truthy() {
		return
	}
	top, left := area.Get("scrollTop"), area.Get("scrollLeft")
	if highlight := c.Ref("highlight"); highlight.Truthy() {
		highlight.Set("scrollTop", top)
		highlight.Set("scrollLeft", left)
	}
	if gutter := c.Ref("gutter"); gutter.Truthy() {
		gutter.Set("scrollTop", top)
	}
}

func (c *Composer) OnKey(e dom.Event) {
	if e.Meta() && e.Key() == "Enter" {
		e.PreventDefault()
		c.Send()
	}
}

func (c *Composer) NewRequest() { c.newRequest(c.fileID()) }

func (c *Composer) DuplicateRequest() {
	if c.Cur == nil {
		return
	}
	clone := *c.Cur
	clone.ID = uid()
	clone.Name = c.Cur.Name + " copy"
	clone.Headers = append([]KV(nil), c.Cur.Headers...)
	c.CurFile.Reqs = append(c.CurFile.Reqs, &clone)
	c.selectRequest(c.CurFile.ID, clone.ID)
}

func (c *Composer) DeleteRequest() {
	if c.Cur == nil {
		return
	}
	kept := make([]*Request, 0, len(c.CurFile.Reqs))
	for _, r := range c.CurFile.Reqs {
		if r != c.Cur {
			kept = append(kept, r)
		}
	}
	c.CurFile.Reqs = kept
	c.Cur = nil
	if len(kept) > 0 {
		c.Cur = kept[0]
	}
	c.touch()
	c.StateHasChanged()
	c.files.StateHasChanged()
}

// KVAdd appends a row to the headers, params or variables table.
func (c *Composer) KVAdd(e dom.Event) {
	switch e.Arg() {
	case "headers":
		c.Cur.Headers = append(c.Cur.Headers, KV{On: true})
	case "params":
		c.Params = append(c.Params, KV{On: true})
	case "vars":
		c.CurFile.Vars = append(c.CurFile.Vars, KV{On: true})
	}
	c.touch()
	c.StateHasChanged()
}

func (c *Composer) KVDel(e dom.Event) {
	i := e.ArgIntOf("i")
	switch e.Arg() {
	case "headers":
		c.Cur.Headers = removeAt(c.Cur.Headers, i)
	case "params":
		c.Params = removeAt(c.Params, i)
		c.ParamChanged()
	case "vars":
		c.CurFile.Vars = removeAt(c.CurFile.Vars, i)
	}
	c.touch()
	c.StateHasChanged()
}

func (c *Composer) KVToggle(e dom.Event) {
	i := e.ArgIntOf("i")
	switch e.Arg() {
	case "headers":
		toggleAt(c.Cur.Headers, i)
	case "params":
		toggleAt(c.Params, i)
		c.ParamChanged()
	case "vars":
		toggleAt(c.CurFile.Vars, i)
	}
	c.touch()
	c.StateHasChanged()
}

func (c *Composer) FormatJSON() {
	if c.Cur == nil {
		return
	}
	var out bytes.Buffer
	if err := json.Indent(&out, []byte(c.Cur.Body), "", "  "); err != nil {
		return
	}
	c.Cur.Body = out.String()
	c.touch()
	c.StateHasChanged()
}

// Send fires the request. The handler itself must not block — the fetch runs
// in a goroutine so the JS callbacks it waits on can be delivered.
func (c *Composer) Send() {
	if c.Cur == nil || c.Sending {
		return
	}
	if c.Tab == "raw" {
		c.applyRaw()
	}
	c.Sending = true
	c.Res = nil
	c.StateHasChanged()
	go c.send()
}

func (c *Composer) send() {
	r, f := c.Cur, c.CurFile
	url := Interpolate(r.URL, f.Vars)
	headers := map[string]string{}
	for _, h := range r.Headers {
		if h.On && h.Key != "" {
			headers[h.Key] = Interpolate(h.Value, f.Vars)
		}
	}
	body := ""
	if r.Method != "GET" && r.Method != "HEAD" {
		body = Interpolate(r.Body, f.Vars)
	}

	start := time.Now()
	res, err := dom.Fetch(r.Method, url, headers, body)
	out := &Result{MS: int(time.Since(start).Milliseconds())}
	if err != nil {
		out.Err = err.Error()
	} else {
		out.Status, out.StatusText = res.Status, res.StatusText
		out.Headers, out.Body = res.Headers, res.Body
	}

	c.Sending = false
	c.Res = out
	c.resp.Tab = "body"
	c.StateHasChanged()
}

// ── model helpers ──────────────────────────────────────────────────────────

func (c *Composer) file(id string) *File {
	for _, f := range c.Files {
		if f.ID == id {
			return f
		}
	}
	return nil
}

func (c *Composer) fileID() string {
	if c.CurFile == nil {
		return ""
	}
	return c.CurFile.ID
}

func (c *Composer) selectRequest(fileID, reqID string) {
	f := c.file(fileID)
	if f == nil {
		return
	}
	c.CurFile = f
	c.Cur = f.find(reqID)
	c.Res = nil
	c.Raw = ""
	if c.Tab == "raw" {
		c.Tab = "body"
	}
	if c.Tab == "params" && c.Cur != nil {
		_, c.Params = SplitURL(c.Cur.URL)
	}
	c.StateHasChanged()
	c.files.StateHasChanged()
}

func (c *Composer) newRequest(fileID string) {
	f := c.file(fileID)
	if f == nil {
		return
	}
	r := &Request{
		ID:      uid(),
		Name:    "New request",
		Method:  "GET",
		Headers: []KV{{Key: "Accept", Value: "application/json", On: true}},
	}
	f.Reqs = append(f.Reqs, r)
	f.Open = true
	c.Dirty[f.ID] = true
	c.selectRequest(f.ID, r.ID)
}

// applyRaw reparses the raw draft into the current file, keeping the selection
// on the request at the same position when possible.
func (c *Composer) applyRaw() {
	if c.CurFile == nil || c.Raw == "" {
		return
	}
	idx := 0
	for i, r := range c.CurFile.Reqs {
		if r == c.Cur {
			idx = i
			break
		}
	}
	parsed := ParseHTTP(c.Raw, c.CurFile.Name)
	c.CurFile.Vars, c.CurFile.Reqs = parsed.Vars, parsed.Reqs
	c.Cur = nil
	if len(parsed.Reqs) > 0 {
		c.Cur = parsed.Reqs[min(idx, len(parsed.Reqs)-1)]
	}
	c.touch()
}

func (c *Composer) touch() {
	if c.CurFile != nil {
		c.Dirty[c.CurFile.ID] = true
	}
	c.save()
}

func (c *Composer) save() {
	if b, err := json.Marshal(c.Files); err == nil {
		dom.LocalSet(storeKey, string(b))
	}
}

func (c *Composer) load() {
	if raw := dom.LocalGet(storeKey); raw != "" {
		var files []*File
		if err := json.Unmarshal([]byte(raw), &files); err == nil && len(files) > 0 {
			c.Files = files
			for _, f := range files {
				f.ID = uid()
				for _, r := range f.Reqs {
					r.ID = uid()
				}
			}
			return
		}
	}
	c.Files = seed()
}

func removeAt(list []KV, i int) []KV {
	if i < 0 || i >= len(list) {
		return list
	}
	return append(list[:i], list[i+1:]...)
}

func toggleAt(list []KV, i int) {
	if i >= 0 && i < len(list) {
		list[i].On = !list[i].On
	}
}

func gutterHTML(lines []int) string {
	var b strings.Builder
	for i, n := range lines {
		if i > 0 {
			b.WriteString("<br>")
		}
		b.WriteString(strconv.Itoa(n))
	}
	return b.String()
}

func seed() []*File {
	return []*File{
		ParseHTTP(`@baseUrl = https://api.github.com
@token = ghp_exampletoken

### Current user
GET {{baseUrl}}/user
Authorization: Bearer {{token}}
Accept: application/vnd.github+json

### Open pull requests
GET {{baseUrl}}/repos/golang/go/pulls?state=open&per_page=5
Accept: application/vnd.github+json

### Create issue comment
POST {{baseUrl}}/repos/golang/go/issues/68412/comments
Authorization: Bearer {{token}}
Content-Type: application/json

{
  "body": "Reproduced on go1.23.2 — trace attached."
}
`, "github.http"),
		ParseHTTP(`@auth = https://auth.corp.local

### Client credentials token
POST {{auth}}/oauth2/token
Content-Type: application/x-www-form-urlencoded

grant_type=client_credentials&client_id=hsl-cli&client_secret=s3cret

### JWKS
GET {{auth}}/.well-known/jwks.json
`, "corp-auth.http"),
	}
}
