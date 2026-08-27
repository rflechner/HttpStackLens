//go:build js && wasm

package composer

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"strconv"
	"strings"

	"httpStackLens/webui/wasm/dom"
)

//go:embed response.html
var responseHTML string

// Result is what a send produced, as reported by the backend.
type Result struct {
	Status     int
	StatusText string
	// Proto is the version the response came back on, as the backend read it.
	Proto   string
	Headers [][2]string
	Body    string
	MS      int
	// Truncated reports that the body was cut at the display limit.
	Truncated bool
	// Upstream is the outbound proxy the request went through, empty when the
	// connection was direct.
	Upstream string
	Err      string
}

// ResponsePane renders the right-hand column. It is a child component so that
// a send re-renders only this pane — the editor keeps its caret and scroll.
type ResponsePane struct {
	dom.Base
	owner *Composer

	Tab  string // body | headers | raw
	Mode string // pretty | raw, on the body tab only
}

func (p *ResponsePane) Template() string { return responseHTML }

func (p *ResponsePane) OnInit() {
	p.Tab, p.Mode = "body", "pretty"
}

// ── template accessors ─────────────────────────────────────────────────────

func (p *ResponsePane) Res() *Result          { return p.owner.Res }
func (p *ResponsePane) Sending() bool         { return p.owner.Sending }
func (p *ResponsePane) TabIs(id string) bool  { return p.Tab == id }
func (p *ResponsePane) ModeIs(id string) bool { return p.Mode == id }

func (p *ResponsePane) StatusColor() template.CSS {
	s := p.owner.Res.Status
	switch {
	case s >= 500:
		return "var(--danger)"
	case s >= 400:
		return "var(--warn)"
	case s >= 300:
		return "var(--info)"
	case s >= 200:
		return "var(--mint)"
	default:
		return "var(--dim)"
	}
}

func (p *ResponsePane) Size() string {
	b := len(p.owner.Res.Body)
	switch {
	case b < 1024:
		return fmt.Sprintf("%d B", b)
	case b < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	default:
		return fmt.Sprintf("%.2f MB", float64(b)/(1024*1024))
	}
}

// Text is the body as displayed: pretty-printed when it parses as JSON and the
// pretty toggle is on, raw otherwise.
func (p *ResponsePane) Text() string {
	body := p.owner.Res.Body
	if p.Mode != "pretty" {
		return body
	}
	var out bytes.Buffer
	if err := json.Indent(&out, []byte(body), "", "  "); err != nil {
		return body
	}
	return out.String()
}

// defaultProto is the status line's version when the backend reported none —
// an old build, or the error path, which never read a response line.
const defaultProto = "HTTP/1.1"

// RawProtocol renders the response the way it reads on the wire: status line,
// headers, a blank line, then the body. It is the view to reach for when the
// question is about the exchange rather than the payload — a redirect chain, a
// cache header, a content type that does not match the body — and it is what
// pastes into a bug report as one piece.
//
// It is a rendering, not a capture. The header order the server used is already
// lost by the time the backend flattens Go's header map, so they come out
// sorted, and a body cut at the size limit is shown cut — the "truncated" badge
// above says so.
func (p *ResponsePane) RawProtocol() string {
	r := p.owner.Res
	if r == nil {
		return ""
	}

	proto := r.Proto
	if proto == "" {
		proto = defaultProto
	}
	var out strings.Builder
	out.WriteString(proto)
	out.WriteString(" ")
	out.WriteString(strconv.Itoa(r.Status))
	if r.StatusText != "" {
		out.WriteString(" ")
		out.WriteString(r.StatusText)
	}
	out.WriteString("\n")

	for _, header := range r.Headers {
		out.WriteString(header[0])
		out.WriteString(": ")
		out.WriteString(header[1])
		out.WriteString("\n")
	}

	// The blank line belongs to the message, so it is written whether or not a
	// body follows: a bodiless response ends with it, and that is the shape a
	// reader is checking for.
	out.WriteString("\n")
	out.WriteString(r.Body)
	return out.String()
}

// ── handlers ───────────────────────────────────────────────────────────────

func (p *ResponsePane) SetTab(e dom.Event) {
	p.Tab = e.Arg()
	p.StateHasChanged()
}

func (p *ResponsePane) SetMode(e dom.Event) {
	p.Mode = e.Arg()
	p.StateHasChanged()
}

// Copy takes what is on screen: the whole message from the raw tab, the body as
// displayed from the other two. Copying a pretty-printed body while the raw
// view is open would be the wrong text every time.
func (p *ResponsePane) Copy() {
	if p.owner.Res == nil {
		return
	}
	if p.Tab == "raw" {
		dom.Clipboard(p.RawProtocol())
		return
	}
	dom.Clipboard(p.Text())
}
