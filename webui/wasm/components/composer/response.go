//go:build js && wasm

package composer

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"

	"httpStackLens/webui/wasm/dom"
)

//go:embed response.html
var responseHTML string

// Result is what a send produced.
type Result struct {
	Status     int
	StatusText string
	Headers    [][2]string
	Body       string
	MS         int
	Err        string
}

// ResponsePane renders the right-hand column. It is a child component so that
// a send re-renders only this pane — the editor keeps its caret and scroll.
type ResponsePane struct {
	dom.Base
	owner *Composer

	Tab  string // body | headers
	Mode string // pretty | raw
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

// ── handlers ───────────────────────────────────────────────────────────────

func (p *ResponsePane) SetTab(e dom.Event) {
	p.Tab = e.Arg()
	p.StateHasChanged()
}

func (p *ResponsePane) SetMode(e dom.Event) {
	p.Mode = e.Arg()
	p.StateHasChanged()
}

func (p *ResponsePane) Copy() {
	if p.owner.Res == nil {
		return
	}
	dom.Clipboard(p.Text())
}
