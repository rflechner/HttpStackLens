package composer

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"httpStackLens/http/models"
)

// RequestBlock is the editable reading of one request block: the values the
// composer form shows, without positions. Title is the text of the "###" line
// that opens the block, empty for a bare separator.
type RequestBlock struct {
	Title   string
	Method  string
	Target  string
	Headers []HeaderLine
	Body    string
}

type HeaderLine struct {
	Name  string
	Value string
}

// ReadRequests lists the request blocks of a file, in the order the composer
// and its play button number them.
func ReadRequests(source string) []RequestBlock {
	runes := []rune(source)
	file := ParseHttpFile(source)
	blocks := make([]RequestBlock, 0, len(file.Requests))
	for _, item := range file.Requests {
		blocks = append(blocks, readBlock(runes, item))
	}
	return blocks
}

func readBlock(runes []rune, item HttpRequestFileItem) RequestBlock {
	block := RequestBlock{
		Method: string(item.HttpRequestLine.HttpMethod.Text),
		Target: item.HttpRequestLine.Target.Text,
		Body:   item.Body.Text,
	}
	if title, ok := titleComment(runes, item); ok {
		block.Title = titleText(title)
	}
	for _, header := range item.Headers {
		block.Headers = append(block.Headers, HeaderLine{Name: header.Name.Text, Value: header.Value.Text})
	}
	return block
}

// AppendRequest adds a block at the end of the file and returns the new source
// and the index of the block. A bare "###" left dangling at the end of the file
// is reused rather than kept: the Web UI would otherwise read it as the title
// line of the new request.
func AppendRequest(source string, block RequestBlock) (string, int, error) {
	block, err := normalizeBlock(block)
	if err != nil {
		return "", 0, err
	}
	before := ParseHttpFile(source)

	kept := strings.TrimRight(source, " \t\r\n")
	if last := kept[strings.LastIndex(kept, "\n")+1:]; strings.TrimSpace(last) == separator {
		kept = strings.TrimRight(strings.TrimSuffix(kept, last), " \t\r\n")
	}
	if kept != "" {
		kept += "\n\n"
	}
	updated := kept + writeBlock(block, nil, nil, "")

	index := len(before.Requests)
	if err := checkRewrite(updated, index, len(before.Requests)+1, block); err != nil {
		return "", 0, err
	}
	return updated, index, nil
}

// ReplaceRequest rewrites the block at index with change applied to its current
// reading. The comment lines of the block survive the rewrite: those above the
// request line stay there, those among the headers — the headers the Web UI
// form unticked, among them — are written after the headers. Everything outside
// the block is left as it was.
func ReplaceRequest(source string, index int, change func(RequestBlock) (RequestBlock, error)) (string, error) {
	runes := []rune(source)
	file := ParseHttpFile(source)
	if index < 0 || index >= len(file.Requests) {
		return "", fmt.Errorf("no request at index %d: the file holds %d", index, len(file.Requests))
	}
	item := file.Requests[index]

	block, err := change(readBlock(runes, item))
	if err != nil {
		return "", err
	}
	block, err = normalizeBlock(block)
	if err != nil {
		return "", err
	}

	lineStart := item.HttpRequestLine.HttpMethod.Start.Offset
	title, hasTitle := titleComment(runes, item)
	var above, among []string
	for _, comment := range item.HeaderComments {
		if hasTitle && comment.Start.Offset == title.Start.Offset {
			continue
		}
		text := string(runes[comment.Start.Offset:comment.End.Offset])
		if comment.Start.Offset < lineStart {
			above = append(above, text)
		} else {
			among = append(among, text)
		}
	}
	version := ""
	if line := item.HttpRequestLine; !line.Version.Empty() {
		version = fmt.Sprintf("HTTP/%d.%d", line.Version.Text.Major, line.Version.Text.Minor)
	}

	start, end := blockExtent(file, index, len(runes))
	rewritten := writeBlock(block, above, among, version)
	if end < len(runes) {
		rewritten += "\n"
	}
	updated := string(runes[:start]) + rewritten + string(runes[end:])

	if err := checkRewrite(updated, index, len(file.Requests), block); err != nil {
		return "", err
	}
	return updated, nil
}

// blockExtent is where block index starts — its first comment or its request
// line — and where the next thing the file holds starts: a block, a variable or
// a stretch the parser could not read, all of which must survive the rewrite.
func blockExtent(file HttpFile, index int, length int) (int, int) {
	item := file.Requests[index]
	start := item.HttpRequestLine.HttpMethod.Start.Offset
	for _, comment := range item.HeaderComments {
		start = min(start, comment.Start.Offset)
	}

	end := length
	consider := func(offset int) {
		if offset > start && offset < end {
			end = offset
		}
	}
	for i, other := range file.Requests {
		if i == index {
			continue
		}
		consider(other.HttpRequestLine.HttpMethod.Start.Offset)
		for _, comment := range other.HeaderComments {
			consider(comment.Start.Offset)
		}
	}
	for _, variable := range file.Variables {
		consider(variable.Name.Start.Offset)
	}
	for _, issue := range file.Issues {
		consider(issue.Start.Offset)
	}
	return start, end
}

// writeBlock prints a block the way the Web UI composer does, ending with a
// line break.
func writeBlock(block RequestBlock, above, among []string, version string) string {
	var b strings.Builder
	b.WriteString(strings.TrimRight(separator+" "+block.Title, " ") + "\n")
	for _, line := range above {
		b.WriteString(line + "\n")
	}
	b.WriteString(block.Method + " " + block.Target)
	if version != "" {
		b.WriteString(" " + version)
	}
	b.WriteString("\n")
	for _, header := range block.Headers {
		b.WriteString(header.Name + ": " + header.Value + "\n")
	}
	for _, line := range among {
		b.WriteString(line + "\n")
	}
	if block.Body != "" {
		b.WriteString("\n" + block.Body + "\n")
	}
	return b.String()
}

// checkRewrite reparses the result: an edit must leave the file with the
// expected number of blocks, and the edited one must read back as written.
// It is the safety net for an input the validation let through but the
// grammar reads differently.
func checkRewrite(updated string, index, count int, want RequestBlock) error {
	blocks := ReadRequests(updated)
	if len(blocks) != count || index >= len(blocks) {
		return errors.New("the request could not be written without changing how the rest of the file reads")
	}
	got := blocks[index]
	if got.Method != want.Method || got.Target != want.Target || got.Body != want.Body || len(got.Headers) != len(want.Headers) {
		return errors.New("the request would not read back as written; check the URL, headers and body")
	}
	return nil
}

var headerNamePattern = regexp.MustCompile("^[A-Za-z0-9!$%&'*+.^_`|~-][A-Za-z0-9!#$%&'*+.^_`|~-]*$")

// normalizeBlock refuses what the line-based format cannot hold — a line break
// in a single-line field, a body line that would open a new block — and tidies
// the rest the way the parser would read it back.
func normalizeBlock(block RequestBlock) (RequestBlock, error) {
	if strings.ContainsAny(block.Title, "\r\n") {
		return RequestBlock{}, errors.New("the title must fit on one line")
	}
	block.Title = strings.TrimSpace(block.Title)

	method, err := models.ParseHttpMethod(strings.TrimSpace(block.Method))
	if err != nil {
		return RequestBlock{}, fmt.Errorf("unsupported HTTP method %q", block.Method)
	}
	block.Method = string(method)

	block.Target = strings.TrimSpace(block.Target)
	if block.Target == "" {
		return RequestBlock{}, errors.New("the URL is required")
	}
	if strings.IndexFunc(block.Target, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return RequestBlock{}, errors.New("the URL must not contain spaces or control characters; percent-encode them")
	}

	headers := make([]HeaderLine, 0, len(block.Headers))
	for _, header := range block.Headers {
		name := strings.TrimSpace(header.Name)
		if !headerNamePattern.MatchString(name) {
			return RequestBlock{}, fmt.Errorf("invalid header name %q", header.Name)
		}
		if strings.ContainsAny(header.Value, "\r\n") {
			return RequestBlock{}, fmt.Errorf("the value of header %s must fit on one line", name)
		}
		headers = append(headers, HeaderLine{Name: name, Value: strings.TrimSpace(header.Value)})
	}
	block.Headers = headers

	body := strings.TrimRight(strings.ReplaceAll(block.Body, "\r\n", "\n"), " \t\n")
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, separator) {
			return RequestBlock{}, errors.New("a body line cannot start with ###: it would open a new request")
		}
	}
	// A body made only of blank lines is no body at all once read back.
	if strings.TrimSpace(body) == "" {
		body = ""
	}
	block.Body = body
	return block, nil
}

// titleComment finds the "###" line that names the block: the first one above
// its request line, as the Web UI reads it.
func titleComment(runes []rune, item HttpRequestFileItem) (CommentLine, bool) {
	lineStart := item.HttpRequestLine.HttpMethod.Start.Offset
	for _, comment := range item.HeaderComments {
		if comment.Start.Offset < lineStart && commentKind(runes, comment) == TokenSeparator {
			return comment, true
		}
	}
	return CommentLine{}, false
}

func titleText(comment CommentLine) string {
	return strings.TrimSpace(strings.TrimLeft(comment.Text, "# "))
}

var placeholderPattern = regexp.MustCompile(`\{\{\s*([\w.-]+)\s*\}\}`)

// Interpolate substitutes {{name}} with the file variables, the first
// declaration of a name winning, as in the Web UI composer. Unknown
// placeholders are left in place and their names reported.
func Interpolate(text string, variables []FileVariable) (string, []string) {
	var unresolved []string
	result := placeholderPattern.ReplaceAllStringFunc(text, func(match string) string {
		name := placeholderPattern.FindStringSubmatch(match)[1]
		for _, variable := range variables {
			if variable.Name.Text == name {
				return variable.Value.Text
			}
		}
		unresolved = append(unresolved, name)
		return match
	})
	return result, unresolved
}
