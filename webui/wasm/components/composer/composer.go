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
	"unicode/utf16"

	httpfile "httpStackLens/composer"
	"httpStackLens/webui/wasm/dom"
	"httpStackLens/webui/wasm/shared"
)

//go:embed composer.html
var composerHTML string

//go:embed composer.css
var composerCSS string

// tabKey remembers which of the two editors the composer was left in. It
// belongs next to the title bar's density and theme: which view a developer
// works in is a preference of their session, not a property of the file.
const tabKey = "hsl-composer-tab"

// The editor views. tabRaw edits the .http file as text; the others are the
// panes of the form view, which edits the selected request.
const (
	tabRaw     = "raw"
	tabBody    = "body"
	tabHeaders = "headers"
	tabParams  = "params"
	tabVars    = "vars"
)

// knownTab guards what comes back from storage: an unknown value would leave
// the editor on a branch the template has no case for.
func knownTab(tab string) bool {
	switch tab {
	case tabRaw, tabBody, tabHeaders, tabParams, tabVars:
		return true
	}
	return false
}

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

	// Folder is where the .http files live, as the backend resolved it from
	// config.yaml. It is what the sidebar shows and what the file manager is
	// asked to open.
	Folder string

	// Loading covers the first read of the collection; LoadErr takes its place
	// when that read failed, and SaveErr reports a file that could not be
	// written back.
	Loading bool
	LoadErr string
	SaveErr string

	// saveGen debounces the write-back, one generation per file: every edit
	// bumps it, and a timer that fires holding a stale generation has been
	// overtaken by a later keystroke.
	saveGen map[string]int

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

	// revealLine asks the next render to bring a line of the draft into view.
	// Scrolling straight from the handler would be undone: the textarea it
	// scrolled is replaced by the render that follows.
	revealLine int

	// panes holds the widths the dividers were dragged to, by pane name. A pane
	// missing from the map has never been dragged and keeps the width the
	// stylesheet gives it.
	panes map[string]int

	// dragMove and dragUp are the listeners of a divider being dragged. They go
	// on the document, not on the 5px grip, which the pointer leaves behind at
	// the first quick move.
	dragMove, dragUp js.Func
}

// The two draggable panes, named by the grips' data-arg and by the suffix of
// the key their width is stored under.
const (
	paneSide = "side"
	paneRes  = "response"
	paneKey  = "hsl-composer-w-"
)

// Pane bounds, ported from the mockup. The response column is capped against
// the room its own row has, so that the editor keeps editorMin whatever the
// window does.
const (
	sideMin, sideMax = 190, 560
	resMin, resFloor = 280, 320
	editorMin        = 380
)

// New returns an unmounted composer. Pass it to dom.Mount.
func New() *Composer { return &Composer{} }

func (c *Composer) Template() string { return composerHTML }

// Styles ships the component's CSS, injected once on first mount.
func (c *Composer) Styles() string { return composerCSS }

func (c *Composer) OnInit() {
	// The file itself is the editor: the form is the alternate view, reachable
	// from the "Edit as" switch. Whichever the window was last left in wins.
	c.Tab = tabRaw
	if tab := dom.LocalGet(tabKey); knownTab(tab) {
		c.Tab = tab
	}
	c.Dirty = map[string]bool{}
	c.saveGen = map[string]int{}
	c.Loading = true
	c.loadPanes()
	c.files = &FilesPane{owner: c}
	c.resp = &ResponsePane{owner: c}
	c.SetChild("files", c.files)
	c.SetChild("response", c.resp)
	// The collection is on disk, behind the backend, so the first render shows
	// the loading state and the files land on the one after it. The fetch has
	// to run in a goroutine: it blocks, and OnInit runs on the JS thread the
	// callbacks it waits for are delivered on.
	go c.loadFiles()
}

// ── template accessors ─────────────────────────────────────────────────────

func (c *Composer) MethodList() []string { return Methods }

func (c *Composer) TabIs(id string) bool { return c.Tab == id }

// Editing says whether the main column shows an editor rather than the empty
// state. The raw view edits the file, so it only needs a file open; the form
// view edits one request and needs that request selected.
func (c *Composer) Editing() bool {
	return c.CurFile != nil && (c.Tab == tabRaw || c.Cur != nil)
}

func (c *Composer) MethodColor() template.CSS {
	if c.Cur == nil {
		return methodColor("")
	}
	return methodColor(c.Cur.Method)
}

// SaveState is the one word the toolbar says about the folder: nothing while
// the file on disk is up to date, "unsaved" between a keystroke and the
// write-back, and the reason when that write failed.
func (c *Composer) SaveState() string {
	if c.SaveErr != "" {
		return c.SaveErr
	}
	if c.CurFile != nil && c.Dirty[c.CurFile.ID] {
		return "unsaved"
	}
	return ""
}

// refreshSaveState pokes the indicator without a render. A write-back finishes
// while the developer is still typing, and re-rendering the editor there would
// replace the textarea under the caret.
func (c *Composer) refreshSaveState() {
	node := c.Ref("saveState")
	if !node.Truthy() {
		return
	}
	node.Set("textContent", c.SaveState())
	node.Get("classList").Call("toggle", "err", c.SaveErr != "")
}

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
	if c.Tab == tabRaw {
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
	if !knownTab(next) {
		return
	}
	if c.Tab == tabRaw && next != tabRaw {
		c.applyRaw()
	}
	if next == tabRaw {
		if c.CurFile == nil {
			return
		}
		c.Raw = ToHTTP(c.CurFile)
		c.rawTop, c.rawLeft = 0, 0
	} else {
		c.Raw = ""
	}
	if next == tabParams && c.Cur != nil {
		_, c.Params = SplitURL(c.Cur.URL)
	}
	c.Tab = next
	dom.LocalSet(tabKey, next)
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
	c.applyPaneWidths()
	c.releaseRawScroll()
	area := c.Ref("rawArea")
	if !area.Truthy() {
		return
	}
	area.Set("scrollTop", c.rawTop)
	area.Set("scrollLeft", c.rawLeft)
	if line := c.revealLine; line > 0 {
		c.revealLine = 0
		revealRawLine(area, line)
	}
	c.syncRawScroll()
	c.rawScroll = js.FuncOf(func(js.Value, []js.Value) any {
		c.syncRawScroll()
		return nil
	})
	area.Call("addEventListener", "scroll", c.rawScroll)
}

func (c *Composer) OnDispose() {
	c.releaseRawScroll()
	c.releaseDrag()
}

func (c *Composer) releaseRawScroll() {
	if c.rawScroll.Truthy() {
		c.rawScroll.Release()
		c.rawScroll = js.Func{}
	}
}

// A textarea counts its selection in UTF-16 code units. Go indexes strings in
// bytes, and the two part ways at the first non-ASCII character — an accented
// word in a body, the em dash in a comment. The pair below converts, so that a
// caret lands on the line it was asked for whatever the file holds.
//
// Neither goes through JS: syscall/js refuses Call and Get on a string, which
// is what `textarea.value` hands back.

// utf16LineOffset is the selection offset of the start of a 1-based line.
func utf16LineOffset(text string, line int) int {
	start := 0
	for i := 1; i < line; i++ {
		next := strings.IndexByte(text[start:], '\n')
		if next < 0 {
			start = len(text)
			break
		}
		start += next + 1
	}
	return len(utf16.Encode([]rune(text[:start])))
}

// utf16LineAt is the 1-based line a selection offset falls on.
func utf16LineAt(text string, offset int) int {
	line, units := 1, 0
	for _, r := range text {
		if units >= offset {
			break
		}
		if size := utf16.RuneLen(r); size > 0 {
			units += size
		} else {
			units++
		}
		if r == '\n' {
			line++
		}
	}
	return line
}

// revealRawLine puts the caret at the start of a line of the draft and scrolls
// it into view, a couple of lines below the top edge so the block's comment
// header stays readable. The line height is read from the stylesheet rather
// than assumed: the density switch in the title bar changes it.
func revealRawLine(area js.Value, line int) {
	offset := utf16LineOffset(area.Get("value").String(), line)
	area.Call("focus")
	area.Call("setSelectionRange", offset, offset)

	height := js.Global().Call("getComputedStyle", area).Get("lineHeight").String()
	pixels, err := strconv.ParseFloat(strings.TrimSuffix(height, "px"), 64)
	if err != nil || pixels <= 0 { // "normal", left unresolved by the browser
		return
	}
	const context = 2 // lines kept visible above the target
	top := (float64(line-1) - context) * pixels
	if top < 0 {
		top = 0
	}
	area.Set("scrollTop", int(top))
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

// ── resizable panes ────────────────────────────────────────────────────────

// StartResize begins dragging one of the two dividers.
func (c *Composer) StartResize(e dom.Event) {
	pane := e.Arg()
	node := c.Ref(pane)
	if !node.Truthy() {
		return
	}
	e.PreventDefault()
	c.releaseDrag()

	startX := e.Raw.Get("clientX").Float()
	startWidth := node.Get("offsetWidth").Float()

	grip := e.Node
	grip.Get("classList").Call("add", "on")
	document := js.Global().Get("document")
	body := document.Get("body").Get("style")
	body.Set("cursor", "col-resize")
	// Without this the drag selects the text it passes over.
	body.Set("userSelect", "none")

	c.dragMove = js.FuncOf(func(_ js.Value, args []js.Value) any {
		delta := args[0].Get("clientX").Float() - startX
		if pane == paneRes {
			delta = -delta // the response column grows as the pointer moves left
		}
		c.setPaneWidth(pane, node, int(startWidth+delta))
		return nil
	})
	c.dragUp = js.FuncOf(func(js.Value, []js.Value) any {
		document.Call("removeEventListener", "pointermove", c.dragMove)
		document.Call("removeEventListener", "pointerup", c.dragUp)
		grip.Get("classList").Call("remove", "on")
		body.Set("cursor", "")
		body.Set("userSelect", "")
		c.savePane(pane)
		c.releaseDrag()
		return nil
	})
	document.Call("addEventListener", "pointermove", c.dragMove)
	document.Call("addEventListener", "pointerup", c.dragUp)
}

// ResetPane gives a pane back the width the stylesheet gives it. Clearing the
// inline width rather than restoring a number keeps the stylesheet the one
// place a pane's natural size is written.
func (c *Composer) ResetPane(e dom.Event) {
	pane := e.Arg()
	if node := c.Ref(pane); node.Truthy() {
		node.Get("style").Set("width", "")
	}
	delete(c.panes, pane)
	dom.LocalSet(paneKey+pane, "")
}

// setPaneWidth writes the width straight to the DOM. Rendering on every pointer
// move would replace the subtree — and the editor's caret with it — dozens of
// times a second.
func (c *Composer) setPaneWidth(pane string, node js.Value, width int) {
	min, max := c.paneBounds(pane)
	width = clamp(width, min, max)
	node.Get("style").Set("width", strconv.Itoa(width)+"px")
	c.panes[pane] = width
}

func (c *Composer) paneBounds(pane string) (min, max int) {
	if pane == paneSide {
		return sideMin, sideMax
	}
	max = resFloor
	if main := c.Ref("main"); main.Truthy() {
		if room := main.Get("offsetWidth").Int() - editorMin; room > max {
			max = room
		}
	}
	return resMin, max
}

// applyPaneWidths re-applies the dragged widths after a render, which replaces
// the very nodes carrying them.
func (c *Composer) applyPaneWidths() {
	for _, pane := range []string{paneSide, paneRes} {
		width, dragged := c.panes[pane]
		if !dragged {
			continue
		}
		if node := c.Ref(pane); node.Truthy() {
			node.Get("style").Set("width", strconv.Itoa(width)+"px")
		}
	}
}

func (c *Composer) savePane(pane string) {
	if width, dragged := c.panes[pane]; dragged {
		dom.LocalSet(paneKey+pane, strconv.Itoa(width))
	}
}

// loadPanes restores the widths the window was left with. A stored width below
// the pane's minimum is dropped rather than clamped: it is a leftover from a
// narrower build, not a choice.
func (c *Composer) loadPanes() {
	c.panes = map[string]int{}
	for _, pane := range []string{paneSide, paneRes} {
		min := sideMin
		if pane == paneRes {
			min = resMin
		}
		if width, err := strconv.Atoi(dom.LocalGet(paneKey + pane)); err == nil && width >= min {
			c.panes[pane] = width
		}
	}
}

func (c *Composer) releaseDrag() {
	if c.dragMove.Truthy() {
		c.dragMove.Release()
		c.dragMove = js.Func{}
	}
	if c.dragUp.Truthy() {
		c.dragUp.Release()
		c.dragUp = js.Func{}
	}
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// refreshRaw rebuilds the draft after a change made outside the editor — a
// request added, duplicated or deleted. Without it the draft would still show
// the file as it was, and the next keystroke would write that back.
func (c *Composer) refreshRaw() {
	if c.Tab == tabRaw && c.CurFile != nil {
		c.Raw = ToHTTP(c.CurFile)
	}
}

// OnKey wires ⌘↵. In the form view there is one request to send; in the raw
// view the shortcut has to pick, and it picks the block the caret sits in — the
// same request the gutter's play button on that row would run.
func (c *Composer) OnKey(e dom.Event) {
	if !e.Meta() || e.Key() != "Enter" {
		return
	}
	e.PreventDefault()
	if c.Tab == tabRaw {
		c.RunCaret()
		return
	}
	c.Send()
}

// RunCaret sends the request the caret is inside. A caret above the first block
// — in the file's @variables — is not inside any request, and nothing happens.
func (c *Composer) RunCaret() {
	if c.Sending {
		return
	}
	area := c.Ref("rawArea")
	if !area.Truthy() {
		return
	}
	line := utf16LineAt(area.Get("value").String(), area.Get("selectionStart").Int())

	file := httpfile.ParseHttpFile(c.RawText())
	index := -1
	for i, request := range file.Requests {
		if blockStartLine(request) <= line {
			index = i
		}
	}
	if index < 0 {
		return
	}
	c.start(fromBlock(file, file.Requests[index]))
}

// blockStartLine is the first line a request block owns. The comment header
// counts: a caret on the "### title" line is inside that request, which is
// where it lands after clicking the request in the sidebar.
func blockStartLine(request httpfile.HttpRequestFileItem) int {
	if len(request.HeaderComments) > 0 {
		return request.HeaderComments[0].Start.Line
	}
	return request.HttpRequestLine.HttpMethod.Start.Line
}

func (c *Composer) NewRequest() { c.newRequest(c.fileID()) }

func (c *Composer) DuplicateRequest() {
	if c.Cur == nil {
		return
	}
	clone := *c.Cur
	clone.ID = uid()
	clone.Name = c.Cur.Title() + " copy"
	clone.Notes = append([]string(nil), c.Cur.Notes...)
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
	if c.Tab == tabRaw {
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
	c.Sending = false
	c.Res = exchange(req)
	c.resp.Tab = "body"
	c.StateHasChanged()
}

// sendPath is the backend endpoint that replays a composer request through the
// proxy pipeline.
const sendPath = "/api/composer/send"

// exchange hands the request to the backend instead of issuing it from the
// browser: the request then goes through the same pipeline as any client of the
// proxy — upstream proxy, no_proxy rules and HTTPS decryption included — and is
// not subject to CORS.
func exchange(req outgoing) *Result {
	request := shared.ComposerRequestDto{Method: req.Method, Url: req.URL, Body: req.Body}
	for name, value := range req.Headers {
		request.Headers = append(request.Headers, shared.HeaderDto{Name: name, Value: value})
	}

	payload, err := json.Marshal(request)
	if err != nil {
		return &Result{Err: err.Error()}
	}

	res, err := dom.Fetch("POST", sendPath,
		map[string]string{"Content-Type": "application/json"}, string(payload))
	if err != nil {
		return &Result{Err: "could not reach HttpStackLens: " + err.Error()}
	}
	if res.Status != 200 {
		return &Result{Err: strings.TrimSpace(res.Body)}
	}

	var dto shared.ComposerResponseDto
	if err := json.Unmarshal([]byte(res.Body), &dto); err != nil {
		return &Result{Err: "unreadable answer from HttpStackLens: " + err.Error()}
	}

	out := &Result{
		Status:     dto.Status,
		StatusText: dto.StatusText,
		Proto:      dto.Proto,
		Body:       dto.Body,
		MS:         dto.DurationMs,
		Truncated:  dto.Truncated,
		Upstream:   dto.Upstream,
		Err:        dto.Error,
	}
	for _, header := range dto.Headers {
		out.Headers = append(out.Headers, [2]string{header.Name, header.Value})
	}
	return out
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

func indexOfRequest(list []*Request, r *Request) int {
	for i, x := range list {
		if x == r {
			return i
		}
	}
	return -1
}

// lineOfRequest locates the nth block of a draft. The text is reparsed rather
// than trusting the Line a request carries, because the model it belongs to may
// have been built from an older version of that text.
func lineOfRequest(raw string, index int) int {
	if index < 0 {
		return 0
	}
	parsed := ParseHTTP(raw, "")
	if index >= len(parsed.Reqs) {
		return 0
	}
	return parsed.Reqs[index].Line
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
	if c.Tab == tabRaw {
		// The draft belongs to the file, not to the request: picking another
		// request inside the same file must not rewrite what is being edited.
		if previous != f {
			c.Raw = ToHTTP(f)
			c.rawTop, c.rawLeft = 0, 0
		}
		// Selecting has no form to fill here, so it navigates instead: the
		// editor jumps to the block, which is the only thing picking a request
		// can usefully mean while the file is the editor.
		c.revealLine = lineOfRequest(c.Raw, indexOfRequest(f.Reqs, c.Cur))
	} else {
		c.Raw = ""
	}
	if c.Tab == tabParams && c.Cur != nil {
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
	c.Tab = tabRaw
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

// touch marks the current file as edited and schedules the write-back. Every
// edit reaches the folder on its own: the composer is a scratchpad, and asking
// a developer to remember a save button between two sends is the kind of step
// that loses work.
func (c *Composer) touch() {
	if c.CurFile == nil {
		return
	}
	c.Dirty[c.CurFile.ID] = true
	c.refreshSaveState()
	c.scheduleSave(c.CurFile)
}

// autosaveDelay is how long the write-back waits after the last keystroke. Long
// enough that typing a URL is one write rather than thirty, short enough that
// the file on disk is what an IDE opened next to the composer would show.
const autosaveDelay = 600 * time.Millisecond

func (c *Composer) scheduleSave(f *File) {
	c.saveGen[f.ID]++
	generation := c.saveGen[f.ID]
	time.AfterFunc(autosaveDelay, func() {
		if c.saveGen[f.ID] != generation {
			return
		}
		c.persist(f)
	})
}

// saveNow writes a file back without waiting for the debounce — what the Save
// button does, and what closing the draft on a file needs.
func (c *Composer) saveNow(f *File) {
	if f == nil {
		return
	}
	c.saveGen[f.ID]++
	go c.persist(f)
}

// persist writes one file to its folder. Only the sidebar and the save
// indicator are refreshed on the way out: a full re-render would replace the
// textarea the developer is still typing into, caret and all.
func (c *Composer) persist(f *File) {
	content := ToHTTP(f)
	if c.CurFile == f && c.Tab == tabRaw {
		content = c.Raw
	}
	if err := putHttpFile(f.Name, content); err != nil {
		c.SaveErr = "could not save " + f.Name + ": " + err.Error()
	} else {
		c.SaveErr = ""
		delete(c.Dirty, f.ID)
	}
	c.refreshSaveState()
	c.files.refresh()
}

// forget drops a file from the model after it has left the folder, and moves
// the editor onto whatever is left.
func (c *Composer) forget(f *File) {
	kept := make([]*File, 0, len(c.Files))
	for _, x := range c.Files {
		if x != f {
			kept = append(kept, x)
		}
	}
	c.Files = kept
	delete(c.Dirty, f.ID)
	delete(c.saveGen, f.ID)
	if c.CurFile == f {
		c.CurFile, c.Cur, c.Res = nil, nil, nil
		c.Raw = ""
		if len(kept) > 0 {
			c.openFile(kept[0].ID)
			return
		}
	}
	c.StateHasChanged()
	c.files.StateHasChanged()
}

// loadFiles reads the collection the backend keeps on disk. A collection still
// held in the browser from an earlier version is carried over on the way, so
// that upgrading does not look like losing every request.
func (c *Composer) loadFiles() {
	dto, err := fetchHttpFiles()
	if err != nil {
		c.Loading = false
		c.LoadErr = err.Error()
		c.StateHasChanged()
		c.files.StateHasChanged()
		return
	}
	if len(dto.Files) == 0 {
		dto.Files = migrateLegacyFiles()
	}

	files := make([]*File, 0, len(dto.Files))
	for _, file := range dto.Files {
		files = append(files, ParseHTTP(file.Content, file.Name))
	}

	c.Loading = false
	c.LoadErr = ""
	c.Folder = dto.Folder
	c.Files = files
	c.CurFile, c.Cur = nil, nil
	if len(files) > 0 {
		files[0].Open = true
		c.CurFile = files[0]
		if len(files[0].Reqs) > 0 {
			c.Cur = files[0].Reqs[0]
		}
	}
	if c.Tab == tabParams && c.Cur != nil {
		_, c.Params = SplitURL(c.Cur.URL)
	}
	// The raw tab opens on the first render, and the textarea is filled from
	// Raw: without this the editor would come up empty over a painted overlay.
	c.refreshRaw()
	c.StateHasChanged()
	c.files.StateHasChanged()
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
