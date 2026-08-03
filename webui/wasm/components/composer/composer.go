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

	// rawTop and rawLeft outlive a render. Sending a request re-renders the
	// whole subtree, and without them clicking a play button on line 40 would
	// throw the reader back to the top of the file.
	rawTop, rawLeft int
}

// New returns an unmounted composer. Pass it to dom.Mount.
func New() *Composer { return &Composer{} }

func (c *Composer) Template() string { return composerHTML }

// Styles ships the component's CSS, injected once on first mount.
func (c *Composer) Styles() string { return composerCSS }

func (c *Composer) OnInit() {
	// The file itself is the editor: the form is the alternate view, reachable
	// from the "Edit as" switch.
	c.Tab = "raw"
	c.Dirty = map[string]bool{}
	c.load()
	c.files = &FilesPane{owner: c}
	c.resp = &ResponsePane{owner: c}
	c.SetChild("files", c.files)
	c.SetChild("response", c.resp)
	if len(c.Files) > 0 && len(c.Files[0].Reqs) > 0 {
		c.CurFile, c.Cur = c.Files[0], c.Files[0].Reqs[0]
	}
	// The raw tab opens on the first render, and the textarea is filled from
	// Raw: without this the editor would come up empty over a painted overlay.
	c.refreshRaw()
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

// RawText is the .http rendering of the current file. While the raw tab is
// open the draft is the truth, so that edits survive a re-render; elsewhere the
// model is. The tab is what says which, rather than an empty Raw: an emptied
// editor is a legitimate draft, and taking it for "no draft" would leave the
// overlay painting a file the textarea no longer holds.
func (c *Composer) RawText() string {
	if c.Tab == "raw" {
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

// rawLine is one row of the raw editor's gutter: its number, and the index of
// the request that starts there when one does.
type rawLine struct {
	Number  int
	Request int
}

func (l rawLine) runnable() bool { return l.Request >= 0 }

func (c *Composer) rawLines() []rawLine {
	text := c.RawText()
	// The parser's spans carry the line each request was read from, which is
	// what puts a play button on the right row without measuring anything.
	opens := map[int]int{}
	for i, request := range httpfile.ParseHttpFile(text).Requests {
		opens[request.HttpRequestLine.HttpMethod.Start.Line] = i
	}

	lines := make([]rawLine, strings.Count(text, "\n")+1)
	for i := range lines {
		request, ok := opens[i+1]
		if !ok {
			request = -1
		}
		lines[i] = rawLine{Number: i + 1, Request: request}
	}
	return lines
}

// GutterHTML is the line-number column, play buttons included. It is built
// here rather than in the template because a keystroke refreshes it in place:
// a full render would replace the textarea and take the caret with it.
func (c *Composer) GutterHTML() template.HTML {
	var out strings.Builder
	for _, line := range c.rawLines() {
		out.WriteString(`<div class="cx-gl">`)
		if line.runnable() {
			out.WriteString(`<button class="cx-run" data-on-click="RunLine" data-arg-i="`)
			out.WriteString(strconv.Itoa(line.Request))
			out.WriteString(`" title="Send this request">▶</button>`)
		}
		out.WriteString(`<span class="cx-ln">`)
		out.WriteString(strconv.Itoa(line.Number))
		out.WriteString(`</span></div>`)
	}
	return template.HTML(out.String())
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
		c.rawTop, c.rawLeft = 0, 0
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
		gutter.Set("innerHTML", string(c.GutterHTML()))
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
	area.Set("scrollTop", c.rawTop)
	area.Set("scrollLeft", c.rawLeft)
	c.syncRawScroll()
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
	c.rawTop, c.rawLeft = top.Int(), left.Int()
}

// refreshRaw rebuilds the draft after a change made outside the editor — a
// request added, duplicated or deleted. Without it the draft would still show
// the file as it was, and the next keystroke would write that back.
func (c *Composer) refreshRaw() {
	if c.Tab == "raw" && c.CurFile != nil {
		c.Raw = ToHTTP(c.CurFile)
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
	c.refreshRaw()
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
	c.refreshRaw()
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

// outgoing is a request resolved down to what dom.Fetch needs: variables
// substituted, disabled rows dropped.
type outgoing struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    string
}

// Send fires the request the form is showing.
func (c *Composer) Send() {
	if c.Cur == nil || c.Sending {
		return
	}
	if c.Tab == "raw" {
		c.applyRaw()
	}
	c.start(fromForm(c.Cur, c.CurFile.Vars))
}

// RunLine sends the request the clicked play button sits next to. It reads the
// block straight out of the draft rather than out of the form model, so what
// runs is exactly what the editor shows.
func (c *Composer) RunLine(e dom.Event) {
	if c.Sending {
		return
	}
	file := httpfile.ParseHttpFile(c.RawText())
	index := e.ArgIntOf("i")
	if index < 0 || index >= len(file.Requests) {
		return
	}
	c.start(fromBlock(file, file.Requests[index]))
}

// start fires a request. The handler itself must not block — the fetch runs in
// a goroutine so the JS callbacks it waits on can be delivered.
func (c *Composer) start(req outgoing) {
	c.Sending = true
	c.Res = nil
	c.StateHasChanged()
	go c.send(req)
}

func (c *Composer) send(req outgoing) {
	start := time.Now()
	res, err := dom.Fetch(req.Method, req.URL, req.Headers, req.Body)
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

func fromForm(r *Request, vars []KV) outgoing {
	out := outgoing{Method: r.Method, URL: Interpolate(r.URL, vars), Headers: map[string]string{}}
	for _, h := range r.Headers {
		if h.On && h.Key != "" {
			out.Headers[h.Key] = Interpolate(h.Value, vars)
		}
	}
	if r.Method != "GET" && r.Method != "HEAD" {
		out.Body = Interpolate(r.Body, vars)
	}
	return out
}

// fromBlock resolves one block of the shared parser's reading of the draft.
// The target is taken as written — a placeholder target has no host to resolve
// until the file variables are substituted, which happens right here.
func fromBlock(file httpfile.HttpFile, item httpfile.HttpRequestFileItem) outgoing {
	vars := make([]KV, 0, len(file.Variables))
	for _, variable := range file.Variables {
		vars = append(vars, KV{Key: variable.Name.Text, Value: variable.Value.Text, On: true})
	}

	method := string(item.HttpRequestLine.HttpMethod.Text)
	out := outgoing{
		Method:  method,
		URL:     Interpolate(item.HttpRequestLine.Target.Text, vars),
		Headers: map[string]string{},
	}
	for _, header := range item.Headers {
		if header.Name.Text != "" {
			out.Headers[header.Name.Text] = Interpolate(header.Value.Text, vars)
		}
	}
	if method != "GET" && method != "HEAD" {
		out.Body = Interpolate(item.Body.Text, vars)
	}
	return out
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
	previous := c.CurFile
	c.CurFile = f
	c.Cur = f.find(reqID)
	c.Res = nil
	if c.Tab == "raw" {
		// The draft belongs to the file, not to the request: picking another
		// request inside the same file must not rewrite what is being edited.
		if previous != f {
			c.Raw = ToHTTP(f)
			c.rawTop, c.rawLeft = 0, 0
		}
	} else {
		c.Raw = ""
	}
	if c.Tab == "params" && c.Cur != nil {
		_, c.Params = SplitURL(c.Cur.URL)
	}
	c.StateHasChanged()
	c.files.StateHasChanged()
}

// openFile makes a file the one being edited and shows it as text. Choosing a
// .http file means opening the file, not one request inside it: the editor is
// the file, and the form is the alternate view of whatever request is current.
func (c *Composer) openFile(id string) {
	f := c.file(id)
	if f == nil {
		return
	}
	f.Open = true
	c.CurFile = f
	c.Cur = nil
	if len(f.Reqs) > 0 {
		c.Cur = f.Reqs[0]
	}
	c.Res = nil
	c.Tab = "raw"
	c.Raw = ToHTTP(f)
	c.rawTop, c.rawLeft = 0, 0
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
	c.refreshRaw()
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
