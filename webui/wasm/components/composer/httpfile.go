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
	ID      string
	Name    string
	Method  string
	URL     string
	Headers []KV
	Body    string
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
func ParseHTTP(text, name string) *File {
	f := &File{ID: uid(), Name: name, Open: true}
	var cur *Request
	section := "none"

	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, "###") {
			title := strings.TrimSpace(strings.TrimLeft(line, "# "))
			if title == "" {
				title = "Untitled"
			}
			cur = &Request{ID: uid(), Name: title, Method: "GET"}
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
			if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
				continue
			}
			if m := reqLine.FindStringSubmatch(line); m != nil && isMethod(strings.ToUpper(m[1])) {
				cur.Method, cur.URL = strings.ToUpper(m[1]), m[2]
			} else {
				cur.URL = strings.TrimSpace(line)
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
		b.WriteString("### " + r.Name + "\n" + r.Method + " " + r.URL + "\n")
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
