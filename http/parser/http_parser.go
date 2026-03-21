package parser

import (
	"httpStackLens/http/ast"
	"strings"
	"unicode"
)
import p "github.com/rflechner/EasyParsingForGo/combinator"

func VersionParser() p.Parser[ast.Version] {
	return p.Map(
		p.Right(
			p.StringMatch("HTTP/"),
			p.Combine(p.Integer(),
				p.Right(p.OneChar('.'), p.Integer()),
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
	return p.Map(
		p.UntilText(p.Many(p.Satisfy(func(c rune) bool {
			return unicode.IsLetter(c) || unicode.IsDigit(c) || c == '-' || c == '_' || c == '.'
		})), ":", false),
		func(host []rune) string { return string(host) })
}

func SpacesParser() p.Parser[struct{}] {
	return p.Skip(p.Spaces())
}

func HostPortParser() p.Parser[ast.HostPort] {
	return p.Map(
		p.Left(
			p.Combine(
				HostParser(),
				p.Right(p.OneChar(':'), p.Integer()),
			),
			SpacesParser(),
		),
		func(hostPort struct {
			Left  string
			Right int
		}) ast.HostPort {
			return ast.HostPort{hostPort.Left, hostPort.Right}
		},
	)
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
