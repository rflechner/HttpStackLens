package composer

import (
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
