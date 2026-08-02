package composer

import (
	"fmt"
	"strings"
	"unicode"

	"httpStackLens/helpers"
	"httpStackLens/http/models"

	p "github.com/rflechner/EasyParsingForGo/combinator"
	parsing_helpers "github.com/rflechner/EasyParsingForGo/helpers"
)
import http_parser "httpStackLens/http/parser"

// cutSpace is the trailing noise a span never covers: the padding between
// tokens, and the '\r' a CRLF file leaves at the end of every line.
const cutSpace = " \t\r"

// spanOf measures what a parser consumed between two contexts and leaves out
// the whitespace it swallowed on either side, so that the span covers the token
// rather than its surroundings. cutRight lists the extra characters to drop on
// the right — the ':' closing a header name, for instance.
func spanOf(before, after p.ParsingContext, cutRight string) (start, end p.TextPosition, text string) {
	consumed := before.Remaining[:after.Position.Offset-before.Position.Offset]
	lead := 0
	for lead < len(consumed) && (consumed[lead] == ' ' || consumed[lead] == '\t') {
		lead++
	}
	tail := len(consumed)
	for tail > lead && strings.ContainsRune(cutRight, consumed[tail-1]) {
		tail--
	}
	return before.Position.Forward(consumed[:lead]),
		before.Position.Forward(consumed[:tail]),
		string(consumed[lead:tail])
}

// positioned pairs a parsed value with the span it was read from.
func positioned[T any](before, after p.ParsingContext, cutRight string, value T) PositionedText[T] {
	start, end, _ := spanOf(before, after, cutRight)
	return PositionedText[T]{Text: value, Start: start, End: end}
}

// positionedSource is positioned for a value that is its own source text.
func positionedSource(before, after p.ParsingContext, cutRight string) PositionedText[string] {
	start, end, text := spanOf(before, after, cutRight)
	return PositionedText[string]{Text: text, Start: start, End: end}
}

// lineContext limits a parser to the line it starts on. helpers.UrlParser looks
// for the " HTTP/" marker anywhere ahead of it, which in a file of several
// blocks would let a target run past its own line and swallow the next request.
func lineContext(context p.ParsingContext) p.ParsingContext {
	end := 0
	for end < len(context.Remaining) && context.Remaining[end] != '\n' {
		end++
	}
	return p.ParsingContext{Remaining: context.Remaining[:end], Position: context.Position}
}

// requestTarget is what the target slot of a request line yields: an endpoint
// when the target is a URL, nothing when it is still a template.
type requestTarget struct {
	Endpoint models.ResourceEndpoint
	Resolved bool
}

// requestTargetParser reads the request target, resolving it into a host, a
// port and a path when it can.
func requestTargetParser() p.Parser[requestTarget] {
	return func(context p.ParsingContext) (p.ParseResult[requestTarget], error) {
		if resolved, err := http_parser.EnrichedUrlParser()(lineContext(context)); err == nil {
			return p.ParseResult[requestTarget]{
				Result:  requestTarget{Endpoint: resolved.Result, Resolved: true},
				Context: context.Forward(resolved.Context.Position.Offset - context.Position.Offset),
			}, nil
		}
		return templateTargetParser()(context)
	}
}

// templateTargetParser accepts a target that still holds {{variables}}: it has
// no host to resolve until the file variables are substituted, so it is carried
// as raw text. Demanding a placeholder is what keeps every other unusable
// target — a relative path, a missing target — a parse error.
func templateTargetParser() p.Parser[requestTarget] {
	return func(context p.ParsingContext) (p.ParseResult[requestTarget], error) {
		line := lineContext(context).Remaining
		length := len(line)
		if index := parsing_helpers.IndexOf(line, " HTTP/"); index >= 0 {
			length = index
		}
		text := strings.TrimRight(string(line[0:length]), cutSpace)
		if !strings.Contains(text, "{{") {
			return p.ParseResult[requestTarget]{Context: context},
				fmt.Errorf("unusable request target %q", text)
		}
		return p.ParseResult[requestTarget]{Context: context.Forward(length)}, nil
	}
}

func HttpRequestLineParser() p.Parser[PositionedHttpRequestLine] {
	return func(context p.ParsingContext) (p.ParseResult[PositionedHttpRequestLine], error) {
		httpMethod, err := http_parser.HttpMethodParser()(context)
		if err != nil {
			return p.ParseResult[PositionedHttpRequestLine]{Context: context}, err
		}

		targetResult, err := requestTargetParser()(httpMethod.Context)
		if err != nil {
			return p.ParseResult[PositionedHttpRequestLine]{Context: context}, err
		}

		someVersionResult, err := p.Optional(http_parser.VersionParser())(targetResult.Context)
		if err != nil {
			return p.ParseResult[PositionedHttpRequestLine]{Context: context}, err
		}
		httpVersion := someVersionResult.Result.UnwrapOrDefault(models.Version{Major: 1, Minor: 1})

		return p.ParseResult[PositionedHttpRequestLine]{
			Result: PositionedHttpRequestLine{
				HttpMethod: positioned(context, httpMethod.Context, cutSpace, httpMethod.Result),
				Target:     positionedSource(httpMethod.Context, targetResult.Context, cutSpace),
				Endpoint: positioned(httpMethod.Context, targetResult.Context, cutSpace,
					targetResult.Result.Endpoint),
				Resolved: targetResult.Result.Resolved,
				Version: positioned(targetResult.Context, someVersionResult.Context, cutSpace,
					httpVersion),
			},
			Context: someVersionResult.Context,
		}, nil
	}
}

// FileVariableParser reads an `@name = value` line. The value runs to the end
// of the line; the spaces around the '=' belong to neither half.
func FileVariableParser() p.Parser[FileVariable] {
	return func(context p.ParsingContext) (p.ParseResult[FileVariable], error) {
		marker, err := p.StringMatch("@")(context)
		if err != nil {
			return p.ParseResult[FileVariable]{Context: context}, err
		}

		nameResult, err := p.Many(p.Satisfy(func(c rune) bool {
			return unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_' || c == '-' || c == '.'
		}))(marker.Context)
		if err != nil || len(nameResult.Result) == 0 {
			return p.ParseResult[FileVariable]{Context: context},
				fmt.Errorf("a file variable needs a name")
		}

		assign, err := p.Right(helpers.SpacesParser(), p.StringMatch("="))(nameResult.Context)
		if err != nil {
			return p.ParseResult[FileVariable]{Context: context}, err
		}

		valueResult, err := p.Many(p.Satisfy(func(c rune) bool {
			return c != '\r' && c != '\n'
		}))(assign.Context)
		if err != nil {
			return p.ParseResult[FileVariable]{Context: context}, err
		}

		return p.ParseResult[FileVariable]{
			Result: FileVariable{
				// The span opens on the '@' so that the whole declaration can be
				// painted, while Text holds the bare name a lookup needs.
				Name:  PositionedText[string]{Text: string(nameResult.Result), Start: context.Position, End: nameResult.Context.Position},
				Value: positionedSource(assign.Context, valueResult.Context, cutSpace),
			},
			Context: valueResult.Context,
		}, nil
	}
}

func commentLineParser(prefix string) p.Parser[CommentLine] {
	return func(context p.ParsingContext) (p.ParseResult[CommentLine], error) {
		prefixResult, err := p.StringMatch(prefix)(context)
		if err != nil {
			return p.ParseResult[CommentLine]{Context: context}, err
		}
		text, err := p.UntilText(p.Many(p.Satisfy(func(r rune) bool { return true })), "\n", false)(prefixResult.Context)
		if err != nil {
			return p.ParseResult[CommentLine]{Context: context}, err
		}

		// The span opens on the '#' marker, which the text leaves out: a
		// highlighter paints the line, a reader gets the note.
		start, end, _ := spanOf(context, text.Context, "\r")
		comment := CommentLine{
			Text:  string(text.Result),
			Start: start,
			End:   end,
		}

		return p.ParseResult[CommentLine]{
			Result:  comment,
			Context: text.Context,
		}, nil
	}
}

func CommentLineParser() p.Parser[CommentLine] {
	return commentLineParser("#")
}

func HeaderCommentsParser() p.Parser[CommentLine] {
	return commentLineParser("###")
}

// separator opens a request block and closes the previous one.
const separator = "###"

// HttpRequestFileItemParser reads one request block of a `.http` file:
//
//	### Current user
//	# the token comes from the file variables
//	GET https://api.github.com/user
//	Accept: application/json
//
//	{ "note": "the body is everything after the empty line" }
//
// Every comment line of the block is collected into HeaderComments, whether it
// sits above the request line or between the headers. The block ends on the
// "###" line that opens the next one, which is left unconsumed so that the
// caller can loop, or at the end of the file.
func HttpRequestFileItemParser() p.Parser[HttpRequestFileItem] {
	return func(context p.ParsingContext) (p.ParseResult[HttpRequestFileItem], error) {
		next := skipBlankLines(context)

		item := HttpRequestFileItem{}
		item.HeaderComments, next = parseCommentLines(next, item.HeaderComments)

		requestLine, err := HttpRequestLineParser()(next)
		if err != nil {
			return p.ParseResult[HttpRequestFileItem]{Context: context}, err
		}
		item.HttpRequestLine = requestLine.Result
		next = consumeEndOfLine(requestLine.Context)

		for !next.AtEnd() && !isBlankLine(next) && !startsWith(next, separator) {
			if startsWith(next, "#") {
				item.HeaderComments, next = parseCommentLines(next, item.HeaderComments)
				continue
			}
			header, err := positionedHeaderParser()(next)
			if err != nil {
				break // not a header line: what follows belongs to the body
			}
			item.Headers = append(item.Headers, header.Result)
			next = consumeEndOfLine(header.Context)
		}

		item.Body, next = parseBody(next)

		return p.ParseResult[HttpRequestFileItem]{Result: item, Context: next}, nil
	}
}

// positionedHeaderParser reads a `Name: value` line, keeping the position of
// each half. It is the positioned twin of http/parser.HeaderParser, and stops
// before the line break, which the caller consumes.
func positionedHeaderParser() p.Parser[PositionedHeader] {
	return func(context p.ParsingContext) (p.ParseResult[PositionedHeader], error) {
		// Excluding '\n' from the name keeps the search for ':' on this line: a
		// line without a colon fails instead of matching a later header's colon.
		nameParser := p.UntilText(p.Many(p.Satisfy(func(c rune) bool {
			return c != ':' && c != '\n'
		})), ":", true)
		valueParser := p.Many(p.Satisfy(func(c rune) bool {
			return c != '\r' && c != '\n'
		}))

		nameResult, err := nameParser(context)
		if err != nil {
			return p.ParseResult[PositionedHeader]{Context: context}, err
		}
		valueResult, err := valueParser(nameResult.Context)
		if err != nil {
			return p.ParseResult[PositionedHeader]{Context: context}, err
		}

		return p.ParseResult[PositionedHeader]{
			Result: PositionedHeader{
				// The name parser swallows the ':' that closes it, which the span
				// gives back so that the separator can be painted on its own.
				Name:  positionedSource(context, nameResult.Context, cutSpace+":"),
				Value: positionedSource(nameResult.Context, valueResult.Context, cutSpace),
			},
			Context: valueResult.Context,
		}, nil
	}
}

// parseCommentLines appends every comment line found at the current position.
// "###" is tried first so that a separator is not read as a "#" comment whose
// text starts with the two remaining markers.
func parseCommentLines(context p.ParsingContext, comments []CommentLine) ([]CommentLine, p.ParsingContext) {
	parser := p.OrElse(HeaderCommentsParser(), CommentLineParser())
	next := context
	for {
		result, err := parser(next)
		if err != nil {
			return comments, next
		}
		comments = append(comments, result.Result)
		next = consumeEndOfLine(result.Context)
	}
}

// parseBody reads everything left in the block: the empty line that separates
// the headers from the body is dropped, the trailing blank lines before the
// next separator are trimmed.
func parseBody(context p.ParsingContext) (PositionedText[string], p.ParsingContext) {
	next := context
	if !next.AtEnd() && isBlankLine(next) {
		_, next = readLine(next)
	}

	start := next.Position
	var body strings.Builder
	for !next.AtEnd() && !startsWith(next, separator) {
		line, rest := readLine(next)
		body.WriteString(line)
		next = rest
	}

	// The span stops where the text does: the blank lines before the next
	// separator were read, but they are not part of the body.
	text := strings.TrimRight(body.String(), "\r\n")
	return PositionedText[string]{
		Text:  text,
		Start: start,
		End:   start.Forward([]rune(text)),
	}, next
}

func skipBlankLines(context p.ParsingContext) p.ParsingContext {
	next := context
	for !next.AtEnd() && isBlankLine(next) {
		_, next = readLine(next)
	}
	return next
}

func consumeEndOfLine(context p.ParsingContext) p.ParsingContext {
	result, err := helpers.NewLineParser()(context)
	if err != nil {
		return context
	}
	return result.Context
}

// readLine returns the current line, line break included, and the context that
// follows it.
func readLine(context p.ParsingContext) (string, p.ParsingContext) {
	length := 0
	for length < len(context.Remaining) && context.Remaining[length] != '\n' {
		length++
	}
	if length < len(context.Remaining) {
		length++ // the line break itself
	}
	return string(context.Remaining[0:length]), context.Forward(length)
}

// isBlankLine reports whether the current line holds nothing but spaces.
func isBlankLine(context p.ParsingContext) bool {
	for _, c := range context.Remaining {
		if c == '\n' {
			return true
		}
		if c != ' ' && c != '\t' && c != '\r' {
			return false
		}
	}
	return true
}

func startsWith(context p.ParsingContext, prefix string) bool {
	runes := []rune(prefix)
	if len(context.Remaining) < len(runes) {
		return false
	}
	return string(context.Remaining[0:len(runes)]) == prefix
}
