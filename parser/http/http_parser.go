package http

import (
	"unicode"
)
import p "github.com/rflechner/EasyParsingForGo/combinator"

func VersionParser() p.Parser[Version] {
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
		}) Version {
			return Version{Major: t.Left, Minor: t.Right}
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

func HostPortParser() p.Parser[HostPort] {
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
		}) HostPort {
			return HostPort{hostPort.Left, hostPort.Right}
		},
	)
}

func ConnectParser() p.Parser[Connect] {
	return func(context p.ParsingContext) (p.ParseResult[Connect], error) {
		verbParser := p.Left(p.StringMatch("CONNECT"), SpacesParser())

		verbResult, err := verbParser(context)
		if err != nil {
			return p.ParseResult[Connect]{Context: context}, err
		}

		hostPortResult, err := HostPortParser()(verbResult.Context)
		if err != nil {
			return p.ParseResult[Connect]{Context: context}, err
		}

		versionResult, err := VersionParser()(hostPortResult.Context)
		if err != nil {
			return p.ParseResult[Connect]{Context: context}, err
		}

		return p.ParseResult[Connect]{
			Result: Connect{
				HostPort: HostPort{
					Host: hostPortResult.Result.Host,
					Port: hostPortResult.Result.Port,
				},
				Version: versionResult.Result,
			},
			Context: versionResult.Context,
		}, nil
	}
}
