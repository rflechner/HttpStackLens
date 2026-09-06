package composer

import (
	"httpStackLens/http/models"

	p "github.com/rflechner/EasyParsingForGo/combinator"
)

// PositionedText associates a value with the extent of source text it was read
// from. Start and End delimit that extent — half-open, Start included — so a
// caller can slice the source back out or paint the range, while Text keeps
// what the parser made of it.
//
// The two differ on purpose: the span of a comment covers its '#' marker and
// the span of a header name stops before the ':', neither of which appears in
// Text. Anything that needs the source exactly as written — a syntax
// highlighter, a request target still holding {{variables}} — reads the span;
// anything that needs the meaning reads Text.
type PositionedText[T any] struct {
	Text  T
	Start p.TextPosition
	End   p.TextPosition
}

// Empty reports whether the parser consumed nothing, which is how a defaulted
// value is told apart from a read one: an omitted HTTP version yields 1.1 over
// an empty span.
func (t PositionedText[T]) Empty() bool { return t.End.Offset <= t.Start.Offset }

type CommentLine = PositionedText[string]

type PositionedHeader struct {
	Name  PositionedText[string]
	Value PositionedText[string]
}

// FileVariable is an `@name = value` line. Variables are file-scoped and are
// substituted into the requests below them.
type FileVariable struct {
	Name  PositionedText[string]
	Value PositionedText[string]
}

type PositionedHttpRequestLine struct {
	HttpMethod PositionedText[models.HttpMethod]
	// Target is the request target exactly as written, {{placeholders}}
	// included.
	Target PositionedText[string]
	// Endpoint is Target resolved into a host, a port and a path. A target that
	// still holds placeholders cannot be resolved before interpolation: Endpoint
	// is then the zero value and Resolved is false.
	Endpoint PositionedText[models.ResourceEndpoint]
	Resolved bool
	Version  PositionedText[models.Version]
}

type HttpRequestFileItem struct {
	HeaderComments  []CommentLine
	HttpRequestLine PositionedHttpRequestLine
	Headers         []PositionedHeader
	InnerComments   []CommentLine
	Body            PositionedText[string]
}

// ParseIssue is a stretch of source ParseHttpFile could not make sense of. The
// span is reported rather than dropped so that the caller can underline it and
// keep going — a file being typed into is invalid most of the time.
type ParseIssue struct {
	Message string
	Start   p.TextPosition
	End     p.TextPosition
}

type HttpFile struct {
	Variables []FileVariable
	Requests  []HttpRequestFileItem
	Issues    []ParseIssue
}
