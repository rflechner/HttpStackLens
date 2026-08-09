//go:build js && wasm

package composer

import (
	"regexp"
	"strconv"
	"strings"
)

// Methods is the set of verbs offered in the method dropdown.
var Methods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "QUERY"}

// KV is one header, query parameter or file variable.
type KV struct {
	Key   string
	Value string
	On    bool
}

// Request is a single `###` block inside a .http file.
type Request struct {
	ID   string
	Name string
	// Line is the 1-based line its `###` was read from. It only describes the
	// text ParseHTTP was given, so a caller wanting to point at the editor
	// reparses the draft rather than trusting a request it kept around; it is
	// not part of the file and does not go to storage.
	Line int `json:"-"`
	// Notes holds the comment lines written between the block's title and its
	// request line, verbatim markers included. They are kept rather than parsed
	// because they only have to survive the trip back to text: dropping them, or
	// mistaking a second `###` for the start of another block, is what used to
	// make the file grow a request on every switch between the form and the raw
	// editor.
	Notes   []string
	Method  string
	URL     string
	Headers []KV
	Body    string
}

// Title is the name to display for the request. A block opened by a bare `###`
// separator has no title of its own; the placeholder lives here rather than in
// Name so that serialising the file back does not invent one.
func (r *Request) Title() string {
	if strings.TrimSpace(r.Name) == "" {
		return "Untitled"
	}
	return r.Name
}

// empty reports whether the block holds nothing but its title — a bare `###`
// separator, or a request whose fields have all been cleared. Such a block is
// written back as just that title line.
func (r *Request) empty() bool {
	return r.URL == "" && len(r.Headers) == 0 && strings.TrimSpace(r.Body) == ""
}

// File is one .http file: `@variables` at the top, then requests.
type File struct {
	ID   string
	Name string
	Vars []KV
	Reqs []*Request
	Open bool
}

func (f *File) find(id string) *Request {
	for _, r := range f.Reqs {
		if r.ID == id {
			return r
		}
	}
	return nil
}

var uidCounter int

func uid() string {
	uidCounter++
	return "x" + strconv.Itoa(uidCounter)
}

var (
	varLine     = regexp.MustCompile(`^@([\w.-]+)\s*=\s*(.*)$`)
	reqLine     = regexp.MustCompile(`^([A-Za-z]+)\s+(\S+)(?:\s+HTTP/[\d.]+)?\s*$`)
	headerLine  = regexp.MustCompile(`^([\w.-]+)\s*:\s*(.*)$`)
	placeholder = regexp.MustCompile(`\{\{\s*([\w.-]+)\s*\}\}`)
)

// ParseHTTP reads the .http / .rest format: `@name = value` variables, then one
// `### title` block per request followed by its request line, headers, a blank
// line and a body.
//
// A `###` line only opens a block when the current one already has its request
// line. Consecutive separators are a multi-line comment header — the shape
// http_file_sample.http opens on — not a run of empty requests.
func ParseHTTP(text, name string) *File {
	f := &File{ID: uid(), Name: name, Open: true}
	var cur *Request
	section := "none"

	for n, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, "###") {
			if cur != nil && section == "reqline" {
				cur.Notes = append(cur.Notes, line)
				continue
			}
			cur = &Request{
				ID:     uid(),
				Name:   strings.TrimSpace(strings.TrimLeft(line, "# ")),
				Line:   n + 1,
				Method: "GET",
			}
			f.Reqs = append(f.Reqs, cur)
			section = "reqline"
			continue
		}
		if m := varLine.FindStringSubmatch(line); m != nil && (section == "none" || section == "reqline") {
			f.Vars = append(f.Vars, KV{Key: m[1], Value: strings.TrimSpace(m[2]), On: true})
			continue
		}
		if cur == nil {
			continue
		}
		switch section {
		case "reqline":
			if strings.TrimSpace(line) == "" {
				continue
			}
			if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
				cur.Notes = append(cur.Notes, line)
				continue
			}
			trimmed := strings.TrimSpace(line)
			switch m := reqLine.FindStringSubmatch(line); {
			case m != nil && isMethod(strings.ToUpper(m[1])):
				cur.Method, cur.URL = strings.ToUpper(m[1]), m[2]
			case isMethod(strings.ToUpper(trimmed)):
				// A verb with no target: the request line of a block still being
				// written. Reading it as a target would turn "GET" into "GET GET"
				// on the next round-trip.
				cur.Method = strings.ToUpper(trimmed)
			default:
				cur.URL = trimmed
			}
			section = "headers"
		case "headers":
			if strings.TrimSpace(line) == "" {
				section = "body"
				continue
			}
			if m := headerLine.FindStringSubmatch(line); m != nil {
				cur.Headers = append(cur.Headers, KV{Key: m[1], Value: strings.TrimSpace(m[2]), On: true})
			}
		default:
			if cur.Body != "" {
				cur.Body += "\n"
			}
			cur.Body += line
		}
	}
	for _, r := range f.Reqs {
		r.Body = strings.TrimRight(r.Body, " \t\n")
	}
	return f
}

// ToHTTP serialises a file back to .http text. Disabled rows are dropped, so a
// round-trip through the raw editor is lossy by design — same as the format.
//
// What it must never be is *additive*: ParseHTTP(ToHTTP(f)) has to give the same
// text back, or every switch between the form and the raw editor grows the file.
// That is why an empty block keeps to its title line and a request line with no
// target carries no trailing space.
func ToHTTP(f *File) string {
	var b strings.Builder
	for _, v := range f.Vars {
		if v.On && v.Key != "" {
			b.WriteString("@" + v.Key + " = " + v.Value + "\n")
		}
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	for i, r := range f.Reqs {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(strings.TrimRight("### "+r.Name, " ") + "\n")
		for _, note := range r.Notes {
			b.WriteString(note + "\n")
		}
		if !r.empty() {
			b.WriteString(strings.TrimRight(r.Method+" "+r.URL, " ") + "\n")
		}
		for _, h := range r.Headers {
			if h.On && h.Key != "" {
				b.WriteString(h.Key + ": " + h.Value + "\n")
			}
		}
		if strings.TrimSpace(r.Body) != "" {
			b.WriteString("\n" + r.Body + "\n")
		}
	}
	return b.String()
}

func isMethod(m string) bool {
	for _, x := range Methods {
		if x == m {
			return true
		}
	}
	return false
}

// SplitURL separates the query string into editable rows.
func SplitURL(url string) (base string, params []KV) {
	i := strings.IndexByte(url, '?')
	if i < 0 {
		return url, nil
	}
	for _, p := range strings.Split(url[i+1:], "&") {
		if p == "" {
			continue
		}
		k, v, _ := strings.Cut(p, "=")
		params = append(params, KV{Key: k, Value: v, On: true})
	}
	return url[:i], params
}

// JoinURL rebuilds a URL from a base and its parameter rows.
func JoinURL(base string, params []KV) string {
	var parts []string
	for _, p := range params {
		if p.On && p.Key != "" {
			parts = append(parts, p.Key+"="+p.Value)
		}
	}
	if len(parts) == 0 {
		return base
	}
	return base + "?" + strings.Join(parts, "&")
}

// Interpolate substitutes {{name}} from the file variables, leaving unknown
// placeholders untouched so they stay visible in the response.
func Interpolate(s string, vars []KV) string {
	return placeholder.ReplaceAllStringFunc(s, func(m string) string {
		name := placeholder.FindStringSubmatch(m)[1]
		for _, v := range vars {
			if v.On && v.Key == name {
				return v.Value
			}
		}
		return m
	})
}
