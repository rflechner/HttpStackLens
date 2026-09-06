package parser

import (
	"httpStackLens/http/models"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)
import p "github.com/rflechner/EasyParsingForGo/combinator"
import helpers "httpStackLens/helpers"
import parsing_helpers "github.com/rflechner/EasyParsingForGo/helpers"

func VersionParser() p.Parser[models.Version] {
	return p.Map(
		p.Right(p.Spaces(),
			p.Right(
				p.StringMatch("HTTP/"),
				p.Combine(p.Integer(),
					p.Right(p.OneChar('.'), p.Integer()),
				),
			),
		),
		func(t struct {
			Left  int
			Right int
		}) models.Version {
			return models.Version{Major: t.Left, Minor: t.Right}
		},
	)
}

func HostParser() p.Parser[string] {
	hostWithPortParser := p.UntilText(p.Many(p.Satisfy(func(c rune) bool {
		return unicode.IsLetter(c) || unicode.IsDigit(c) || c == '-' || c == '_' || c == '.'
	})), ":", false)
	hostWithoutPortParser := p.UntilText(p.Many(p.Satisfy(func(c rune) bool {
		return unicode.IsLetter(c) || unicode.IsDigit(c) || c == '-' || c == '_' || c == '.'
	})), " ", false)

	return p.Map(
		p.OrElse(hostWithPortParser, hostWithoutPortParser),
		func(host []rune) string { return string(host) })
}

func EnrichedUrlParser() p.Parser[models.ResourceEndpoint] {
	return p.Map(helpers.UrlParser(), func(url url.URL) models.ResourceEndpoint {

		var defaultPort int
		if strings.ToLower(url.Scheme) == "https" {
			defaultPort = 443
		} else {
			defaultPort = 80
		}

		var pathAndQuery string
		if url.RawQuery != "" {
			pathAndQuery = url.Path + "?" + url.RawQuery
		} else {
			pathAndQuery = url.Path
		}

		if pathAndQuery == "" {
			pathAndQuery = "/"
		}

		if strings.ContainsRune(url.Host, ':') {
			portText := url.Port()
			port, err := strconv.Atoi(portText)
			if err != nil {
				return models.ResourceEndpoint{Host: url.Hostname(), Port: defaultPort}
			}
			return models.ResourceEndpoint{
				Host:         url.Hostname(),
				Port:         port,
				PathAndQuery: pathAndQuery,
			}
		}

		return models.ResourceEndpoint{
			Host:         url.Hostname(),
			Port:         defaultPort,
			PathAndQuery: pathAndQuery,
		}
	})
}

func ResourceEndpointParser() p.Parser[models.ResourceEndpoint] {
	onlyHostPortParser := p.Map(
		p.Left(
			p.Combine(
				HostParser(),
				p.Optional(p.Right(p.OneChar(':'), p.Integer())),
			),
			helpers.SpacesParser(),
		),
		func(hostPort struct {
			Left  string
			Right parsing_helpers.Option[int]
		}) models.ResourceEndpoint {
			return models.ResourceEndpoint{Host: hostPort.Left, Port: hostPort.Right.UnwrapOrDefault(443)}
		},
	)

	return p.OrElse(EnrichedUrlParser(), onlyHostPortParser)
}

func HttpMethodParser() p.Parser[models.HttpMethod] {
	return func(context p.ParsingContext) (p.ParseResult[models.HttpMethod], error) {
		vp := p.Map(p.Many(p.Alphanumeric()), func(runes []rune) string { return string(runes) })
		verbParser := p.Left(
			vp,
			helpers.SpacesParser(),
		)

		verbResult, err := verbParser(context)
		if err != nil {
			return p.ParseResult[models.HttpMethod]{Context: context}, err
		}
		httpMethod, err := models.ParseHttpMethod(verbResult.Result)
		if err != nil {
			return p.ParseResult[models.HttpMethod]{Context: context}, err
		}

		return p.ParseResult[models.HttpMethod]{Context: verbResult.Context, Result: httpMethod}, nil
	}
}

func HttpRequestLineParser() p.Parser[models.HttpRequestLine] {
	return func(context p.ParsingContext) (p.ParseResult[models.HttpRequestLine], error) {

		httpMethod, err := HttpMethodParser()(context)
		if err != nil {
			return p.ParseResult[models.HttpRequestLine]{Context: context}, err
		}

		hostPortResult, err := ResourceEndpointParser()(httpMethod.Context)
		if err != nil {
			return p.ParseResult[models.HttpRequestLine]{Context: context}, err
		}

		versionResult, err := VersionParser()(hostPortResult.Context)
		if err != nil {
			return p.ParseResult[models.HttpRequestLine]{Context: context}, err
		}

		return p.ParseResult[models.HttpRequestLine]{
			Result: models.HttpRequestLine{
				HttpMethod: httpMethod.Result,
				Endpoint:   hostPortResult.Result,
				Version:    versionResult.Result,
			},
			Context: versionResult.Context,
		}, nil
	}
}

func HeaderParser() p.Parser[models.Header] {
	return func(context p.ParsingContext) (p.ParseResult[models.Header], error) {
		nameParser := p.Map(
			p.UntilText(p.Many(p.Satisfy(func(c rune) bool {
				return c != ':'
			})), ":", true),
			func(n []rune) string { return string(n) })

		valueParser := p.Map(
			p.Left(
				p.Many(p.Satisfy(func(c rune) bool {
					return c != '\r' && c != '\n'
				})),
				p.Optional(helpers.NewLineParser()),
			),
			func(v []rune) string { return strings.TrimSpace(string(v)) })

		nameResult, err := nameParser(context)
		if err != nil {
			return p.ParseResult[models.Header]{Context: context}, err
		}
		valueResult, err := valueParser(nameResult.Context)
		if err != nil {
			return p.ParseResult[models.Header]{Context: context}, err
		}

		return p.ParseResult[models.Header]{
			Result: models.Header{
				Name:  nameResult.Result,
				Value: valueResult.Result,
			},
			Context: valueResult.Context,
		}, nil
	}
}

type responseStatus struct {
	HttpVersion       models.Version
	StatusCode        int
	StatusDescription string
}

func responseStatusParser() p.Parser[responseStatus] {
	statusDescriptionParser := p.Map(
		p.Many(p.Satisfy(func(c rune) bool {
			return c != '\r' && c != '\n'
		})),
		func(r []rune) string { return string(r) })

	firstLineParserStart := p.Map(
		p.Left(
			p.Combine(
				p.Left(
					VersionParser(),
					helpers.SpacesParser(),
				),
				p.Integer(),
			), p.Spaces()),
		func(r struct {
			Left  models.Version
			Right int
		}) responseStatus {
			return responseStatus{
				HttpVersion: r.Left,
				StatusCode:  r.Right,
			}
		},
	)

	return p.Map(
		p.Combine(firstLineParserStart, statusDescriptionParser),
		func(r struct {
			Left  responseStatus
			Right string
		}) responseStatus {
			return responseStatus{
				HttpVersion:       r.Left.HttpVersion,
				StatusCode:        r.Left.StatusCode,
				StatusDescription: r.Right,
			}
		})
}

func ResponseHeadParser() p.Parser[models.HttpResponseHead] {
	headersParser := p.Many(HeaderParser())

	return p.Map(
		p.Combine(
			p.Left(
				responseStatusParser(),
				p.Optional(helpers.NewLineParser()),
			),
			p.Left(
				headersParser,
				p.Optional(helpers.NewLineParser()),
			),
		),
		func(r struct {
			Left  responseStatus
			Right []models.Header
		}) models.HttpResponseHead {
			return models.HttpResponseHead{
				HttpVersion:       r.Left.HttpVersion,
				StatusCode:        r.Left.StatusCode,
				StatusDescription: r.Left.StatusDescription,
				Headers:           r.Right,
			}
		},
	)
}
