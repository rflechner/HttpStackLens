package composer

import (
	"httpStackLens/http/models"

	p "github.com/rflechner/EasyParsingForGo/combinator"
)
import http_parser "httpStackLens/http/parser"

func HttpRequestLineParser() p.Parser[models.HttpRequestLine] {
	return func(context p.ParsingContext) (p.ParseResult[models.HttpRequestLine], error) {
		httpMethod, err := http_parser.HttpMethodParser()(context)
		if err != nil {
			return p.ParseResult[models.HttpRequestLine]{Context: context}, err
		}

		targetResult, err := http_parser.EnrichedUrlParser()(httpMethod.Context)
		if err != nil {
			return p.ParseResult[models.HttpRequestLine]{Context: context}, err
		}

		someVersionResult, err := p.Optional(http_parser.VersionParser())(targetResult.Context)
		if err != nil {
			return p.ParseResult[models.HttpRequestLine]{Context: context}, err
		}
		httpVersion := someVersionResult.Result.UnwrapOrDefault(models.Version{Major: 1, Minor: 1})

		return p.ParseResult[models.HttpRequestLine]{
			Result: models.HttpRequestLine{
				HttpMethod: httpMethod.Result,
				Endpoint:   targetResult.Result,
				Version:    httpVersion,
			},
			Context: someVersionResult.Context,
		}, nil
	}
}
