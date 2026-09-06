//go:build js && wasm

package composer

import (
	_ "embed"
	"html/template"
	"strings"

	"httpStackLens/webui/wasm/dom"
)

//go:embed files.html
var filesHTML string

// namingNew is what naming holds while the field is about to create a file
// rather than rename one. It cannot collide with a file ID: those are decimal.
const namingNew = "+"

// newFileContent is what a freshly created .http file holds. A file with a
// variable and one request is a starting point to edit; an empty one is a blank
// page with no hint of the syntax.
const newFileContent = "@baseUrl = https://api.example.com\n\n### Health check\nGET {{baseUrl}}/health\nAccept: application/json\n"

// FilesPane is the collections sidebar. Keeping it separate from the editor is
// what makes the raw .http editor usable: typing there reparses the file and
// re-renders this pane only, so the textarea never loses its caret.
type FilesPane struct {
	dom.Base
	owner *Composer

	// naming says what the name field at the top of the list is for: empty when
	// it is closed, namingNew while a file is being created, or the ID of the
	// file being renamed.
	naming string

	// nameDraft only seeds the field on the render that opens it. It is not kept
	// in step with what is typed — reading the field back on confirm is what
	// spares this pane a re-render under the caret on every keystroke.
	nameDraft string
	nameErr   string

	// busy covers a create, rename or delete in flight, so a second Enter does
	// not start the same call twice.
	busy bool

	// footErr reports a folder that would not open. It belongs next to the path
	// it is about, which is where it is shown.
	footErr string
}

func (p *FilesPane) Template() string { return filesHTML }

// ── template accessors ─────────────────────────────────────────────────────

func (p *FilesPane) Files() []*File { return p.owner.Files }

func (p *FilesPane) IsActiveFile(f *File) bool { return p.owner.CurFile == f }

func (p *FilesPane) IsActiveReq(r *Request) bool { return p.owner.Cur == r }

func (p *FilesPane) IsDirty(f *File) bool { return p.owner.Dirty[f.ID] }

func (p *FilesPane) MethodColor(method string) template.CSS { return methodColor(method) }

func (p *FilesPane) Loading() bool { return p.owner.Loading }

func (p *FilesPane) LoadErr() string { return p.owner.LoadErr }

// Folder is where the files are read from and written back to, as config.yaml
// has it. It stays empty until the backend answers, and the sidebar says so
// rather than showing a path that is only a guess.
func (p *FilesPane) Folder() string { return p.owner.Folder }

func (p *FilesPane) FootErr() string { return p.footErr }

// Naming reports whether the name field is open, so the list can make room for
// it; NamingNew tells the field whether it names a new file or renames one.
func (p *FilesPane) Naming() bool { return p.naming != "" }

func (p *FilesPane) NamingNew() bool { return p.naming == namingNew }

func (p *FilesPane) NameDraft() string { return p.nameDraft }

func (p *FilesPane) NameErr() string { return p.nameErr }

func (p *FilesPane) Busy() bool { return p.busy }

// BusyAttrs disables the field while a call is in flight — the same trick the
// send button uses, since a bare `disabled` cannot come from a template action.
func (p *FilesPane) BusyAttrs() template.HTMLAttr {
	if p.busy {
		return "disabled"
	}
	return ""
}

// OnAfterRender puts the caret in the name field the render just opened. The
// name is selected rather than appended to: renaming usually replaces it.
func (p *FilesPane) OnAfterRender(bool) {
	if p.naming == "" || p.busy {
		return
	}
	if input := p.Ref("nameInput"); input.Truthy() {
		input.Call("focus")
		input.Call("select")
	}
}

// refresh re-renders the pane unless the name field is open — a render would
// replace the input being typed into. It is what the write-back calls, which
// can land at any moment.
func (p *FilesPane) refresh() {
	if p.naming != "" {
		return
	}
	p.StateHasChanged()
}

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

// NewFile opens the name field rather than inventing a name: the file is about
// to exist on disk under whatever it is called, and requests.http, requests
// (1).http, requests (2).http is not a collection anyone can read later.
func (p *FilesPane) NewFile() {
	p.naming, p.nameDraft, p.nameErr = namingNew, "", ""
	p.StateHasChanged()
}

// RenameFile opens the same field on an existing file.
func (p *FilesPane) RenameFile(e dom.Event) {
	f := p.owner.file(e.Arg())
	if f == nil {
		return
	}
	p.naming, p.nameDraft, p.nameErr = f.ID, f.Name, ""
	p.StateHasChanged()
}

func (p *FilesPane) CancelName() {
	p.naming, p.nameDraft, p.nameErr = "", "", ""
	p.StateHasChanged()
}

// NameKey makes the field behave like a field: Enter confirms, Escape gives up.
func (p *FilesPane) NameKey(e dom.Event) {
	switch e.Key() {
	case "Enter":
		e.PreventDefault()
		p.ConfirmName()
	case "Escape":
		e.PreventDefault()
		p.CancelName()
	}
}

// ConfirmName validates what was typed before anything reaches the disk. The
// backend checks the same rules — this pass is what lets the field answer a bad
// name without a round trip.
func (p *FilesPane) ConfirmName() {
	if p.busy || p.naming == "" {
		return
	}
	name, err := FileName(p.nameValue())
	if err != nil {
		p.nameErr = err.Error()
		p.StateHasChanged()
		return
	}
	if p.nameTaken(name) {
		p.nameErr = name + " already exists in this folder"
		p.StateHasChanged()
		return
	}

	p.busy, p.nameDraft, p.nameErr = true, name, ""
	p.StateHasChanged()
	go p.applyName(name)
}

// SaveFile writes the current file now instead of waiting for the debounce.
// Editing already reaches the folder on its own; this is for the moment before
// switching to a terminal, when "did it land?" is worth an answer.
func (p *FilesPane) SaveFile() {
	p.owner.saveNow(p.owner.CurFile)
}

// DeleteFile removes the file from the folder. It asks first: this is the one
// action in the composer that destroys something outside the app.
func (p *FilesPane) DeleteFile() {
	f := p.owner.CurFile
	if f == nil || p.busy {
		return
	}
	if !dom.Confirm("Delete " + f.Name + " from the .http folder?\n\nThe file is removed from disk.") {
		return
	}
	p.busy = true
	p.StateHasChanged()
	go p.deleteFile(f)
}

// OpenFolder shows the folder in the file manager of the machine HttpStackLens
// runs on — the developer's own, since this is a local tool.
func (p *FilesPane) OpenFolder() {
	if p.owner.Folder == "" {
		return
	}
	go func() {
		if err := openHttpFolder(); err != nil {
			p.footErr = err.Error()
		} else {
			p.footErr = ""
		}
		p.StateHasChanged()
	}()
}

// ── the work behind the handlers ───────────────────────────────────────────

// applyName creates or renames, then puts the pane back the way it was. It runs
// in a goroutine: the calls it makes block on the backend.
func (p *FilesPane) applyName(name string) {
	if p.naming == namingNew {
		f := ParseHTTP(newFileContent, name)
		if err := putHttpFile(name, ToHTTP(f)); err != nil {
			p.failName(err)
			return
		}
		p.owner.Files = append(p.owner.Files, f)
		p.closeName()
		p.owner.openFile(f.ID)
		return
	}

	f := p.owner.file(p.naming)
	if f == nil {
		p.closeName()
		return
	}
	if err := renameHttpFile(f.Name, name); err != nil {
		p.failName(err)
		return
	}
	f.Name = name
	p.closeName()
	// The name is on the editor's toolbar and its tab as well as in the list.
	p.owner.StateHasChanged()
}

func (p *FilesPane) deleteFile(f *File) {
	if err := deleteHttpFile(f.Name); err != nil {
		p.busy = false
		p.footErr = err.Error()
		p.StateHasChanged()
		return
	}
	p.busy, p.footErr = false, ""
	p.owner.forget(f)
}

func (p *FilesPane) closeName() {
	p.naming, p.nameDraft, p.nameErr, p.busy = "", "", "", false
	p.StateHasChanged()
}

func (p *FilesPane) failName(err error) {
	p.busy = false
	p.nameErr = err.Error()
	p.StateHasChanged()
}

// nameValue reads the field rather than a bound property, which is what keeps
// the pane from re-rendering on every keystroke.
func (p *FilesPane) nameValue() string {
	input := p.Ref("nameInput")
	if !input.Truthy() {
		return p.nameDraft
	}
	return input.Get("value").String()
}

// nameTaken reports a collision with another file. The file being renamed is
// not one: keeping its own name, or changing only its case, is not a clash.
func (p *FilesPane) nameTaken(name string) bool {
	for _, f := range p.owner.Files {
		if f.ID == p.naming {
			continue
		}
		if strings.EqualFold(f.Name, name) {
			return true
		}
	}
	return false
}
