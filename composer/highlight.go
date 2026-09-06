package composer

import (
	"sort"
	"strings"
	"unicode"

	p "github.com/rflechner/EasyParsingForGo/combinator"
)

// TokenKind names what a stretch of source is. The values double as CSS class
// suffixes: "method" is painted by `.hl-method`.
type TokenKind string

const (
	TokenSeparator   TokenKind = "separator"
	TokenComment     TokenKind = "comment"
	TokenVariable    TokenKind = "variable"
	TokenMethod      TokenKind = "method"
	TokenTarget      TokenKind = "target"
	TokenVersion     TokenKind = "version"
	TokenHeaderName  TokenKind = "header"
	TokenValue       TokenKind = "value"
	TokenBody        TokenKind = "body"
	TokenPlaceholder TokenKind = "placeholder"
)

// Token is one painted stretch of source. Offsets are in runes, half-open, and
// count from the start of the file.
type Token struct {
	Kind  TokenKind
	Start int
	End   int
}

// placeholderHosts are the tokens a {{variable}} can appear in.
var placeholderHosts = map[TokenKind]bool{
	TokenTarget: true,
	TokenValue:  true,
	TokenBody:   true,
}

// Tokenize turns a `.http` source into the spans a syntax highlighter paints.
// It is the same grammar the back end executes, read for its positions instead
// of its values, so the two can never drift.
//
// The result covers only what is meaningful: the whitespace and punctuation
// between two tokens are left out, and the caller emits them as they are.
func Tokenize(source string) []Token {
	runes := []rune(source)
	file := ParseHttpFile(source)

	var tokens []Token
	for _, variable := range file.Variables {
		tokens = appendSpan(tokens, TokenVariable, variable.Name)
		tokens = appendSpan(tokens, TokenValue, variable.Value)
	}

	for _, request := range file.Requests {
		for _, comment := range request.HeaderComments {
			tokens = appendSpan(tokens, commentKind(runes, comment), comment)
		}
		line := request.HttpRequestLine
		tokens = appendSpan(tokens, TokenMethod, line.HttpMethod)
		tokens = appendSpan(tokens, TokenTarget, line.Target)
		tokens = appendSpan(tokens, TokenVersion, line.Version)
		for _, header := range request.Headers {
			tokens = appendSpan(tokens, TokenHeaderName, header.Name)
			tokens = appendSpan(tokens, TokenValue, header.Value)
		}
		tokens = appendSpan(tokens, TokenBody, request.Body)
	}

	for _, issue := range file.Issues {
		tokens = append(tokens, lexUnreadable(runes, issue.Start.Offset, issue.End.Offset)...)
	}

	sort.SliceStable(tokens, func(i, j int) bool { return tokens[i].Start < tokens[j].Start })
	return splitPlaceholders(runes, dropOverlaps(tokens))
}

// HighlightHTML renders the source as the inner HTML of a <pre>, every token
// wrapped in a `<span class="hl-…">` and everything else escaped as it is.
//
// The markup is built here rather than from offsets handed to JavaScript on
// purpose: a TextPosition counts runes where the DOM counts UTF-16 code units,
// so a single accent in a comment would shift every span after it.
func HighlightHTML(source string) string {
	runes := []rune(source)
	var out strings.Builder
	out.Grow(len(source) * 2)

	cursor := 0
	for _, token := range Tokenize(source) {
		if token.Start > cursor {
			writeEscaped(&out, runes[cursor:token.Start])
		}
		out.WriteString(`<span class="hl-`)
		out.WriteString(string(token.Kind))
		out.WriteString(`">`)
		writeEscaped(&out, runes[token.Start:token.End])
		out.WriteString(`</span>`)
		cursor = token.End
	}
	writeEscaped(&out, runes[cursor:])

	// A <pre> swallows its last line break, which would shorten the overlay by
	// one line against the textarea it sits behind.
	out.WriteByte('\n')
	return out.String()
}

// lexUnreadable paints a block the grammar could not read as a whole. Every
// line is offered to the same parsers one at a time, so that a file caught
// mid-edit still shows its comments, its variables and the request lines that
// are already valid — only the line actually being typed goes unpainted.
func lexUnreadable(runes []rune, from, to int) []Token {
	var tokens []Token
	for start := from; start < to; {
		end := start
		for end < to && runes[end] != '\n' {
			end++
		}
		tokens = append(tokens, lexLine(runes, start, end)...)
		start = end + 1
	}
	return tokens
}

func lexLine(runes []rune, start, end int) []Token {
	context := lineContextAt(runes, start, end)

	if variable, err := FileVariableParser()(context); err == nil {
		return []Token{
			{TokenVariable, variable.Result.Name.Start.Offset, variable.Result.Name.End.Offset},
			{TokenValue, variable.Result.Value.Start.Offset, variable.Result.Value.End.Offset},
		}
	}
	if comment, err := HeaderCommentsParser()(context); err == nil {
		return []Token{{TokenSeparator, comment.Result.Start.Offset, comment.Result.End.Offset}}
	}
	if comment, err := CommentLineParser()(context); err == nil {
		return []Token{{TokenComment, comment.Result.Start.Offset, comment.Result.End.Offset}}
	}
	if request, err := HttpRequestLineParser()(context); err == nil {
		return []Token{
			{TokenMethod, request.Result.HttpMethod.Start.Offset, request.Result.HttpMethod.End.Offset},
			{TokenTarget, request.Result.Target.Start.Offset, request.Result.Target.End.Offset},
			{TokenVersion, request.Result.Version.Start.Offset, request.Result.Version.End.Offset},
		}
	}
	// Any line holding a ':' parses as a header, which would paint a JSON body
	// as one. Demanding a header-shaped name keeps the guess to lines that
	// really look like headers.
	if header, err := positionedHeaderParser()(context); err == nil && isHeaderName(header.Result.Name.Text) {
		return []Token{
			{TokenHeaderName, header.Result.Name.Start.Offset, header.Result.Name.End.Offset},
			{TokenValue, header.Result.Value.Start.Offset, header.Result.Value.End.Offset},
		}
	}
	return nil
}

// lineContextAt isolates one line, keeping its offsets those of the whole file
// so that the spans come back absolute. The line break is added back when the
// file does not have one: the comment parsers read up to a delimiter, and would
// not see the last line of a file that ends without a newline.
func lineContextAt(runes []rune, start, end int) p.ParsingContext {
	line := make([]rune, 0, end-start+1)
	line = append(line, runes[start:end]...)
	line = append(line, '\n')
	return p.ParsingContext{
		Remaining: line,
		Position:  p.TextPosition{Offset: start, Line: 1, Column: 1},
	}
}

func isHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '-' && c != '_' {
			return false
		}
	}
	return true
}

// commentKind tells the "###" line that opens a block from an ordinary "#"
// note. Both are the same parsed value, and only the source says which is which.
func commentKind(runes []rune, comment CommentLine) TokenKind {
	start := comment.Start.Offset
	if start+len(separator) <= len(runes) && string(runes[start:start+len(separator)]) == separator {
		return TokenSeparator
	}
	return TokenComment
}

func appendSpan[T any](tokens []Token, kind TokenKind, span PositionedText[T]) []Token {
	if span.Empty() {
		return tokens
	}
	return append(tokens, Token{Kind: kind, Start: span.Start.Offset, End: span.End.Offset})
}

// dropOverlaps keeps the token stream strictly ordered. Two parsers can claim
// the same stretch — a recovered block re-reading a line the item parser had
// already got through — and the renderer needs one owner per rune.
func dropOverlaps(tokens []Token) []Token {
	kept := tokens[:0]
	end := 0
	for _, token := range tokens {
		if token.Start < end || token.End <= token.Start {
			continue
		}
		kept = append(kept, token)
		end = token.End
	}
	return kept
}

// splitPlaceholders breaks the tokens that may hold {{variables}} into their
// plain stretches and the placeholders between them.
func splitPlaceholders(runes []rune, tokens []Token) []Token {
	out := make([]Token, 0, len(tokens))
	for _, token := range tokens {
		if !placeholderHosts[token.Kind] {
			out = append(out, token)
			continue
		}
		cursor := token.Start
		for cursor < token.End {
			open := indexRunes(runes, cursor, token.End, "{{")
			if open < 0 {
				break
			}
			closing := indexRunes(runes, open+2, token.End, "}}")
			if closing < 0 {
				break
			}
			if open > cursor {
				out = append(out, Token{token.Kind, cursor, open})
			}
			out = append(out, Token{TokenPlaceholder, open, closing + 2})
			cursor = closing + 2
		}
		if cursor < token.End {
			out = append(out, Token{token.Kind, cursor, token.End})
		}
	}
	return out
}

func indexRunes(runes []rune, from, to int, needle string) int {
	target := []rune(needle)
	for i := from; i+len(target) <= to; i++ {
		if string(runes[i:i+len(target)]) == needle {
			return i
		}
	}
	return -1
}

func writeEscaped(out *strings.Builder, runes []rune) {
	for _, c := range runes {
		switch c {
		case '&':
			out.WriteString("&amp;")
		case '<':
			out.WriteString("&lt;")
		case '>':
			out.WriteString("&gt;")
		default:
			out.WriteRune(c)
		}
	}
}
