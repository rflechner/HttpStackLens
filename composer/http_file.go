package composer

import (
	"httpStackLens/http/models"

	p "github.com/rflechner/EasyParsingForGo/combinator"
)

// PositionedText associates a value with its location in the parsed source.
// The generic value keeps the position handling reusable for other parsed
// text values while CommentLine retains its existing public shape.
type PositionedText[T any] struct {
	Text     T
	Position p.TextPosition
}

type CommentLine = PositionedText[string]

type PositionedHeader struct {
	Name  PositionedText[string]
	Value PositionedText[string]
}

type PositionedHttpRequestLine struct {
	HttpMethod PositionedText[models.HttpMethod]
	Endpoint   PositionedText[models.ResourceEndpoint]
	Version    PositionedText[models.Version]
}

type HttpRequestFileItem struct {
	HeaderComments  []CommentLine
	HttpRequestLine PositionedHttpRequestLine
	Headers         []PositionedHeader
	Body            PositionedText[string]
}

type HttpFile struct {
	Requests []HttpRequestFileItem
}
