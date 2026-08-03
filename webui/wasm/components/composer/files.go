//go:build js && wasm

package composer

import (
	_ "embed"
	"html/template"

	"httpStackLens/webui/wasm/dom"
)

//go:embed files.html
var filesHTML string

// FilesPane is the collections sidebar. Keeping it separate from the editor is
// what makes the raw .http editor usable: typing there reparses the file and
// re-renders this pane only, so the textarea never loses its caret.
type FilesPane struct {
	dom.Base
	owner *Composer
}

func (p *FilesPane) Template() string { return filesHTML }

// ── template accessors ─────────────────────────────────────────────────────

func (p *FilesPane) Files() []*File { return p.owner.Files }

func (p *FilesPane) IsActiveFile(f *File) bool { return p.owner.CurFile == f }

func (p *FilesPane) IsActiveReq(r *Request) bool { return p.owner.Cur == r }

func (p *FilesPane) IsDirty(f *File) bool { return p.owner.Dirty[f.ID] }

func (p *FilesPane) MethodColor(method string) template.CSS { return methodColor(method) }

func (p *FilesPane) Folder() string { return "~/HttpStackLens/requests" }

// ── handlers ───────────────────────────────────────────────────────────────

// OpenFile shows the file as text. The caret keeps the disclosure to itself, so
// that clicking the row opens the file rather than merely folding it away.
func (p *FilesPane) OpenFile(e dom.Event) { p.owner.openFile(e.Arg()) }

func (p *FilesPane) ToggleFile(e dom.Event) {
	if f := p.owner.file(e.Arg()); f != nil {
		f.Open = !f.Open
		p.StateHasChanged()
	}
}

func (p *FilesPane) Select(e dom.Event) {
	p.owner.selectRequest(e.ArgOf("file"), e.ArgOf("req"))
}

func (p *FilesPane) NewRequest(e dom.Event) {
	p.owner.newRequest(e.Arg())
}

func (p *FilesPane) NewFile() {
	f := ParseHTTP("@baseUrl = https://api.example.com\n\n### Health check\nGET {{baseUrl}}/health\nAccept: application/json\n", "requests.http")
	p.owner.Files = append(p.owner.Files, f)
	p.owner.Dirty[f.ID] = true
	p.owner.openFile(f.ID)
}

func (p *FilesPane) SaveFile() {
	f := p.owner.CurFile
	if f == nil {
		return
	}
	dom.Download(f.Name, "text/plain", ToHTTP(f))
	delete(p.owner.Dirty, f.ID)
	p.owner.StateHasChanged()
	p.StateHasChanged()
}

func (p *FilesPane) CloseFile() {
	f := p.owner.CurFile
	if f == nil {
		return
	}
	kept := make([]*File, 0, len(p.owner.Files))
	for _, x := range p.owner.Files {
		if x != f {
			kept = append(kept, x)
		}
	}
	p.owner.Files = kept
	delete(p.owner.Dirty, f.ID)
	p.owner.CurFile, p.owner.Cur = nil, nil
	if len(kept) > 0 && len(kept[0].Reqs) > 0 {
		p.owner.openFile(kept[0].ID)
		return
	}
	p.owner.save()
	p.owner.StateHasChanged()
	p.StateHasChanged()
}
