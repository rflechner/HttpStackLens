package composer

import (
	"strings"

	"httpStackLens/helpers"
	"httpStackLens/http/models"

	p "github.com/rflechner/EasyParsingForGo/combinator"
)
import http_parser "httpStackLens/http/parser"

func HttpRequestLineParser() p.Parser[PositionedHttpRequestLine] {
	return func(context p.ParsingContext) (p.ParseResult[PositionedHttpRequestLine], error) {
		httpMethod, err := http_parser.HttpMethodParser()(context)
		if err != nil {
			return p.ParseResult[PositionedHttpRequestLine]{Context: context}, err
		}

		targetResult, err := http_parser.EnrichedUrlParser()(httpMethod.Context)
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
				HttpMethod: PositionedText[models.HttpMethod]{
					Text:     httpMethod.Result,
					Position: httpMethod.Context.Position,
				},
				Endpoint: PositionedText[models.ResourceEndpoint]{
					Text:     targetResult.Result,
					Position: targetResult.Context.Position,
				},
				Version: PositionedText[models.Version]{
					Text:     httpVersion,
					Position: someVersionResult.Context.Position,
				},
			},
			Context: someVersionResult.Context,
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

		comment := CommentLine{
			Text:     string(text.Result),
			Position: text.Context.Position,
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
				Name: PositionedText[string]{
					Text:     strings.TrimSpace(string(nameResult.Result)),
					Position: nameResult.Context.Position,
				},
				Value: PositionedText[string]{
					Text:     strings.TrimSpace(string(valueResult.Result)),
					Position: valueResult.Context.Position,
				},
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

	var body strings.Builder
	for !next.AtEnd() && !startsWith(next, separator) {
		line, rest := readLine(next)
		body.WriteString(line)
		next = rest
	}

	return PositionedText[string]{
		Text:     strings.TrimRight(body.String(), "\r\n"),
		Position: next.Position,
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
