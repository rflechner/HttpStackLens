package parser

import (
	"fmt"
	"httpStackLens/http/ast"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"github.com/rflechner/EasyParsingForGo/helpers"
)
import p "github.com/rflechner/EasyParsingForGo/combinator"

func VersionParser() p.Parser[ast.Version] {
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
		}) ast.Version {
			return ast.Version{Major: t.Left, Minor: t.Right}
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

func SpacesParser() p.Parser[struct{}] {
	return p.Skip(p.Spaces())
}

func HostPortParser() p.Parser[ast.HostPort] {
	onlyHostPortParser := p.Map(
		p.Left(
			p.Combine(
				HostParser(),
				p.Optional(p.Right(p.OneChar(':'), p.Integer())),
			),
			SpacesParser(),
		),
		func(hostPort struct {
			Left  string
			Right helpers.Option[int]
		}) ast.HostPort {
			return ast.HostPort{Host: hostPort.Left, Port: hostPort.Right.UnwrapOrDefault(443)}
		},
	)

	urlParser := p.Map(UrlParser(), func(url url.URL) ast.HostPort {

		var defaultPort int
		if strings.ToLower(url.Scheme) == "https" {
			defaultPort = 443
		} else {
			defaultPort = 80
		}

		if strings.ContainsRune(url.Host, ':') {
			portText := url.Port()
			port, err := strconv.Atoi(portText)
			if err != nil {
				return ast.HostPort{Host: url.Hostname(), Port: defaultPort}
			}
			return ast.HostPort{Host: url.Hostname(), Port: port}
		}

		return ast.HostPort{Host: url.Hostname(), Port: defaultPort}
	})

	return p.OrElse(urlParser, onlyHostPortParser)
}

func UrlParser() p.Parser[url.URL] {
	return func(context p.ParsingContext) (p.ParseResult[url.URL], error) {
		text, err := p.UntilText(p.Many(p.Satisfy(func(r rune) bool { return true })), " HTTP/", false)(context)
		if err != nil {
			return p.ParseResult[url.URL]{Context: context}, err
		}

		urlString := string(text.Result)
		if strings.Contains(urlString, "://") == false {
			return p.ParseResult[url.URL]{Context: context}, fmt.Errorf("URL must contain protocol")
		}
		parsedUrl, err := url.Parse(urlString)
		if err != nil {
			return p.ParseResult[url.URL]{Context: context}, err
		}

		return p.ParseResult[url.URL]{
			Result:  *parsedUrl,
			Context: text.Context,
		}, nil
	}
}

func ConnectParser() p.Parser[ast.Connect] {
	return func(context p.ParsingContext) (p.ParseResult[ast.Connect], error) {
		verbParser := p.Left(p.StringMatch("CONNECT"), SpacesParser())

		verbResult, err := verbParser(context)
		if err != nil {
			return p.ParseResult[ast.Connect]{Context: context}, err
		}

		hostPortResult, err := HostPortParser()(verbResult.Context)
		if err != nil {
			return p.ParseResult[ast.Connect]{Context: context}, err
		}

		versionResult, err := VersionParser()(hostPortResult.Context)
		if err != nil {
			return p.ParseResult[ast.Connect]{Context: context}, err
		}

		return p.ParseResult[ast.Connect]{
			Result: ast.Connect{
				HostPort: ast.HostPort{
					Host: hostPortResult.Result.Host,
					Port: hostPortResult.Result.Port,
				},
				Version: versionResult.Result,
			},
			Context: versionResult.Context,
		}, nil
	}
}

func HeaderParser() p.Parser[ast.Header] {
	return func(context p.ParsingContext) (p.ParseResult[ast.Header], error) {
		nameParser := p.Map(
			p.UntilText(p.Many(p.Satisfy(func(c rune) bool {
				return c != ':'
			})), ":", true),
			func(n []rune) string { return string(n) })

		valueParser := p.Map(
			p.Left(
				p.Many(p.Satisfy(func(c rune) bool {
					return true
				})),
				p.Spaces(),
			),
			func(v []rune) string { return strings.TrimSpace(string(v)) })

		nameResult, err := nameParser(context)
		if err != nil {
			return p.ParseResult[ast.Header]{Context: context}, err
		}
		valueResult, err := valueParser(nameResult.Context)
		if err != nil {
			return p.ParseResult[ast.Header]{Context: context}, err
		}

		return p.ParseResult[ast.Header]{
			Result: ast.Header{
				Name:  nameResult.Result,
				Value: valueResult.Result,
			},
			Context: valueResult.Context,
		}, nil
	}
}
